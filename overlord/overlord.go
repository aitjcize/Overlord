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
	ldInterval         = 5
	usrShareDir        = "../share/overlord"
	webRootDirName     = "webroot"
	appsDirName        = "apps"
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
	Room  string      `json:"room"`
	Data  interface{} `json:"data"`
}

// WebSocketClient represents a connected websocket client
type WebSocketClient struct {
	conn     *websocket.Conn
	sendChan chan []byte
}

// Overlord type is the main context for storing the overlord server state.
type Overlord struct {
	bindAddr         string                            // Bind address
	port             int                               // Port number to listen to
	lanDiscInterface string                            // Network interface used for broadcasting LAN discovery packet
	lanDisc          bool                              // Enable LAN discovery broadcasting
	jwtSecretPath    string                            // Path to the file containing the JWT secret
	certs            *TLSCerts                         // TLS certificate
	linkTLS          bool                              // Enable TLS between ghost and overlord
	htpasswdPath     string                            // Path to .htpasswd file
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
}

// NewOverlord creates an Overlord object.
func NewOverlord(
	bindAddr string, port int,
	lanDiscInterface string,
	lanDisc bool,
	certsString string, linkTLS bool,
	htpasswdPath string, jwtSecretPath string) *Overlord {

	var certs *TLSCerts
	if certsString != "" {
		parts := strings.Split(certsString, ",")
		if len(parts) != 2 {
			log.Fatalf("TLSCerts: invalid TLS certs argument")
		} else {
			certs = &TLSCerts{parts[0], parts[1]}
		}
	}
	if !filepath.IsAbs(htpasswdPath) && htpasswdPath != "" {
		execPath, err := os.Executable()
		if err != nil {
			log.Fatalln(err)
		}
		execDir := filepath.Dir(execPath)
		htpasswdPath, err = filepath.Abs(filepath.Join(execDir, htpasswdPath))
		if err != nil {
			log.Fatalln(err)
		}
	}
	if !filepath.IsAbs(jwtSecretPath) && jwtSecretPath != "" {
		execPath, err := os.Executable()
		if err != nil {
			log.Fatalln(err)
		}
		execDir := filepath.Dir(execPath)
		jwtSecretPath, err = filepath.Abs(filepath.Join(execDir, jwtSecretPath))
		if err != nil {
			log.Fatalln(err)
		}
	}
	return &Overlord{
		bindAddr:         bindAddr,
		port:             port,
		lanDiscInterface: lanDiscInterface,
		lanDisc:          lanDisc,
		jwtSecretPath:    jwtSecretPath,
		certs:            certs,
		linkTLS:          linkTLS,
		htpasswdPath:     htpasswdPath,
		agents:           make(map[string]*ConnServer),
		logcats:          make(map[string]map[string]*ConnServer),
		wsctxs:           make(map[string]*webSocketContext),
		downloads:        make(map[string]*ConnServer),
		uploads:          make(map[string]*ConnServer),
		monitorClients:   make(map[*WebSocketClient]bool),
	}
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
		ovl.BroadcastEvent("monitor", "agent joined", string(msg))
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
		ovl.BroadcastEvent("monitor", "logcat joined", string(msg))
	case ModeFile:
		// Do nothing, we wait until 'request_to_download' call from client to
		// send the message to the browser
	default:
		return nil, errors.New("Unknown client mode")
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
		ovl.BroadcastEvent("monitor", "agent left", string(msg))
		ovl.agentsMu.Lock()
		delete(ovl.agents, conn.Mid)
		ovl.agentsMu.Unlock()
	case ModeLogcat:
		ovl.logcatsMu.Lock()
		if _, ok := ovl.logcats[conn.Mid]; ok {
			ovl.BroadcastEvent("monitor", "logcat left", string(msg))
			delete(ovl.logcats[conn.Mid], conn.Sid)
			if len(ovl.logcats[conn.Mid]) == 0 {
				delete(ovl.logcats, conn.Mid)
			}
		}
		ovl.logcatsMu.Unlock()
	case ModeFile:
		ovl.downloadsMu.Lock()
		if _, ok := ovl.downloads[conn.Sid]; ok {
			delete(ovl.downloads, conn.Sid)
		}
		ovl.downloadsMu.Unlock()
		ovl.uploadsMu.Lock()
		if _, ok := ovl.uploads[conn.Sid]; ok {
			delete(ovl.uploads, conn.Sid)
		}
		ovl.uploadsMu.Unlock()
	default:
		ovl.wsctxsMu.Lock()
		if _, ok := ovl.wsctxs[conn.Sid]; ok {
			delete(ovl.wsctxs, conn.Sid)
		}
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

// AddWebsocketContext adds an websocket context to the overlord state.
func (ovl *Overlord) AddWebsocketContext(wc *webSocketContext) {
	ovl.wsctxsMu.Lock()
	ovl.wsctxs[wc.Sid] = wc
	ovl.wsctxsMu.Unlock()
}

// RegisterDownloadRequest registers a file download request.
func (ovl *Overlord) RegisterDownloadRequest(conn *ConnServer) {
	// Use session ID as download session ID instead of machine ID, so a machine
	// can have multiple download at the same time
	ovl.BroadcastEvent(conn.TerminalSid, "file download", conn.Sid)
	ovl.downloadsMu.Lock()
	ovl.downloads[conn.Sid] = conn
	ovl.downloadsMu.Unlock()
}

// RegisterUploadRequest registers a file upload request.
func (ovl *Overlord) RegisterUploadRequest(conn *ConnServer) {
	// Use session ID as upload session ID instead of machine ID, so a machine
	// can have multiple upload at the same time
	ovl.BroadcastEvent(conn.TerminalSid, "file upload", conn.Sid)
	ovl.uploadsMu.Lock()
	ovl.uploads[conn.Sid] = conn
	ovl.uploadsMu.Unlock()
}

// Handle TCP Connection.
func (ovl *Overlord) handleConnection(conn net.Conn) {
	handler := NewConnServer(ovl, conn)
	go handler.Listen()
	ovl.BroadcastEvent("monitor", "agent joined", handler.Mid)
}

// BroadcastEvent broadcasts an event to all monitor clients
func (ovl *Overlord) BroadcastEvent(room, event string, args ...interface{}) {
	msg := BroadcastMessage{
		Event: event,
		Room:  room,
		Data:  args,
	}

	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal broadcast message: %v", err)
		return
	}

	ovl.monitorClientsMu.RLock()
	for client := range ovl.monitorClients {
		select {
		case client.sendChan <- jsonMsg:
		default:
			close(client.sendChan)
			delete(ovl.monitorClients, client)
		}
	}
	ovl.monitorClientsMu.RUnlock()
}

