// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/googollee/go-socket.io"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/satori/go.uuid"
	"io"
	"io/ioutil"
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
)

const (
	systemAppDir    = "../share/overlord"
	webServerHost   = "0.0.0.0"
	ldInterval      = 5
	keepAlivePeriod = 1
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
	Port int    // Port to forward
}

// ConnectLogcatCmd is an overlord intend to connect to a logcat session.
type ConnectLogcatCmd struct {
	Conn *websocket.Conn
}

// webSocketContext is used for maintaining the session information of
// WebSocket requests. When requests come from Web Server, we create a new
// WebSocketConext to store the session ID and WebSocket connection. ConnServer
// will request a new terminal connection with the given session ID.
// This way, the ConnServer can retreive the connresponding webSocketContext
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

// Overlord type is the main context for storing the overlord server state.
type Overlord struct {
	lanDiscInterface string                            // Network interface used for broadcasting LAN discovery packet
	noAuth           bool                              // Disable HTTP basic authentication
	certs            *TLSCerts                         // TLS certificate
	disableLinkTLS   bool                              // Disable TLS between ghost and overlord
	agents           map[string]*ConnServer            // Normal ghost agents
	logcats          map[string]map[string]*ConnServer // logcat clients
	wsctxs           map[string]*webSocketContext      // (sid, webSocketContext) mapping
	downloads        map[string]*ConnServer            // Download file agents
	uploads          map[string]*ConnServer            // Upload file agents
	ioserver         *socketio.Server                  // SocketIO server handle
	agentsMu         sync.Mutex                        // Mutex for agents
	logcatsMu        sync.Mutex                        // Mutex for logcats
	wsctxsMu         sync.Mutex                        // Mutex for wsctxs
	downloadsMu      sync.Mutex                        // Mutex for downloads
	uploadsMu        sync.Mutex                        // Mutex for uploads
	ioserverMu       sync.Mutex                        // Mutex for ioserver
}

// NewOverlord creates an Overlord object.
func NewOverlord(lanDiscInterface string, noAuth bool, certsString string,
	disableLinkTLS bool) *Overlord {
	var certs *TLSCerts
	if certsString != "" {
		parts := strings.Split(certsString, ",")
		if len(parts) != 2 {
			log.Fatalf("TLSCerts: invalid TLS certs argument")
		} else {
			certs = &TLSCerts{parts[0], parts[1]}
		}
	}
	return &Overlord{
		lanDiscInterface: lanDiscInterface,
		noAuth:           noAuth,
		certs:            certs,
		disableLinkTLS:   disableLinkTLS,
		agents:           make(map[string]*ConnServer),
		logcats:          make(map[string]map[string]*ConnServer),
		wsctxs:           make(map[string]*webSocketContext),
		downloads:        make(map[string]*ConnServer),
		uploads:          make(map[string]*ConnServer),
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
		ovl.ioserver.BroadcastTo("monitor", "agent joined", string(msg))
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
		ovl.ioserver.BroadcastTo("monitor", "logcat joined", string(msg))
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
		ovl.ioserver.BroadcastTo("monitor", "agent left", string(msg))
		ovl.agentsMu.Lock()
		delete(ovl.agents, conn.Mid)
		ovl.agentsMu.Unlock()
	case ModeLogcat:
		ovl.logcatsMu.Lock()
		if _, ok := ovl.logcats[conn.Mid]; ok {
			ovl.ioserver.BroadcastTo("monitor", "logcat left", string(msg))
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
	ovl.ioserver.BroadcastTo(conn.TerminalSid, "file download", string(conn.Sid))
	ovl.downloadsMu.Lock()
	ovl.downloads[conn.Sid] = conn
	ovl.downloadsMu.Unlock()
}

// RegisterUploadRequest registers a file upload request.
func (ovl *Overlord) RegisterUploadRequest(conn *ConnServer) {
	// Use session ID as upload session ID instead of machine ID, so a machine
	// can have multiple upload at the same time
	ovl.ioserver.BroadcastTo(conn.TerminalSid, "file upload", string(conn.Sid))
	ovl.uploadsMu.Lock()
	ovl.uploads[conn.Sid] = conn
	ovl.uploadsMu.Unlock()
}

// Handle TCP Connection.
func (ovl *Overlord) handleConnection(conn net.Conn) {
	handler := NewConnServer(ovl, conn)
	go handler.Listen()
}

// ServSocket is the socket server main routine.
func (ovl *Overlord) ServSocket(port int) {
	var (
		err       error
		ln        net.Listener
		tlsConfig *tls.Config
	)

	addr := fmt.Sprintf("0.0.0.0:%d", port)

	if !ovl.disableLinkTLS && ovl.certs != nil { // TLS enabled
		cert, err := tls.LoadX509KeyPair(ovl.certs.Cert, ovl.certs.Key)
		if err != nil {
			log.Fatalf("Unable to load TLS cert files: %s\n", err)
		}

		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}

	ln, err = net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Unable to listen at %s: %s\n", addr, err)
	}
	defer ln.Close()

	log.Printf("Overlord started, listening at %s", addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatalln("Unable to accept client:", err)
		}
		log.Printf("Incoming connection from %s\n", conn.RemoteAddr())

		// Set TCP Keep Alive
		tcpConn := conn.(*net.TCPConn)
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(keepAlivePeriod * time.Second)

		if tlsConfig != nil {
			conn = tls.Server(conn, tlsConfig)
		}
		ovl.handleConnection(conn)
	}
}

