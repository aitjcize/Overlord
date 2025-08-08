// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
)

const (
	defaultForwardHost = "127.0.0.1"

	ldInterval       = 5
	usrShareDir      = "../share/overlord"
	webRootDirName   = "webroot"
	appsDirName      = "apps"
	fileOpMaxRetries = 100
	fileOpRetryDelay = 200 * time.Millisecond
)

// SpawnTerminalCmd is an overlord intend to launch a terminal.
type SpawnTerminalCmd struct {
	Sid       string // Session ID
	TtyDevice string // Termainl device to open
}

// SpawnShellCmd is an overlord intend to launch a shell command.
type SpawnShellCmd struct {
	Sid     string // Session ID
	Command string // Command to execute
}

// SpawnFileCmd is an overlord intend to perform file transfer.
type SpawnFileCmd struct {
	Sid         string // Session ID
	TerminalSid string // Target terminal's session ID
	Action      string // Action, download or upload
	Filename    string // File to perform action on
	Dest        string // Destination, use for upload
	Perm        int    // File permissions to set
	CheckOnly   bool   // Check permission only (writable?)
}

// SpawnModeForwarderCmd is an overlord intend to perform port forwarding.
type SpawnModeForwarderCmd struct {
	Sid  string // Session ID
	Host string // Host to forward
	Port int    // Port to forward
}

// ConnectLogcatCmd is an overlord intend to connect to a logcat session.
type ConnectLogcatCmd struct {
	Conn *websocket.Conn
}

// ListTreeCmd is a command to request a directory listing.
type ListTreeCmd struct {
	Path string
}

// FstatCmd is a command to request a file stat.
type FstatCmd struct {
	Path string
}

// CreateSymlinkCmd is a command to create a symlink.
type CreateSymlinkCmd struct {
	Target string // Target path
	Dest   string // Destination path
}

// MkdirCmd is a command to create a directory.
type MkdirCmd struct {
	Path string // Path to create
	Perm int    // Directory permissions
}

// webSocketContext is used for maintaining the session information of
// WebSocket requests. When requests come from Web Server, we create a new
// WebSocketConext to store the session ID and WebSocket connection. ConnServer
// will request a new terminal connection with the given session ID.
// This way, the ConnServer can retrieve the connresponding webSocketContext
// with it's the given session ID and get the WebSocket.
type webSocketContext struct {
	Sid  string          // Session ID
	Conn *websocket.Conn // WebSocket connection
}

// newWebsocketContext create  webSocketContext object.
func newWebsocketContext(conn *websocket.Conn) *webSocketContext {
	return &webSocketContext{
		Sid:  uuid.NewV4().String(),
		Conn: conn,
	}
}

// TLSCerts stores the TLS certificate filenames.
type TLSCerts struct {
	Cert string // cert.pem filename
	Key  string // key.pem filename
}

// BroadcastMessage represents a message to be broadcast
type BroadcastMessage struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// WebSocketClient represents a connected websocket client
type WebSocketClient struct {
	conn     *websocket.Conn
	sendChan chan []byte
	username string
	isAdmin  bool
}

// Overlord type is the main context for storing the overlord server state.
type Overlord struct {
	bindAddr         string                            // Bind address
	port             int                               // Port number to listen to
	lanDiscInterface string                            // Network interface used for broadcasting LAN discovery packet
	lanDisc          bool                              // Enable LAN discovery broadcasting
	dbPath           string                            // Path to the SQLite database file
	dbManager        *DatabaseManager                  // Database manager for users and groups
	certs            *TLSCerts                         // TLS certificate
	linkTLS          bool                              // Enable TLS between ghost and overlord
	agents           map[string]*ConnServer            // Normal ghost agents
	logcats          map[string]map[string]*ConnServer // logcat clients
	wsctxs           map[string]*webSocketContext      // (sid, webSocketContext) mapping
	downloads        map[string]*ConnServer            // Download file agents
	uploads          map[string]*ConnServer            // Upload file agents
	monitorClients   map[*WebSocketClient]bool         // Connected monitor WebSocket clients
	agentsMu         sync.Mutex                        // Mutex for agents
	logcatsMu        sync.Mutex                        // Mutex for logcats
	wsctxsMu         sync.Mutex                        // Mutex for wsctxs
	downloadsMu      sync.Mutex                        // Mutex for downloads
	uploadsMu        sync.Mutex                        // Mutex for uploads
	monitorClientsMu sync.RWMutex                      // Mutex for monitorClients
	upgrader         websocket.Upgrader                // Websocket upgrader
}

