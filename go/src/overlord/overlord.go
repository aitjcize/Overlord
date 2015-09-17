// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
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
	"time"
)

const (
	SYSTEM_APP_DIR    = "../share/overlord"
	WEBSERVER_HOST    = "0.0.0.0"
	LD_INTERVAL       = 5
	KEEP_ALIVE_PERIOD = 1
)

type SpawnTerminalCmd struct {
	Sid       string // Session ID
	TtyDevice string // Termainl device to open
}

type SpawnShellCmd struct {
	Sid     string
	Command string
}

type SpawnFileCmd struct {
	Sid         string
	TerminalSid string
	Action      string
	Filename    string
}

type ConnectLogcatCmd struct {
	Conn *websocket.Conn
}

// WebSocketContext is used for maintaining the session information of
// WebSocket requests. When requests come from Web Server, we create a new
// WebSocketConext to store the session ID and WebSocket connection. ConnServer
// will request a new terminal connection with the given session ID.
// This way, the ConnServer can retreive the connresponding WebSocketContext
// with it's the given session ID and get the WebSocket.
type WebSocketContext struct {
	Sid  string
	Conn *websocket.Conn
}

func NewWebsocketContext(conn *websocket.Conn) *WebSocketContext {
	return &WebSocketContext{
		Sid:  uuid.NewV4().String(),
		Conn: conn,
	}
}

type Overlord struct {
	lanDiscInterface  string                            // Network interface used for broadcasting LAN discovery packet
	noAuth            bool                              // Disable HTTP basic authentication
	TLSSettings       string                            // TLS settings in the form of "cert.pem,key.pem". Empty to disable TLS
	agents            map[string]*ConnServer            // Normal ghost agents
	logcats           map[string]map[string]*ConnServer // logcat clients
	wsctxs            map[string]*WebSocketContext      // (sid, WebSocketContext) mapping
	downloads         map[string]*ConnServer            // Download file agents
	uploads           map[string]*ConnServer            // Upload file agents
	ioserver          *socketio.Server
	lastTargetSSHPort int // Last target SSH port suggested to ConnServer
}

func NewOverlord(lanDiscInterface string, noAuth bool, TLSSettings string) *Overlord {
	return &Overlord{
		lanDiscInterface:  lanDiscInterface,
		noAuth:            noAuth,
		TLSSettings:       TLSSettings,
		agents:            make(map[string]*ConnServer),
		logcats:           make(map[string]map[string]*ConnServer),
		wsctxs:            make(map[string]*WebSocketContext),
		downloads:         make(map[string]*ConnServer),
		uploads:           make(map[string]*ConnServer),
		lastTargetSSHPort: TARGET_SSH_PORT_START - 1,
	}
}

// Register a client.
func (self *Overlord) Register(conn *ConnServer) (*websocket.Conn, error) {
	msg, err := json.Marshal(map[string]interface{}{
		"mid": conn.Mid,
		"sid": conn.Sid,
	})
	if err != nil {
		return nil, err
	}

	var wsconn *websocket.Conn

	switch conn.Mode {
	case AGENT:
		if _, ok := self.agents[conn.Mid]; ok {
			return nil, errors.New("duplicate machine ID: " + conn.Mid)
		}

		self.agents[conn.Mid] = conn
		self.ioserver.BroadcastTo("monitor", "agent joined", string(msg))
	case TERMINAL, SHELL:
		if ctx, ok := self.wsctxs[conn.Sid]; !ok {
			return nil, errors.New("client " + conn.Sid + " registered without context")
		} else {
			wsconn = ctx.Conn
		}
	case LOGCAT:
		if _, ok := self.logcats[conn.Mid]; !ok {
			self.logcats[conn.Mid] = make(map[string]*ConnServer)
		}
		if _, ok := self.logcats[conn.Mid][conn.Sid]; ok {
			return nil, errors.New("duplicate session ID: " + conn.Sid)
		}
		self.logcats[conn.Mid][conn.Sid] = conn
		self.ioserver.BroadcastTo("monitor", "logcat joined", string(msg))
	case FILE:
		// Do nothing, we wait until 'request_to_download' call from client to
		// send the message to the browser
	default:
		return nil, errors.New("Unknown client mode")
	}

	var id string
	if conn.Mode == AGENT {
		id = conn.Mid
	} else {
		id = conn.Sid
	}

	log.Printf("%s %s registered\n", ModeStr(conn.Mode), id)

	return wsconn, nil
}