// InitSocketIOServer initializes the Socket.io server.
func (ovl *Overlord) InitSocketIOServer() {
	server, err := socketio.NewServer(nil)
	if err != nil {
		log.Fatalln(err)
	}

	server.On("connection", func(so socketio.Socket) {
		so.Join("monitor")
	})

	server.On("error", func(so socketio.Socket, err error) {
		log.Println("error:", err)
	})

	// Client initiated subscribtion
	server.On("subscribe", func(so socketio.Socket, name string) {
		so.Join(name)
	})

	// Client initiated unsubscribtion
	server.On("unsubscribe", func(so socketio.Socket, name string) {
		so.Leave(name)
	})

	ovl.ioserver = server
}

// GetAppDir returns the overlord application directory.
func (ovl *Overlord) GetAppDir() string {
	execPath, err := GetExecutablePath()
	if err != nil {
		log.Fatalln(err)
	}
	execDir := filepath.Dir(execPath)

	appDir, err := filepath.Abs(filepath.Join(execDir, "app"))
	if err != nil {
		log.Fatalln(err)
	}

	if _, err := os.Stat(appDir); err != nil {
		// Try system install directory
		appDir, err = filepath.Abs(filepath.Join(execDir, systemAppDir, "app"))
		if err != nil {
			log.Fatalln(err)
		}
		if _, err := os.Stat(appDir); err != nil {
			log.Fatalln("Can not find app directory")
		}
	}
	return appDir
}