// NewOverlord creates an Overlord object.
func NewOverlord(
	bindAddr string, port int,
	lanDiscInterface string,
	lanDisc bool,
	certsString string, linkTLS bool,
	dbPath string) *Overlord {

	var certs *TLSCerts
	if certsString != "" {
		parts := strings.Split(certsString, ",")
		if len(parts) != 2 {
			log.Fatalf("TLSCerts: invalid TLS certs argument")
		} else {
			certs = &TLSCerts{parts[0], parts[1]}
		}
	}
	upgrader := websocket.Upgrader{
		ReadBufferSize:  bufferSize,
		WriteBufferSize: bufferSize,
		Subprotocols:    []string{"binary"},
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	dbManager := NewDatabaseManager(dbPath)
	if err := dbManager.Connect(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	ovl := &Overlord{
		bindAddr:         bindAddr,
		port:             port,
		lanDiscInterface: lanDiscInterface,
		lanDisc:          lanDisc,
		dbPath:           dbPath,
		dbManager:        dbManager,
		certs:            certs,
		linkTLS:          linkTLS,
		agents:           make(map[string]*ConnServer),
		logcats:          make(map[string]map[string]*ConnServer),
		wsctxs:           make(map[string]*webSocketContext),
		downloads:        make(map[string]*ConnServer),
		uploads:          make(map[string]*ConnServer),
		monitorClients:   make(map[*WebSocketClient]bool),
		upgrader:         upgrader,
	}
	return ovl
}

// Register a client.
func (ovl *Overlord) Register(conn *ConnServer) (*websocket.Conn, error) {
	msg, err := json.Marshal(map[string]interface{}{
		"mid": conn.Mid,
		"sid": conn.Sid,
	})
	if err != nil {
		return nil, err
	}

	var wsconn *websocket.Conn

	switch conn.Mode {
	case ModeControl:
		ovl.agentsMu.Lock()
		if _, ok := ovl.agents[conn.Mid]; ok {
			ovl.agentsMu.Unlock()
			return nil, errors.New("duplicate machine ID: " + conn.Mid)
		}
		ovl.agents[conn.Mid] = conn
		ovl.agentsMu.Unlock()
		ovl.BroadcastEvent(conn.Mid, "agent joined", string(msg))
	case ModeTerminal, ModeShell, ModeForward:
		ovl.wsctxsMu.Lock()
		ctx, ok := ovl.wsctxs[conn.Sid]
		if !ok {
			ovl.wsctxsMu.Unlock()
			return nil, errors.New("client " + conn.Sid + " registered without context")
		}
		wsconn = ctx.Conn
		ovl.wsctxsMu.Unlock()
	case ModeLogcat:
		ovl.logcatsMu.Lock()
		if _, ok := ovl.logcats[conn.Mid]; !ok {
			ovl.logcats[conn.Mid] = make(map[string]*ConnServer)
		}
		if _, ok := ovl.logcats[conn.Mid][conn.Sid]; ok {
			ovl.logcatsMu.Unlock()
			return nil, errors.New("duplicate session ID: " + conn.Sid)
		}
		ovl.logcats[conn.Mid][conn.Sid] = conn
		ovl.logcatsMu.Unlock()
		ovl.BroadcastEvent(conn.Mid, "logcat joined", string(msg))
	case ModeFile:
		// Do nothing, we wait until 'request_to_download' call from client to
		// send the message to the browser
	default:
		return nil, errors.New("unknown client mode")
	}

	var id string
	if conn.Mode == ModeControl {
		id = conn.Mid
	} else {
		id = conn.Sid
	}

	log.Printf("%s %s registered\n", ModeStr(conn.Mode), id)

	return wsconn, nil
}

// Unregister a client.
func (ovl *Overlord) Unregister(conn *ConnServer) {
	msg, err := json.Marshal(map[string]interface{}{
		"mid": conn.Mid,
		"sid": conn.Sid,
	})

	if err != nil {
		panic(err)
	}

	switch conn.Mode {
	case ModeControl:
		ovl.BroadcastEvent(conn.Mid, "agent left", string(msg))
		ovl.agentsMu.Lock()
		delete(ovl.agents, conn.Mid)
		ovl.agentsMu.Unlock()
	case ModeLogcat:
		ovl.logcatsMu.Lock()
		if _, ok := ovl.logcats[conn.Mid]; ok {
			ovl.BroadcastEvent(conn.Mid, "logcat left", string(msg))
			delete(ovl.logcats[conn.Mid], conn.Sid)
			if len(ovl.logcats[conn.Mid]) == 0 {
				delete(ovl.logcats, conn.Mid)
			}
		}
		ovl.logcatsMu.Unlock()
	case ModeFile:
		ovl.downloadsMu.Lock()
		delete(ovl.downloads, conn.Sid)
		ovl.downloadsMu.Unlock()
		ovl.uploadsMu.Lock()
		delete(ovl.uploads, conn.Sid)
		ovl.uploadsMu.Unlock()
	default:
		ovl.wsctxsMu.Lock()
		delete(ovl.wsctxs, conn.Sid)
		ovl.wsctxsMu.Unlock()
	}

	var id string
	if conn.Mode == ModeControl {
		id = conn.Mid
	} else {
		id = conn.Sid
	}
	log.Printf("%s %s unregistered\n", ModeStr(conn.Mode), id)
}

// addWebsocketContext adds an websocket context to the overlord state.
func (ovl *Overlord) addWebsocketContext(wc *webSocketContext) {
	ovl.wsctxsMu.Lock()
	ovl.wsctxs[wc.Sid] = wc
	ovl.wsctxsMu.Unlock()
}

// RegisterDownloadRequest registers a file download request.
func (ovl *Overlord) RegisterDownloadRequest(conn *ConnServer) {
	// Use session ID as download session ID instead of machine ID, so a machine
	// can have multiple download at the same time
	ovl.BroadcastEvent(conn.Mid, "file download", conn.Sid, conn.TerminalSid)
	ovl.downloadsMu.Lock()
	ovl.downloads[conn.Sid] = conn
	ovl.downloadsMu.Unlock()
}

// RegisterUploadRequest registers a file upload request.
func (ovl *Overlord) RegisterUploadRequest(conn *ConnServer) {
	// Use session ID as upload session ID instead of machine ID, so a machine
	// can have multiple upload at the same time
	ovl.BroadcastEvent(conn.Mid, "file upload", conn.Sid, conn.TerminalSid)
	ovl.uploadsMu.Lock()
	ovl.uploads[conn.Sid] = conn
	ovl.uploadsMu.Unlock()
}

// BroadcastEvent broadcasts an event to all monitor clients
// If mid is not empty, the event will only be sent to clients with permission
// to access that agent
func (ovl *Overlord) BroadcastEvent(mid, event string, args ...interface{}) {
	msg := BroadcastMessage{
		Event: event,
		Data:  args,
	}

	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal broadcast message: %v", err)
		return
	}

	ovl.monitorClientsMu.RLock()
	for client := range ovl.monitorClients {
		// If mid is provided, check permissions
		if mid != "" {
			ovl.agentsMu.Lock()
			agent, exists := ovl.agents[mid]
			ovl.agentsMu.Unlock()

			// Skip this client if agent exists and user doesn't have permission
			if exists && !ovl.checkAllowlist(client.username, client.isAdmin, agent) {
				continue
			}
		}

		select {
		case client.sendChan <- jsonMsg:
		default:
			close(client.sendChan)
			delete(ovl.monitorClients, client)
		}
	}
	ovl.monitorClientsMu.RUnlock()
}

// getWebRoot returns the absolute path to the webroot directory.
func (ovl *Overlord) getWebRoot() string {
	execPath, err := os.Executable()
	if err != nil {
		log.Fatalln(err)
	}
	execDir := filepath.Dir(execPath)

	webroot := filepath.Join(execDir, webRootDirName)

	if _, err := os.Stat(webroot); err != nil {
		// Try system install directory
		webroot, err = filepath.Abs(
			filepath.Join(execDir, usrShareDir, webRootDirName))
		if err != nil {
			log.Fatalln(err)
		}
		if _, err := os.Stat(webroot); err != nil {
			log.Fatalln("Can not find webroot directory")
		}
	}
	return webroot
}

func (ovl *Overlord) getAppDir() string {
	appDir, err := filepath.Abs(filepath.Join(ovl.getWebRoot(), appsDirName))
	if err != nil {
		log.Fatalln(err)
	}
	return appDir
}

func (ovl *Overlord) getAppNames() ([]string, error) {
	appNames := []string{}

	apps, err := os.ReadDir(ovl.getAppDir())
	if err != nil {
		return nil, nil
	}

	for _, app := range apps {
		if !app.IsDir() {
			continue
		}
		appNames = append(appNames, app.Name())
	}
	return appNames, nil
}

func (ovl *Overlord) ghostConnectHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := ovl.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	log.Printf("Incoming connection from %s", conn.UnderlyingConn().RemoteAddr())
	cs := NewConnServer(ovl, conn.UnderlyingConn())
	cs.Listen()
}