// Unregister a client.
func (self *Overlord) Unregister(conn *ConnServer) {
	msg, err := json.Marshal(map[string]interface{}{
		"mid": conn.Mid,
		"sid": conn.Sid,
	})

	if err != nil {
		panic(err)
	}

	switch conn.Mode {
	case AGENT:
		self.ioserver.BroadcastTo("monitor", "agent left", string(msg))
		delete(self.agents, conn.Mid)
	case LOGCAT:
		if _, ok := self.logcats[conn.Mid]; ok {
			self.ioserver.BroadcastTo("monitor", "logcat left", string(msg))
			delete(self.logcats[conn.Mid], conn.Sid)
			if len(self.logcats[conn.Mid]) == 0 {
				delete(self.logcats, conn.Mid)
			}
		}
	case FILE:
		if _, ok := self.downloads[conn.Sid]; ok {
			delete(self.downloads, conn.Sid)
		}
		if _, ok := self.uploads[conn.Sid]; ok {
			delete(self.uploads, conn.Sid)
		}
	default:
		if _, ok := self.wsctxs[conn.Sid]; ok {
			delete(self.wsctxs, conn.Sid)
		}
	}

	var id string
	if conn.Mode == AGENT {
		id = conn.Mid
	} else {
		id = conn.Sid
	}
	log.Printf("%s %s unregistered\n", ModeStr(conn.Mode), id)
}

func (self *Overlord) AddWebsocketContext(wc *WebSocketContext) {
	self.wsctxs[wc.Sid] = wc
}

// Register a download request clients.
func (self *Overlord) RegisterDownloadRequest(conn *ConnServer) {
	// Use session ID as download session ID instead of machine ID, so a machine
	// can have multiple download at the same time
	self.ioserver.BroadcastTo(conn.TerminalSid, "file download", string(conn.Sid))
	self.downloads[conn.Sid] = conn
}

// Register a upload request clients.
func (self *Overlord) RegisterUploadRequest(conn *ConnServer) {
	// Use session ID as upload session ID instead of machine ID, so a machine
	// can have multiple upload at the same time
	self.ioserver.BroadcastTo(conn.TerminalSid, "file upload", string(conn.Sid))
	self.uploads[conn.Sid] = conn
}

// It's not that important to worry about race conditions here, since
// after all the port is just a suggestion.  If it doesn't work, ConnServer
// will come back and request another one.
func (self *Overlord) SuggestTargetSSHPort() int {
	self.lastTargetSSHPort++
	if self.lastTargetSSHPort > TARGET_SSH_PORT_END {
		self.lastTargetSSHPort = TARGET_SSH_PORT_START - 1
	}
	return self.lastTargetSSHPort
}

// Handle TCP Connection.
func (self *Overlord) handleConnection(conn net.Conn) {
	handler := NewConnServer(self, conn)
	go handler.Listen()
}

// Socket server main routine.
func (self *Overlord) ServSocket(port int) {
	addrStr := fmt.Sprintf("0.0.0.0:%d", port)
	addr, err := net.ResolveTCPAddr("tcp", addrStr)
	if err != nil {
		panic(err)
	}
	ln, err := net.ListenTCP("tcp", addr)
	if err != nil {
		panic(err)
	}
	log.Printf("Overlord started, listening at %s", addr)
	for {
		conn, err := ln.AcceptTCP()
		if err != nil {
			panic(err)
		}
		log.Printf("Incoming connection from %s\n", conn.RemoteAddr())
		conn.SetKeepAlive(true)
		conn.SetKeepAlivePeriod(KEEP_ALIVE_PERIOD * time.Second)
		self.handleConnection(conn)
	}
}