// GetWebRoot returns the absolute path to the webroot directory.
func (ovl *Overlord) GetWebRoot() string {
	execPath, err := os.Executable()
	if err != nil {
		log.Fatalln(err)
	}
	execDir := filepath.Dir(execPath)

	webroot, err := filepath.Abs(filepath.Join(execDir, webRootDirName))

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

// GetAppDir returns the overlord application directory.
func (ovl *Overlord) GetAppDir() string {
	appDir, err := filepath.Abs(filepath.Join(ovl.GetWebRoot(), appsDirName))
	if err != nil {
		log.Fatalln(err)
	}
	return appDir
}

// GetAppNames return the name of overlord apps.
func (ovl *Overlord) GetAppNames() ([]string, error) {
	var appNames []string

	apps, err := os.ReadDir(ovl.GetAppDir())
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

type byMid []map[string]interface{}

func (a byMid) Len() int      { return len(a) }
func (a byMid) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byMid) Less(i, j int) bool {
	return a[i]["mid"].(string) < a[j]["mid"].(string)
}

// RegisterHTTPHandlers register handlers for http routes.
func (ovl *Overlord) RegisterHTTPHandlers() {
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  bufferSize,
		WriteBufferSize: bufferSize,
		Subprotocols:    []string{"binary"},
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Helper function for writing error message to WebSocket
	WebSocketSendError := func(ws *websocket.Conn, err string) {
		log.Println(err)
		msg := websocket.FormatCloseMessage(websocket.CloseProtocolError, err)
		ws.WriteMessage(websocket.CloseMessage, msg)
		ws.Close()
	}

	GhostConnectHandler := func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}

		log.Printf("Incoming connection from %s", conn.UnderlyingConn().RemoteAddr())
		cs := NewConnServer(ovl, conn.UnderlyingConn())
		cs.Listen()
	}

	// List all apps available on Overlord.
	AppsListHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		apps, err := ovl.GetAppNames()
		if err != nil {
			w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err)))
		}

		result, err := json.Marshal(map[string][]string{"apps": apps})
		if err != nil {
			w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err)))
		} else {
			w.Write(result)
		}
	}

	// List all agents connected to the Overlord.
	AgentsListHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var data = make([]map[string]interface{}, 0)
		ovl.agentsMu.Lock()
		for _, agent := range ovl.agents {
			data = append(data, map[string]interface{}{
				"mid":        agent.Mid,
				"sid":        agent.Sid,
				"properties": agent.Properties,
			})
		}
		ovl.agentsMu.Unlock()
		sort.Sort(byMid(data))

		result, err := json.Marshal(data)
		if err != nil {
			w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err)))
		} else {
			w.Write(result)
		}
	}

	// Agent upgrade request handler.
	AgentsUpgradeHandler := func(w http.ResponseWriter, r *http.Request) {
		ovl.agentsMu.Lock()
		for _, agent := range ovl.agents {
			err := agent.SendUpgradeRequest()
			if err != nil {
				w.Write([]byte(fmt.Sprintf("Failed to send upgrade request for `%s'.\n",
					agent.Mid)))
			}
		}
		ovl.agentsMu.Unlock()
		w.Write([]byte(`{"status": "success"}`))
	}

	// List all logcat clients connected to the Overlord.
	LogcatsListHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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

		result, err := json.Marshal(data)
		if err != nil {
			w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err)))
		} else {
			w.Write(result)
		}
	}

	// Logcat request handler.
	// We directly send the WebSocket connection to ConnServer for forwarding
	// the log stream.
	LogcatHandler := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Logcat request from %s\n", r.RemoteAddr)

		conn, err := upgrader.Upgrade(w, r, nil)
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

	// TTY stream request handler.
	// We first create a webSocketContext to store the connection, then send a
	// command to Overlord to client to spawn a terminal connection.
	AgentTtyHandler := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Terminal request from %s\n", r.RemoteAddr)

		// Upgrade the connection to WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
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

		ovl.agentsMu.Lock()
		if agent, ok := ovl.agents[mid]; ok {
			ovl.agentsMu.Unlock()
			wc := newWebsocketContext(conn)
			ovl.AddWebsocketContext(wc)
			agent.Command <- SpawnTerminalCmd{wc.Sid, ttyDevice}
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

	// Shell command request handler.
	// We first create a webSocketContext to store the connection, then send a
	// command to ConnServer to client to spawn a shell connection.
	ModeShellHandler := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Shell request from %s\n", r.RemoteAddr)

		// Upgrade the connection to WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}

		vars := mux.Vars(r)
		mid := vars["mid"]
		command := r.URL.Query().Get("command")

		ovl.agentsMu.Lock()
		if agent, ok := ovl.agents[mid]; ok {
			ovl.agentsMu.Unlock()
			wc := newWebsocketContext(conn)
			ovl.AddWebsocketContext(wc)
			agent.Command <- SpawnShellCmd{wc.Sid, command}
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

	// Get agent properties as JSON.
	AgentPropertiesHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		vars := mux.Vars(r)
		mid := vars["mid"]
		ovl.agentsMu.Lock()
		if agent, ok := ovl.agents[mid]; ok {
			ovl.agentsMu.Unlock()
			jsonResult, err := json.Marshal(agent.Properties)
			if err != nil {
				w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err)))
				return
			}
			w.Write(jsonResult)
		} else {
			ovl.agentsMu.Unlock()
			w.Write([]byte(fmt.Sprintf(`{"error": "No client with mid %s"}`, mid)))
		}
	}

	// Helper function for serving file and write it into response body.
	serveFileHTTP := func(w http.ResponseWriter, c *ConnServer) {
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
	AgentDownloadHandler := func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		mid := vars["mid"]

		var agent *ConnServer
		var ok bool

		ovl.agentsMu.Lock()
		if agent, ok = ovl.agents[mid]; !ok {
			ovl.agentsMu.Unlock()
			http.NotFound(w, r)
			return
		}
		ovl.agentsMu.Unlock()

		var filename []string
		if filename, ok = r.URL.Query()["filename"]; !ok {
			http.NotFound(w, r)
			return
		}

		sid := uuid.NewV4().String()
		agent.Command <- SpawnFileCmd{
			Sid: sid, Action: "download", Filename: filename[0]}

		res := <-agent.Response
		if res.Status == Failed {
			http.Error(w, string(res.Payload), http.StatusBadRequest)
			return
		}

		var c *ConnServer
		const maxTries = 100 // 20 seconds
		count := 0

		// Wait until download client connects
		for {
			if count++; count == maxTries {
				http.NotFound(w, r)
				return
			}
			ovl.downloadsMu.Lock()
			if c, ok = ovl.downloads[sid]; ok {
				ovl.downloadsMu.Unlock()
				break
			}
			ovl.downloadsMu.Unlock()
			time.Sleep(200 * time.Millisecond)
		}
		serveFileHTTP(w, c)
	}

	// Passive file download request handler.
	// This handler deal with requests that are initiated by the client. We
	// simply check if the session id exists in the download client list, than
	// start to download the file if it does.
	FileDownloadHandler := func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		sid := vars["sid"]

		var c *ConnServer
		var ok bool

		ovl.downloadsMu.Lock()
		if c, ok = ovl.downloads[sid]; !ok {
			ovl.downloadsMu.Unlock()
			http.NotFound(w, r)
			return
		}
		ovl.downloadsMu.Unlock()
		serveFileHTTP(w, c)
	}

	// File upload request handler.
	AgentUploadHandler := func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		var err error
		var agent *ConnServer
		var errMsg string

		defer func() {
			if errMsg != "" {
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, errMsg),
					http.StatusBadRequest)
			}
		}()

		vars := mux.Vars(r)
		mid := vars["mid"]
		ovl.agentsMu.Lock()
		if agent, ok = ovl.agents[mid]; !ok {
			ovl.agentsMu.Unlock()
			errMsg = fmt.Sprintf("No client with mid %s", mid)
			return
		}
		ovl.agentsMu.Unlock()

		// Target terminal session ID
		var terminalSids []string
		if terminalSids, ok = r.URL.Query()["terminal_sid"]; !ok {
			terminalSids = []string{""}
		}
		sid := uuid.NewV4().String()

		// Upload destination
		var dsts []string
		if dsts, ok = r.URL.Query()["dest"]; !ok {
			dsts = []string{""}
		}

		// Upload destination
		var perm int64
		if perms, ok := r.URL.Query()["perm"]; ok {
			if perm, err = strconv.ParseInt(perms[0], 8, 32); err != nil {
				errMsg = err.Error()
				return
			}
		}

		// Check only
		if r.Method == "GET" {
			// If we are checking only, we need a extra filename parameters since
			// we don't have a form to supply the filename.
			var filenames []string
			if filenames, ok = r.URL.Query()["filename"]; !ok {
				filenames = []string{""}
			}

			agent.Command <- SpawnFileCmd{sid, terminalSids[0], "upload",
				filenames[0], dsts[0], int(perm), true}

			res := <-agent.Response
			if res.Status == Failed {
				errMsg = string(res.Payload)
				return
			}
		}

		mr, err := r.MultipartReader()
		if err != nil {
			errMsg = err.Error()
			return
		}

		p, err := mr.NextPart()
		if err != nil {
			errMsg = err.Error()
			return
		}

		agent.Command <- SpawnFileCmd{sid, terminalSids[0], "upload",
			p.FileName(), dsts[0], int(perm), false}

		res := <-agent.Response
		if res.Status == Failed {
			http.Error(w, string(res.Payload), http.StatusBadRequest)
			return
		}

		const maxTries = 100 // 20 seconds
		count := 0

		// Wait until upload client connects
		var c *ConnServer
		for {
			if count++; count == maxTries {
				http.Error(w, "no response from client", http.StatusInternalServerError)
				return
			}
			ovl.uploadsMu.Lock()
			if c, ok = ovl.uploads[sid]; ok {
				ovl.uploadsMu.Unlock()
				break
			}
			ovl.uploadsMu.Unlock()
			time.Sleep(200 * time.Millisecond)
		}

		_, err = io.Copy(c.Conn, p)
		c.StopListen()

		if err != nil {
			errMsg = err.Error()
			return
		}
		w.Write([]byte(`{"status": "success"}`))
		return
	}

	// Port forwarding request handler
	AgentModeForwardHandler := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("ModeForward request from %s\n", r.RemoteAddr)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}

		var host string = defaultForwardHost
		var port int

		vars := mux.Vars(r)
		mid := vars["mid"]
		// default thost to 127.0.0.1 if not specified
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
			ovl.AddWebsocketContext(wc)
			agent.Command <- SpawnModeForwarderCmd{wc.Sid, host, port}
			if res := <-agent.Response; res.Status == Failed {
				WebSocketSendError(conn, string(res.Payload))
			}
		} else {
			ovl.agentsMu.Unlock()
			WebSocketSendError(conn, "No client with mid "+mid)
		}
	}

	// Directory listing handler
	AgentLsTreeHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		vars := mux.Vars(r)
		mid := vars["mid"]

		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, `{"error": "Path parameter is required"}`, http.StatusBadRequest)
			return
		}

		ovl.agentsMu.Lock()
		agent, ok := ovl.agents[mid]
		ovl.agentsMu.Unlock()
		if !ok {
			http.Error(w, fmt.Sprintf(`{"error": "No client with mid %s"}`, mid), http.StatusNotFound)
			return
		}

		agent.Command <- ListTreeCmd{Path: path}
		res := <-agent.Response
		if res.Status == Failed {
			http.Error(w, string(res.Payload), http.StatusInternalServerError)
			return
		}
		w.Write(res.Payload)
	}

	// File stat handler
	AgentFstatHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		vars := mux.Vars(r)
		mid := vars["mid"]

		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, `{"error": "Path parameter is required"}`, http.StatusBadRequest)
			return
		}

		ovl.agentsMu.Lock()
		agent, ok := ovl.agents[mid]
		ovl.agentsMu.Unlock()
		if !ok {
			http.Error(w, fmt.Sprintf(`{"error": "No client with mid %s"}`, mid), http.StatusNotFound)
			return
		}

		agent.Command <- FstatCmd{Path: path}
		res := <-agent.Response
		if res.Status == Failed {
			http.Error(w, string(res.Payload), http.StatusInternalServerError)
			return
		}
		w.Write(res.Payload)
	}

	// WebSocket monitor endpoint
	MonitorHandler := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Monitor request from %s\n", r.RemoteAddr)

		// Upgrade the connection to WebSocket
		conn, err := websocket.Upgrade(w, r, nil, 1024, 1024)
		if err != nil {
			log.Printf("Failed to upgrade connection: %v", err)
			return
		}

		client := &WebSocketClient{
			conn:     conn,
			sendChan: make(chan []byte, 256),
		}

		ovl.monitorClientsMu.Lock()
		ovl.monitorClients[client] = true
		ovl.monitorClientsMu.Unlock()

		// Start client read/write pumps
		go client.writePump()
		go client.readPump(ovl)
	}
	webRootDir := ovl.GetWebRoot()

	// JWT Auth
	jwtConfig := &JWTConfig{
		SecretPath:   ovl.jwtSecretPath,
		HtpasswdPath: ovl.htpasswdPath,
	}
	jwtAuth, err := NewJWTAuth(jwtConfig)
	if err != nil {
		log.Fatalf("Failed to initialize JWT authentication: %v", err)
	}

	http.HandleFunc("/connect", GhostConnectHandler)

	// Create a single router for all API endpoints
	apiRouter := mux.NewRouter()

	// Add login endpoint for JWT authentication (public)
	apiRouter.HandleFunc("/api/auth/login", jwtAuth.Login).Methods("POST")

	// Protected endpoints
	apiRouter.HandleFunc("/api/apps/list", AppsListHandler)
	apiRouter.HandleFunc("/api/agents/list", AgentsListHandler)
	apiRouter.HandleFunc("/api/agents/upgrade", AgentsUpgradeHandler)
	apiRouter.HandleFunc("/api/logcats/list", LogcatsListHandler)

	// Logcat methods
	apiRouter.HandleFunc("/api/log/{mid}/{sid}", LogcatHandler)

	// Agent methods
	apiRouter.HandleFunc("/api/agent/tty/{mid}", AgentTtyHandler)
	apiRouter.HandleFunc("/api/agent/shell/{mid}", ModeShellHandler)
	apiRouter.HandleFunc("/api/agent/properties/{mid}", AgentPropertiesHandler)
	apiRouter.HandleFunc("/api/agent/lstree/{mid}", AgentLsTreeHandler)
	apiRouter.HandleFunc("/api/agent/fstat/{mid}", AgentFstatHandler)
	apiRouter.HandleFunc("/api/agent/download/{mid}", AgentDownloadHandler)
	apiRouter.HandleFunc("/api/agent/upload/{mid}", AgentUploadHandler)
	apiRouter.HandleFunc("/api/agent/forward/{mid}", AgentModeForwardHandler)

	// File methods
	apiRouter.HandleFunc("/api/file/download/{sid}", FileDownloadHandler)

	// Monitor
	apiRouter.HandleFunc("/api/monitor", MonitorHandler)

	// Create a middleware that conditionally applies authentication based on the path
	conditionalAuthMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip authentication for login endpoint only
			if strings.HasPrefix(r.URL.Path, "/api/auth/login") {
				log.Printf("Auth: Skipping authentication for login endpoint: %s", r.URL.Path)
				next.ServeHTTP(w, r)
				return
			}

			// Apply JWT authentication for all other API endpoints
			jwtAuth.Middleware(next).ServeHTTP(w, r)
		})
	}

	// Register the API router with conditional authentication
	http.Handle("/api/", conditionalAuthMiddleware(apiRouter))

	// /upgrade/ does not need authentication
	http.Handle("/", http.StripPrefix("/",
		http.FileServer(http.Dir(webRootDir))))
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
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
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
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
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

	ticker := time.NewTicker(time.Duration(ldInterval * time.Second))

	for {
		select {
		case <-ticker.C:
			conn.Write([]byte(fmt.Sprintf("OVERLORD %s:%d", ifaceIP, ovl.port)))
		}
	}
}

// Serv is the main routine for starting all the overlord sub-server.
func (ovl *Overlord) Serv() {
	ovl.RegisterHTTPHandlers()
	go ovl.ServHTTP()
	if ovl.lanDisc {
		go ovl.StartUDPBroadcast(OverlordLDPort)
	}

	ticker := time.NewTicker(time.Duration(60 * time.Second))

	for {
		select {
		case <-ticker.C:
			log.Printf("#Goroutines, #Ghostclient: %d, %d\n",
				runtime.NumGoroutine(), len(ovl.agents))
		}
	}
}

// StartOverlord starts the overlord server.
func StartOverlord(bindAddr string, port int, lanDiscInterface string, lanDisc bool,
	certsString string, linkTLS bool, htpasswdPath string, jwtSecretPath string) {
	ovl := NewOverlord(bindAddr, port, lanDiscInterface, lanDisc, certsString, linkTLS, htpasswdPath, jwtSecretPath)
	ovl.Serv()
}