// List all apps available on Overlord.
func (ovl *Overlord) appsListHandler(w http.ResponseWriter, r *http.Request) {
	apps, err := ovl.getAppNames()
	if err != nil {
		ResponseError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ResponseSuccess(w, apps)
}

// List all agents connected to the Overlord.
func (ovl *Overlord) agentsListHandler(w http.ResponseWriter, r *http.Request) {
	username, ok := GetUserFromContext(r.Context())
	if !ok {
		ResponseError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	isAdmin, ok := GetAdminStatusFromContext(r.Context())
	if !ok {
		ResponseError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	result := make([]map[string]interface{}, 0)
	ovl.agentsMu.Lock()
	for _, agent := range ovl.agents {
		if !ovl.checkAllowlist(username, isAdmin, agent) {
			continue
		}

		result = append(result, map[string]interface{}{
			"mid":        agent.Mid,
			"sid":        agent.Sid,
			"properties": agent.Properties,
		})
	}
	ovl.agentsMu.Unlock()

	// Sort by machine ID for consistent output
	sort.Slice(result, func(i, j int) bool {
		return result[i]["mid"].(string) < result[j]["mid"].(string)
	})

	ResponseSuccess(w, result)
}

// Agent upgrade request handler.
func (ovl *Overlord) agentsUpgradeHandler(w http.ResponseWriter, r *http.Request) {
	var failedAgents []string

	ovl.agentsMu.Lock()
	for _, agent := range ovl.agents {
		err := agent.SendUpgradeRequest()
		if err != nil {
			failedAgents = append(failedAgents, agent.Mid)
		}
	}
	ovl.agentsMu.Unlock()

	if len(failedAgents) > 0 {
		ResponseError(w, fmt.Sprintf("Failed to send upgrade request for agents: %s",
			strings.Join(failedAgents, ", ")), http.StatusInternalServerError)
		return
	}
	ResponseSuccess(w, nil)
}

// List all logcat clients connected to the Overlord.
func (ovl *Overlord) logcatsListHandler(w http.ResponseWriter, r *http.Request) {
	var data = make([]map[string]interface{}, 0)
	ovl.logcatsMu.Lock()
	for mid, logcats := range ovl.logcats {
		var sids []string
		for sid := range logcats {
			sids = append(sids, sid)
		}
		data = append(data, map[string]interface{}{
			"mid":  mid,
			"sids": sids,
		})
	}
	ovl.logcatsMu.Unlock()

	ResponseSuccess(w, data)
}

// Logcat request handler.
// We directly send the WebSocket connection to ConnServer for forwarding
// the log stream.
func (ovl *Overlord) logcatHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Logcat request from %s\n", r.RemoteAddr)

	conn, err := ovl.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	vars := mux.Vars(r)
	mid := vars["mid"]
	sid := vars["sid"]

	ovl.logcatsMu.Lock()
	if logcats, ok := ovl.logcats[mid]; ok {
		ovl.logcatsMu.Unlock()
		if logcat, ok := logcats[sid]; ok {
			logcat.Command <- ConnectLogcatCmd{conn}
		} else {
			WebSocketSendError(conn, "No client with sid "+sid)
		}
	} else {
		ovl.logcatsMu.Unlock()
		WebSocketSendError(conn, "No client with mid "+mid)
	}
}

// spawnAgentCommand is a helper function to eliminate duplicate code between terminal and shell handlers
func (ovl *Overlord) spawnAgentCommand(conn *websocket.Conn, mid string, cmd interface{}) {
	ovl.agentsMu.Lock()
	if agent, ok := ovl.agents[mid]; ok {
		ovl.agentsMu.Unlock()

		agent.Command <- cmd
		if res := <-agent.Response; res.Status == Failed {
			var error Error
			if err := json.Unmarshal(res.Payload, &error); err != nil {
				WebSocketSendError(conn, "Failed to unmarshal error response: "+err.Error())
			} else {
				WebSocketSendError(conn, error.Error)
			}
		}
	} else {
		ovl.agentsMu.Unlock()
		WebSocketSendError(conn, "No client with mid "+mid)
	}
}

// TTY stream request handler.
// We first create a webSocketContext to store the connection, then send a
// command to Overlord to client to spawn a terminal connection.
func (ovl *Overlord) agentTtyHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Terminal request from %s\n", r.RemoteAddr)

	// Upgrade the connection to WebSocket
	conn, err := ovl.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	var ttyDevice string

	vars := mux.Vars(r)
	mid := vars["mid"]
	if _ttyDevice, ok := r.URL.Query()["tty_device"]; ok {
		ttyDevice = _ttyDevice[0]
	}

	wc := newWebsocketContext(conn)
	ovl.addWebsocketContext(wc)
	ovl.spawnAgentCommand(conn, mid, SpawnTerminalCmd{wc.Sid, ttyDevice})
}

// Shell command request handler.
// We first create a webSocketContext to store the connection, then send a
// command to ConnServer to client to spawn a shell connection.
func (ovl *Overlord) modeShellHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Shell request from %s\n", r.RemoteAddr)

	// Upgrade the connection to WebSocket
	conn, err := ovl.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	vars := mux.Vars(r)
	mid := vars["mid"]
	command := r.URL.Query().Get("command")

	wc := newWebsocketContext(conn)
	ovl.addWebsocketContext(wc)
	ovl.spawnAgentCommand(conn, mid, SpawnShellCmd{wc.Sid, command})
}