// Initialize the Socket.io server.
func (self *Overlord) InitSocketIOServer() {
	server, err := socketio.NewServer(nil)
	if err != nil {
		log.Fatal(err)
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

	self.ioserver = server
}

func (self *Overlord) GetAppDir() string {
	execPath, err := GetExecutablePath()
	if err != nil {
		log.Fatalf(err.Error())
	}
	execDir := filepath.Dir(execPath)

	appDir, err := filepath.Abs(filepath.Join(execDir, "app"))
	if err != nil {
		log.Fatalf(err.Error())
	}

	if _, err := os.Stat(appDir); err != nil {
		// Try system install directory
		appDir, err = filepath.Abs(filepath.Join(execDir, SYSTEM_APP_DIR, "app"))
		if err != nil {
			log.Fatalf(err.Error())
		}
		if _, err := os.Stat(appDir); err != nil {
			log.Fatalf("Can not find app directory\n")
		}
	}
	return appDir
}

func (self *Overlord) GetAppNames(ignoreSpecial bool) ([]string, error) {
	var appNames []string

	isSpecial := func(target string) bool {
		for _, name := range []string{"common", "index", "upgrade"} {
			if name == target {
				return true
			}
		}
		return false
	}

	apps, err := ioutil.ReadDir(self.GetAppDir())
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

type ByMid []map[string]interface{}

func (a ByMid) Len() int      { return len(a) }
func (a ByMid) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByMid) Less(i, j int) bool {
	return a[i]["mid"].(string) < a[j]["mid"].(string)
}

// Web server main routine.
func (self *Overlord) ServHTTP(port int) {
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  BUFSIZ,
		WriteBufferSize: BUFSIZ,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	appDir := self.GetAppDir()

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

		apps, err := self.GetAppNames(true)
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
		data := make([]map[string]interface{}, len(self.agents))
		idx := 0
		for _, agent := range self.agents {
			data[idx] = map[string]interface{}{
				"mid": agent.Mid,
				"sid": agent.Sid,
			}
			if agent.TargetSSHPort == 0 {
				data[idx]["target_ssh_port"] = nil
			} else {
				data[idx]["target_ssh_port"] = agent.TargetSSHPort
			}
			idx++
		}
		sort.Sort(ByMid(data))

		result, err := json.Marshal(data)
		if err != nil {
			w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err)))
		} else {
			w.Write(result)
		}
	}

	// Agent upgrade request handler.
	AgentsUpgradeHandler := func(w http.ResponseWriter, r *http.Request) {
		for _, agent := range self.agents {
			err := agent.SendUpgradeRequest()
			if err != nil {
				w.Write([]byte(fmt.Sprintf("Failed to send upgrade request for `%s'.\n",
					agent.Mid)))
			}
		}
		w.Write([]byte(`{"status": "success"}`))
	}

	// List all logcat clients connected to the Overlord.
	LogcatsListHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data := make([]map[string]interface{}, len(self.logcats))
		idx := 0
		for mid, logcats := range self.logcats {
			var sids []string
			for sid, _ := range logcats {
				sids = append(sids, sid)
			}
			data[idx] = map[string]interface{}{
				"mid":  mid,
				"sids": sids,
			}
			idx++
		}

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

		if logcats, ok := self.logcats[mid]; ok {
			if logcat, ok := logcats[sid]; ok {
				logcat.Command <- ConnectLogcatCmd{conn}
			} else {
				WebSocketSendError(conn, "No client with sid "+sid)
			}
		} else {
			WebSocketSendError(conn, "No client with mid "+mid)
		}
	}

	// TTY stream request handler.
	// We first create a WebSocketContext to store the connection, then send a
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

		if agent, ok := self.agents[mid]; ok {
			wc := NewWebsocketContext(conn)
			self.AddWebsocketContext(wc)
			agent.Command <- SpawnTerminalCmd{wc.Sid, ttyDevice}
			if res := <-agent.Response; res != "" {
				WebSocketSendError(conn, res)
			}
		} else {
			WebSocketSendError(conn, "No client with mid "+mid)
		}
	}

	// Shell command request handler.
	// We first create a WebSocketContext to store the connection, then send a
	// command to ConnServer to client to spawn a shell connection.
	AgentShellHandler := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Shell request from %s\n", r.RemoteAddr)

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}

		vars := mux.Vars(r)
		mid := vars["mid"]
		if agent, ok := self.agents[mid]; ok {
			if command, ok := r.URL.Query()["command"]; ok {
				wc := NewWebsocketContext(conn)
				self.AddWebsocketContext(wc)
				agent.Command <- SpawnShellCmd{wc.Sid, command[0]}
				if res := <-agent.Response; res != "" {
					WebSocketSendError(conn, res)
				}
			} else {
				WebSocketSendError(conn, "No command specified for shell request "+mid)
			}
		} else {
			WebSocketSendError(conn, "No client with mid "+mid)
		}
	}

	// Get agent properties as JSON.
	AgentPropertiesHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		vars := mux.Vars(r)
		mid := vars["mid"]
		if agent, ok := self.agents[mid]; ok {
			jsonResult, err := json.Marshal(agent.Properties)
			if err != nil {
				w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err)))
				return
			}
			w.Write(jsonResult)
		} else {
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

		if agent, ok = self.agents[mid]; !ok {
			http.NotFound(w, r)
			return
		}

		var filename []string
		if filename, ok = r.URL.Query()["filename"]; !ok {
			http.NotFound(w, r)
			return
		}

		sid := uuid.NewV4().String()
		agent.Command <- SpawnFileCmd{sid, "", "download", filename[0]}

		res := <-agent.Response
		if res != "" {
			http.NotFound(w, r)
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
			if c, ok = self.downloads[sid]; ok {
				break
			}
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

		if c, ok = self.downloads[sid]; !ok {
			http.NotFound(w, r)
			return
		}
		serveFileHTTP(w, c)
	}

	// File upload request handler.
	AgentUploadHandler := func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		var agent *ConnServer
		var errMsg string

		defer func() {
			if errMsg != "" {
				w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, errMsg)))
				http.Error(w, "", http.StatusBadRequest)
			}
		}()

		vars := mux.Vars(r)
		mid := vars["mid"]
		if agent, ok = self.agents[mid]; !ok {
			errMsg = fmt.Sprintf("No client with mid %s", mid)
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

		var sids []string
		if sids, ok = r.URL.Query()["sid"]; !ok {
			sids = []string{""}
		}
		sid := uuid.NewV4().String()
		agent.Command <- SpawnFileCmd{sid, sids[0], "upload", p.FileName()}

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
			if c, ok = self.uploads[sid]; ok {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}

		_, err = io.Copy(c.Conn, p)
		c.StopListen()

		if err != nil {
			w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err.Error())))
			return
		}
		w.Write([]byte(`{"status": "success"}`))
		return
	}

	// HTTP basic auth
	auth := NewBasicAuth("Overlord", filepath.Join(appDir, "overlord.htpasswd"),
		self.noAuth)

	// Initialize socket IO server
	self.InitSocketIOServer()

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
	r.HandleFunc("/api/agent/shell/{mid}", AgentShellHandler)
	r.HandleFunc("/api/agent/properties/{mid}", AgentPropertiesHandler)
	r.HandleFunc("/api/agent/download/{mid}", AgentDownloadHandler)
	r.HandleFunc("/api/agent/upload/{mid}", AgentUploadHandler)

	// File methods
	r.HandleFunc("/api/file/download/{sid}", FileDownloadHandler)

	http.Handle("/api/", auth.WrapHandler(r))
	http.Handle("/api/socket.io/", auth.WrapHandler(self.ioserver))
	http.Handle("/upgrade/", http.StripPrefix("/upgrade/",
		http.FileServer(http.Dir(filepath.Join(appDir, "upgrade")))))
	http.Handle("/vendor/", auth.WrapHandler(http.FileServer(
		http.Dir(filepath.Join(appDir, "common")))))
	http.Handle("/", auth.WrapHandler(http.FileServer(
		http.Dir(filepath.Join(appDir, "dashboard")))))

	// Serve all apps
	appNames, err := self.GetAppNames(false)
	if err != nil {
		panic(err)
	}

	for _, app := range appNames {
		if app == "upgrade" {
			continue
		}
		if app != "common" && app != "index" {
			log.Printf("Serving app `%s' ...\n", app)
		}
		prefix := fmt.Sprintf("/%s/", app)
		http.Handle(prefix, http.StripPrefix(prefix,
			auth.WrapHandler(http.FileServer(http.Dir(filepath.Join(appDir, app))))))
	}

	webServerAddr := fmt.Sprintf("%s:%d", WEBSERVER_HOST, port)

	if self.TLSSettings != "" {
		parts := strings.Split(self.TLSSettings, ",")
		if len(parts) != 2 {
			log.Fatalf("TLSSettings: invalid key assignment")
		}
		err = http.ListenAndServeTLS(webServerAddr, parts[0], parts[1], nil)
	} else {
		err = http.ListenAndServe(webServerAddr, nil)
	}
	if err != nil {
		log.Fatalf("net.http could not listen on address '%s': %s\n",
			webServerAddr, err)
	}
}