// GetAppNames return the name of overlord apps.
func (ovl *Overlord) GetAppNames(ignoreSpecial bool) ([]string, error) {
	var appNames []string

	isSpecial := func(target string) bool {
		for _, name := range []string{"common", "upgrade", "third_party"} {
			if name == target {
				return true
			}
		}
		return false
	}

	apps, err := ioutil.ReadDir(ovl.GetAppDir())
	if err != nil {
		return nil, nil
	}

	for _, app := range apps {
		if !app.IsDir() || (ignoreSpecial && isSpecial(app.Name())) {
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

// ServHTTP is the Web server main routine.
func (ovl *Overlord) ServHTTP(port int) {
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  bufferSize,
		WriteBufferSize: bufferSize,
		Subprotocols: []string{"binary"},
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	appDir := ovl.GetAppDir()

	// Helper function for writing error message to WebSocket
	WebSocketSendError := func(ws *websocket.Conn, err string) {
		log.Println(err)
		msg := websocket.FormatCloseMessage(websocket.CloseProtocolError, err)
		ws.WriteMessage(websocket.CloseMessage, msg)
		ws.Close()
	}

	// List all apps available on Overlord.
	AppsListHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		apps, err := ovl.GetAppNames(true)
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
				"mid": agent.Mid,
				"sid": agent.Sid,
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
			if res := <-agent.Response; res != "" {
				WebSocketSendError(conn, res)
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

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}

		vars := mux.Vars(r)
		mid := vars["mid"]
		ovl.agentsMu.Lock()
		if agent, ok := ovl.agents[mid]; ok {
			ovl.agentsMu.Unlock()
			if command, ok := r.URL.Query()["command"]; ok {
				wc := newWebsocketContext(conn)
				ovl.AddWebsocketContext(wc)
				agent.Command <- SpawnShellCmd{wc.Sid, command[0]}
				if res := <-agent.Response; res != "" {
					WebSocketSendError(conn, res)
				}
			} else {
				WebSocketSendError(conn, "No command specified for shell request "+mid)
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
		if res != "" {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, res),
				http.StatusBadRequest)
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

			errMsg = <-agent.Response
			return
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
		if res != "" {
			http.NotFound(w, r)
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

		var port int

		vars := mux.Vars(r)
		mid := vars["mid"]
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
			agent.Command <- SpawnModeForwarderCmd{wc.Sid, port}
			if res := <-agent.Response; res != "" {
				WebSocketSendError(conn, res)
			}
		} else {
			ovl.agentsMu.Unlock()
			WebSocketSendError(conn, "No client with mid "+mid)
		}
	}

	// HTTP basic auth
	auth := NewBasicAuth("Overlord", filepath.Join(appDir, "overlord.htpasswd"),
		ovl.noAuth)

	// Initialize socket IO server
	ovl.InitSocketIOServer()

	// Register the request handlers and start the WebServer.
	r := mux.NewRouter()

	r.HandleFunc("/api/apps/list", AppsListHandler)
	r.HandleFunc("/api/agents/list", AgentsListHandler)
	r.HandleFunc("/api/agents/upgrade", AgentsUpgradeHandler)
	r.HandleFunc("/api/logcats/list", LogcatsListHandler)

	// Logcat methods
	r.HandleFunc("/api/log/{mid}/{sid}", LogcatHandler)

	// Agent methods
	r.HandleFunc("/api/agent/tty/{mid}", AgentTtyHandler)
	r.HandleFunc("/api/agent/shell/{mid}", ModeShellHandler)
	r.HandleFunc("/api/agent/properties/{mid}", AgentPropertiesHandler)
	r.HandleFunc("/api/agent/download/{mid}", AgentDownloadHandler)
	r.HandleFunc("/api/agent/upload/{mid}", AgentUploadHandler)
	r.HandleFunc("/api/agent/forward/{mid}", AgentModeForwardHandler)

	// File methods
	r.HandleFunc("/api/file/download/{sid}", FileDownloadHandler)

	http.Handle("/api/", auth.WrapHandler(r))
	http.Handle("/api/socket.io/", auth.WrapHandler(ovl.ioserver))

	// /upgrade/ does not need authenticiation
	http.Handle("/upgrade/", http.StripPrefix("/upgrade/",
		http.FileServer(http.Dir(filepath.Join(appDir, "upgrade")))))
	http.Handle("/vendor/", auth.WrapHandler(http.FileServer(
		http.Dir(filepath.Join(appDir, "common")))))
	http.Handle("/", auth.WrapHandler(http.FileServer(
		http.Dir(filepath.Join(appDir, "dashboard")))))

	// Serve all apps
	appNames, err := ovl.GetAppNames(false)
	if err != nil {
		log.Fatalln(err)
	}

	for _, app := range appNames {
		if app == "upgrade" {
			continue
		}
		if app != "common" && app != "third_party" {
			log.Printf("Serving app `%s' ...\n", app)
		}
		prefix := fmt.Sprintf("/%s/", app)
		http.Handle(prefix, http.StripPrefix(prefix,
			auth.WrapHandler(http.FileServer(http.Dir(filepath.Join(appDir, app))))))
	}

	webServerAddr := fmt.Sprintf("%s:%d", webServerHost, port)
	if ovl.certs != nil {
		err = http.ListenAndServeTLS(webServerAddr, ovl.certs.Cert,
			ovl.certs.Key, nil)
	} else {
		err = http.ListenAndServe(webServerAddr, nil)
	}
	if err != nil {
		log.Fatalf("net.http could not listen on address '%s': %s\n",
			webServerAddr, err)
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

	conn, err := net.Dial("udp", fmt.Sprintf("%s:%d", bcastIP, port))
	if err != nil {
		log.Fatalln("Unable to start UDP broadcast:", err)
	}

	ticker := time.NewTicker(time.Duration(ldInterval * time.Second))

	for {
		select {
		case <-ticker.C:
			conn.Write([]byte(fmt.Sprintf("OVERLORD %s:%d", ifaceIP, OverlordPort)))
		}
	}
}

// Serv is the main routine for starting all the overlord sub-server.
func (ovl *Overlord) Serv() {
	go ovl.ServSocket(OverlordPort)
	go ovl.ServHTTP(OverlordHTTPPort)
	go ovl.StartUDPBroadcast(OverlordLDPort)

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
func StartOverlord(lanDiscInterface string, noAuth bool, certsString string,
	disableLinkTLS bool) {
	ovl := NewOverlord(lanDiscInterface, noAuth, certsString, disableLinkTLS)
	ovl.Serv()
}