// Get agent properties as JSON.
func (ovl *Overlord) agentPropertiesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	mid := vars["mid"]
	ovl.agentsMu.Lock()
	if agent, ok := ovl.agents[mid]; ok {
		ovl.agentsMu.Unlock()
		ResponseSuccess(w, agent.Properties)
	} else {
		ovl.agentsMu.Unlock()
		ResponseError(w, "No client with mid "+mid, http.StatusNotFound)
	}
}

// Helper function for serving file and write it into response body.
func (ovl *Overlord) serveFileHTTP(w http.ResponseWriter, c *ConnServer) {
	defer func() {
		if c != nil {
			c.StopListen()
		}
	}()
	c.SendClearToDownload()

	dispose := fmt.Sprintf("attachment; filename=\"%s\"", c.Download.Name)
	w.Header().Set("Content-Disposition", dispose)
	w.Header().Set("Content-Length", strconv.FormatInt(c.Download.Size, 10))

	for {
		data := <-c.Download.Data
		if data == nil {
			return
		}
		if _, err := w.Write(data); err != nil {
			break
		}
	}
}

// File download request handler.
// Handler for file download request, the filename target machine is
// specified in the request URL.
func (ovl *Overlord) agentFileDownloadHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("File download request from %s\n", r.RemoteAddr)

	vars := mux.Vars(r)
	mid := vars["mid"]

	var agent *ConnServer
	var ok bool

	ovl.agentsMu.Lock()
	if agent, ok = ovl.agents[mid]; !ok {
		ovl.agentsMu.Unlock()
		ResponseError(w, "No such agent exists", http.StatusBadRequest)
		return
	}
	ovl.agentsMu.Unlock()

	var filename []string
	if filename, ok = r.URL.Query()["filename"]; !ok {
		ResponseError(w, "No filename specified", http.StatusBadRequest)
		return
	}

	sid := uuid.NewV4().String()
	agent.Command <- SpawnFileCmd{
		Sid: sid, Action: "download", Filename: filename[0]}

	res := <-agent.Response
	if res.Status == Failed {
		ResponseJSON(w, string(res.Payload), http.StatusBadRequest)
		return
	}

	var c *ConnServer
	count := 0

	// Wait until download client connects
	for {
		if count++; count == fileOpMaxRetries {
			ResponseError(w, "Download client connection timeout", http.StatusInternalServerError)
			return
		}
		ovl.downloadsMu.Lock()
		if c, ok = ovl.downloads[sid]; ok {
			ovl.downloadsMu.Unlock()
			break
		}
		ovl.downloadsMu.Unlock()
		time.Sleep(fileOpRetryDelay)
	}
	ovl.serveFileHTTP(w, c)
}