// Broadcast LAN discovery message.
func (self *Overlord) StartUDPBroadcast(port int) {
	ifaceIP := ""
	bcastIP := net.IPv4bcast.String()

	if self.lanDiscInterface != "" {
		interfaces, err := net.Interfaces()
		if err != nil {
			panic(err)
		}

	outter:
		for _, iface := range interfaces {
			if iface.Name == self.lanDiscInterface {
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
				self.lanDiscInterface)
		}
	}

	conn, err := net.Dial("udp", fmt.Sprintf("%s:%d", bcastIP, port))
	if err != nil {
		panic(err)
	}

	ticker := time.NewTicker(time.Duration(LD_INTERVAL * time.Second))

	for {
		select {
		case <-ticker.C:
			conn.Write([]byte(fmt.Sprintf("OVERLORD %s:%d", ifaceIP, OVERLORD_PORT)))
		}
	}
}

func (self *Overlord) Serv() {
	go self.ServSocket(OVERLORD_PORT)
	go self.ServHTTP(OVERLORD_HTTP_PORT)
	go self.StartUDPBroadcast(OVERLORD_LD_PORT)

	ticker := time.NewTicker(time.Duration(60 * time.Second))

	for {
		select {
		case <-ticker.C:
			log.Printf("#Goroutines, #Ghostclient: %d, %d\n",
				runtime.NumGoroutine(), len(self.agents))
		}
	}
}

func StartOverlord(lanDiscInterface string, noAuth bool, TLSSettings string) {
	ovl := NewOverlord(lanDiscInterface, noAuth, TLSSettings)
	ovl.Serv()
}