// Passive file download request handler.
// This handler deal with requests that are initiated by the client. We
// simply check if the session id exists in the download client list, than
// start to download the file if it does.
func (ovl *Overlord) sessionFileDownloadHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]

	var c *ConnServer
	var ok bool

	ovl.downloadsMu.Lock()
	if c, ok = ovl.downloads[sid]; !ok {
		ovl.downloadsMu.Unlock()
		ResponseError(w, "No download session with ID "+sid, http.StatusNotFound)
		return
	}
	ovl.downloadsMu.Unlock()
	ovl.serveFileHTTP(w, c)
}

// Port forwarding handler
func (ovl *Overlord) agentModeForwardHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("ModeForward request from %s\n", r.RemoteAddr)
	conn, err := ovl.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	host := defaultForwardHost
	var port int

	vars := mux.Vars(r)
	mid := vars["mid"]

	// default host to 127.0.0.1 if not specified
	if _host, ok := r.URL.Query()["host"]; ok {
		host = _host[0]
	}

	if _port, ok := r.URL.Query()["port"]; ok {
		if port, err = strconv.Atoi(_port[0]); err != nil {
			WebSocketSendError(conn, "invalid port")
			return
		}
	} else {
		WebSocketSendError(conn, "no port specified")
		return
	}

	ovl.agentsMu.Lock()
	if agent, ok := ovl.agents[mid]; ok {
		ovl.agentsMu.Unlock()

		wc := newWebsocketContext(conn)
		ovl.addWebsocketContext(wc)
		agent.Command <- SpawnModeForwarderCmd{wc.Sid, host, port}
		if res := <-agent.Response; res.Status == Failed {
			WebSocketSendError(conn, string(res.Payload))
		}
	} else {
		ovl.agentsMu.Unlock()
		WebSocketSendError(conn, "No client with mid "+mid)
	}
}

// File listing and stat operations handler
func (ovl *Overlord) agentFsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	mid := vars["mid"]

	ovl.agentsMu.Lock()
	agent, ok := ovl.agents[mid]
	ovl.agentsMu.Unlock()
	if !ok {
		ResponseError(w, "No client with mid "+mid, http.StatusNotFound)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		ResponseError(w, "path parameter is required", http.StatusBadRequest)
		return
	}

	op := r.URL.Query().Get("op")
	if op == "" {
		op = ListTreeOp // Default to listing
	}

	switch op {
	case ListTreeOp:
		agent.Command <- ListTreeCmd{Path: path}
	case "fstat":
		agent.Command <- FstatCmd{Path: path}
	default:
		ResponseError(w, "Unknown operation "+op, http.StatusBadRequest)
		return
	}

	res := <-agent.Response
	if res.Status == Failed {
		ResponseJSON(w, string(res.Payload), http.StatusInternalServerError)
		return
	}
	ResponseJSON(w, string(res.Payload), http.StatusOK)
}

// Symlink creation handler
func (ovl *Overlord) agentFsSymlinkHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	mid := vars["mid"]

	ovl.agentsMu.Lock()
	agent, ok := ovl.agents[mid]
	ovl.agentsMu.Unlock()
	if !ok {
		ResponseError(w, "No client with mid "+mid, http.StatusNotFound)
		return
	}

	target := r.URL.Query().Get("target")
	dest := r.URL.Query().Get("dest")

	if target == "" {
		ResponseError(w, "target parameter is required", http.StatusBadRequest)
		return
	}
	if dest == "" {
		ResponseError(w, "dest parameter is required", http.StatusBadRequest)
		return
	}

	agent.Command <- CreateSymlinkCmd{Target: target, Dest: dest}
	res := <-agent.Response
	if res.Status == Failed {
		ResponseJSON(w, string(res.Payload), http.StatusInternalServerError)
		return
	}
	ResponseJSON(w, string(res.Payload), http.StatusOK)
}

// Directory creation handler
func (ovl *Overlord) agentFsDirHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	mid := vars["mid"]

	ovl.agentsMu.Lock()
	agent, ok := ovl.agents[mid]
	ovl.agentsMu.Unlock()
	if !ok {
		ResponseError(w, "No client with mid "+mid, http.StatusNotFound)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		ResponseError(w, "path parameter is required", http.StatusBadRequest)
		return
	}

	perm := 0755
	if permStr := r.URL.Query().Get("perm"); permStr != "" {
		var err error
		perm64, err := strconv.ParseInt(permStr, 10, 32)
		if err != nil {
			ResponseError(w, "invalid perm parameter", http.StatusBadRequest)
			return
		}
		perm = int(perm64)
	}

	agent.Command <- MkdirCmd{Path: path, Perm: perm}
	res := <-agent.Response
	if res.Status == Failed {
		ResponseJSON(w, string(res.Payload), http.StatusInternalServerError)
		return
	}
	ResponseJSON(w, string(res.Payload), http.StatusOK)
}

// File upload handler
func (ovl *Overlord) agentFileUploadHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	mid := vars["mid"]

	ovl.agentsMu.Lock()
	agent, ok := ovl.agents[mid]
	ovl.agentsMu.Unlock()
	if !ok {
		ResponseError(w, "No client with mid "+mid, http.StatusNotFound)
		return
	}

	var terminalSid string
	if terminalSids, ok := r.URL.Query()["terminal_sid"]; ok {
		terminalSid = terminalSids[0]
	}

	var dest string
	if dests, ok := r.URL.Query()["dest"]; ok {
		dest = dests[0]
	}

	var perm int64 = 0644 // Default permission
	if perms, ok := r.URL.Query()["perm"]; ok {
		var err error
		perm, err = strconv.ParseInt(perms[0], 8, 32)
		if err != nil {
			ResponseError(w, "invalid permission format", http.StatusBadRequest)
			return
		}
	}

	mr, err := r.MultipartReader()
	if err != nil {
		ResponseError(w, err.Error(), http.StatusBadRequest)
		return
	}

	p, err := mr.NextPart()
	if err != nil {
		ResponseError(w, err.Error(), http.StatusBadRequest)
		return
	}

	sid := uuid.NewV4().String()
	agent.Command <- SpawnFileCmd{sid, terminalSid, "upload",
		p.FileName(), dest, int(perm), false}

	res := <-agent.Response
	if res.Status == Failed {
		ResponseJSON(w, string(res.Payload), http.StatusBadRequest)
		return
	}

	count := 0

	// Wait until upload client connects
	var c *ConnServer
	for {
		if count++; count == fileOpMaxRetries {
			ResponseError(w, "no response from client", http.StatusInternalServerError)
			return
		}
		ovl.uploadsMu.Lock()
		if c, ok = ovl.uploads[sid]; ok {
			ovl.uploadsMu.Unlock()
			break
		}
		ovl.uploadsMu.Unlock()
		time.Sleep(fileOpRetryDelay)
	}

	_, err = io.Copy(c.Conn, p)
	c.StopListen()

	if err != nil {
		ResponseError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ResponseSuccess(w, nil)
}

// WebSocket monitor endpoint
func (ovl *Overlord) monitorHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Monitor request from %s\n", r.RemoteAddr)

	username, ok := GetUserFromContext(r.Context())
	if !ok {
		ResponseError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	isAdmin, ok := GetAdminStatusFromContext(r.Context())
	if !ok {
		ResponseError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Upgrade the connection to WebSocket
	conn, err := ovl.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	client := &WebSocketClient{
		conn:     conn,
		sendChan: make(chan []byte, 256),
		username: username,
		isAdmin:  isAdmin,
	}

	ovl.monitorClientsMu.Lock()
	ovl.monitorClients[client] = true
	ovl.monitorClientsMu.Unlock()

	// Start client read/write pumps
	go client.writePump()
	go client.readPump(ovl)
}

// register handlers for http routes.
func (ovl *Overlord) registerRoutes() *mux.Router {
	// JWT Auth with database
	jwtConfig := &JWTConfig{
		DBPath: ovl.dbPath,
	}
	jwtAuth, err := NewJWTAuth(jwtConfig)
	if err != nil {
		log.Fatalf("Failed to initialize JWT authentication: %v", err)
	}

	// Create a main router for all API endpoints
	mainRouter := mux.NewRouter()

	// Public routes (no authentication required)
	publicRouter := mainRouter.PathPrefix("/api").Subrouter()
	publicRouter.HandleFunc("/auth/login", jwtAuth.Login).Methods("POST")

	// Protected routes (authentication required)
	apiRouter := mainRouter.PathPrefix("/api").Subrouter()
	apiRouter.Use(jwtAuth.Middleware)

	// Register agent-specific routes with both auth and allowlist middleware
	agentRoutes := apiRouter.PathPrefix("/agents").Subrouter()
	agentRoutes.Use(ovl.allowlistMiddleware)

	// Register agent-specific routes with the allowlist middleware
	agentRoutes.HandleFunc("/{mid}/properties", ovl.agentPropertiesHandler).Methods("GET")
	agentRoutes.HandleFunc("/{mid}/file", ovl.agentFileDownloadHandler).Methods("GET")
	agentRoutes.HandleFunc("/{mid}/file", ovl.agentFileUploadHandler).Methods("POST")
	agentRoutes.HandleFunc("/{mid}/fs", ovl.agentFsHandler).Methods("GET")
	agentRoutes.HandleFunc("/{mid}/fs/symlinks", ovl.agentFsSymlinkHandler).Methods("POST")
	agentRoutes.HandleFunc("/{mid}/fs/directories", ovl.agentFsDirHandler).Methods("POST")
	agentRoutes.HandleFunc("/{mid}/tty", ovl.agentTtyHandler).Methods("GET")
	agentRoutes.HandleFunc("/{mid}/shell", ovl.modeShellHandler).Methods("GET")
	agentRoutes.HandleFunc("/{mid}/forward", ovl.agentModeForwardHandler).Methods("GET")

	// These routes need authentication but not allowlist check
	apiRouter.HandleFunc("/agents", ovl.agentsListHandler).Methods("GET")
	apiRouter.HandleFunc("/agents/upgrade", ovl.agentsUpgradeHandler).Methods("POST")

	// Other authenticated routes
	apiRouter.HandleFunc("/apps", ovl.appsListHandler).Methods("GET")
	apiRouter.HandleFunc("/logcats", ovl.logcatsListHandler).Methods("GET")
	apiRouter.HandleFunc("/logcats/{mid}/{sid}", ovl.logcatHandler).Methods("GET")
	apiRouter.HandleFunc("/monitor", ovl.monitorHandler).Methods("GET")
	apiRouter.HandleFunc("/sessions/{sid}/file", ovl.sessionFileDownloadHandler).Methods("GET")

	// Connect endpoint (special case for agent connections)
	mainRouter.HandleFunc("/connect", ovl.ghostConnectHandler)

	// User management API routes
	userRoutes := apiRouter.PathPrefix("/users").Subrouter()

	// Route for users to change their own password (no admin check)
	userRoutes.HandleFunc("/self/password", ovl.updateOwnPasswordHandler).Methods("PUT")

	// Routes that require admin privileges
	adminUserRoutes := userRoutes.NewRoute().Subrouter()
	adminUserRoutes.Use(ovl.adminRequired)
	adminUserRoutes.HandleFunc("", ovl.listUsersHandler).Methods("GET")
	adminUserRoutes.HandleFunc("", ovl.createUserHandler).Methods("POST")
	adminUserRoutes.HandleFunc("/{username}", ovl.deleteUserHandler).Methods("DELETE")
	adminUserRoutes.HandleFunc("/{username}/password", ovl.updateUserPasswordHandler).Methods("PUT")

	// Group management API routes
	groupRoutes := apiRouter.PathPrefix("/groups").Subrouter()
	// All group management requires admin privileges
	groupRoutes.Use(ovl.adminRequired)
	groupRoutes.HandleFunc("", ovl.listGroupsHandler).Methods("GET")
	groupRoutes.HandleFunc("", ovl.createGroupHandler).Methods("POST")
	groupRoutes.HandleFunc("/{groupname}", ovl.deleteGroupHandler).Methods("DELETE")
	groupRoutes.HandleFunc("/{groupname}/users", ovl.addUserToGroupHandler).Methods("POST")
	groupRoutes.HandleFunc("/{groupname}/users/{username}", ovl.removeUserFromGroupHandler).Methods("DELETE")
	groupRoutes.HandleFunc("/{groupname}/users", ovl.listGroupUsersHandler).Methods("GET")

	return mainRouter
}

func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(time.Second * 30)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.sendChan:
			if !ok {
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					log.Printf("Failed to write WebSocket close message: %v", err)
				}
				return
			}

			err := c.conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				return
			}

		case <-ticker.C:
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *WebSocketClient) readPump(ovl *Overlord) {
	defer func() {
		ovl.monitorClientsMu.Lock()
		delete(ovl.monitorClients, c)
		ovl.monitorClientsMu.Unlock()
		close(c.sendChan)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		log.Printf("Failed to set read deadline: %v", err)
	}
	c.conn.SetPongHandler(func(string) error {
		if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			log.Printf("Failed to set read deadline: %v", err)
		}
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
	}
}

// ServHTTP is the Web server main routine.
func (ovl *Overlord) ServHTTP() {
	var err error

	tlsStatus := "disabled"
	if ovl.certs != nil {
		tlsStatus = "enabled"
	}
	if ovl.port == 0 {
		if ovl.certs != nil {
			ovl.port = DefaultHTTPSPort
		} else {
			ovl.port = DefaultHTTPPort
		}
	}

	addr := fmt.Sprintf("%s:%d", ovl.bindAddr, ovl.port)
	log.Printf("HTTP server started, listening at %s, TLS: %s",
		addr, tlsStatus)

	if ovl.certs != nil {
		err = http.ListenAndServeTLS(addr, ovl.certs.Cert, ovl.certs.Key, nil)
	} else {
		err = http.ListenAndServe(addr, nil)
	}
	if err != nil {
		log.Fatalf("net.http could not listen on address '%s': %s\n",
			addr, err)
	}
}

// StartUDPBroadcast is the main routine for broadcasting LAN discovery message.
func (ovl *Overlord) StartUDPBroadcast(port int) {
	ifaceIP := ""
	bcastIP := net.IPv4bcast.String()

	if ovl.lanDiscInterface != "" {
		interfaces, err := net.Interfaces()
		if err != nil {
			panic(err)
		}

	outter:
		for _, iface := range interfaces {
			if iface.Name == ovl.lanDiscInterface {
				addrs, err := iface.Addrs()
				if err != nil {
					panic(err)
				}
				for _, addr := range addrs {
					ip, ipnet, err := net.ParseCIDR(addr.String())
					if err != nil {
						continue
					}
					// Calculate broadcast IP
					ip4 := ip.To4()

					// We only care about IPv4 address
					if ip4 == nil {
						continue
					}
					bcastIPraw := make(net.IP, 4)
					for i := 0; i < 4; i++ {
						bcastIPraw[i] = ip4[i] | ^ipnet.Mask[i]
					}
					ifaceIP = ip.String()
					bcastIP = bcastIPraw.String()
					break outter
				}
			}
		}

		if ifaceIP == "" {
			log.Fatalf("can not found any interface with name %s\n",
				ovl.lanDiscInterface)
		}
	}

	addr := fmt.Sprintf("%s:%d", bcastIP, port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		log.Fatalln("Unable to start UDP broadcast:", err)
	}

	log.Printf("UDP Broadcasting started, broadcasting at %s", addr)

	ticker := time.NewTicker(ldInterval * time.Second)

	for range ticker.C {
		if _, err := fmt.Fprintf(conn, "OVERLORD %s:%d", ifaceIP, ovl.port); err != nil {
			log.Printf("Failed to write UDP broadcast: %v", err)
		}
	}
}

// Serv is the main routine for starting all the overlord sub-server.
func (ovl *Overlord) Serv() {
	router := ovl.registerRoutes()

	// Serve static files
	router.PathPrefix("/").Handler(http.FileServer(http.Dir(ovl.getWebRoot())))

	http.Handle("/", router)

	go ovl.ServHTTP()
	if ovl.lanDisc {
		go ovl.StartUDPBroadcast(OverlordLDPort)
	}

	ticker := time.NewTicker(60 * time.Second)

	for range ticker.C {
		log.Printf("#Goroutines, #Ghostclient: %d, %d\n",
			runtime.NumGoroutine(), len(ovl.agents))
	}
}

// checkAllowlist checks if the provided username has access to the ghost agent
// based on the allowlist property
func (ovl *Overlord) checkAllowlist(username string, isAdmin bool, agent *ConnServer) bool {
	if isAdmin {
		return true
	}

	// If no allowlist is provided, deny access
	if agent.Properties == nil {
		return false
	}

	// Get the allowlist from agent properties
	allowlistProp, ok := agent.Properties["allowlist"]
	if !ok {
		// No allowlist property, deny access
		return false
	}

	if allowlistArray, ok := allowlistProp.([]interface{}); ok {
		var allowlist []string
		for _, entity := range allowlistArray {
			if entityStr, ok := entity.(string); ok {
				allowlist = append(allowlist, entityStr)
			}
		}

		hasAccess, err := ovl.dbManager.CheckAllowlist(username, allowlist)
		if err != nil {
			log.Printf("Error checking allowlist: %v", err)
			return false
		}
		return hasAccess
	}

	return false
}

// allowlistMiddleware checks if the user has permission to access the requested agent
func (ovl *Overlord) allowlistMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip non-agent routes
		vars := mux.Vars(r)
		mid, ok := vars["mid"]
		if !ok {
			// Route doesn't have a mid parameter, pass through
			next.ServeHTTP(w, r)
			return
		}

		if mid == "" {
			ResponseError(w, "Invalid agent ID", http.StatusBadRequest)
			return
		}

		username, ok := GetUserFromContext(r.Context())
		if !ok {
			ResponseError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		isAdmin, ok := GetAdminStatusFromContext(r.Context())
		if !ok {
			ResponseError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Check if agent exists and user has permission
		ovl.agentsMu.Lock()
		agent, exists := ovl.agents[mid]
		ovl.agentsMu.Unlock()

		if !exists {
			log.Printf("AllowlistMiddleware: Agent %s not found", mid)
			ResponseError(w, "No client with mid "+mid, http.StatusNotFound)
			return
		}

		if !ovl.checkAllowlist(username, isAdmin, agent) {
			log.Printf("AllowlistMiddleware: User %s not authorized to access agent %s", username, mid)
			ResponseError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// User has permission, continue to the next handler
		next.ServeHTTP(w, r)
	})
}

// StartOverlord starts the overlord server.
func StartOverlord(bindAddr string, port int, lanDiscInterface string, lanDisc bool,
	certsString string, linkTLS bool, dbPath string) {
	ovl := NewOverlord(bindAddr, port, lanDiscInterface, lanDisc, certsString,
		linkTLS, dbPath)
	ovl.Serv()
}
