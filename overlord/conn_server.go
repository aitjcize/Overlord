// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

// RegistrationFailedError indicats an registration fail error.
type RegistrationFailedError error

const (
	logBufferSize   = 1024 * 16
	pingRecvTimeout = pingTimeout * 2
)

// TerminalControl is a JSON struct for storing terminal control messages.
type TerminalControl struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type logcatContext struct {
	Format  int               // Log format, see constants.go
	WsConns []*websocket.Conn // WebSockets for logcat
	History string            // Log buffer for logcat
}

type fileDownloadContext struct {
	Name  string      // Download filename
	Size  int64       // Download filesize
	Data  chan []byte // Channel for download data
	Ready bool        // Ready for download
}

// ConnServer is the main struct for storing connection context between
// Overlord and Ghost.
type ConnServer struct {
	*RPCCore
	Mode        int                    // Client mode, see constants.go
	Command     chan interface{}       // Channel for overlord command
	Response    chan string            // Channel for reponsing overlord command
	Sid         string                 // Session ID
	Mid         string                 // Machine ID
	TerminalSid string                 // Associated terminal session ID
	Properties  map[string]interface{} // Client properties
	ovl         *Overlord              // Overlord handle
	registered  bool                   // Whether we are registered or not
	wsConn      *websocket.Conn        // WebSocket for Terminal and Shell
	logcat      logcatContext          // Logcat context
	Download    fileDownloadContext    // File download context
	stopListen  chan bool              // Stop the Listen() loop
	lastPing    int64                  // Last time the client pinged
}

// NewConnServer create a ConnServer object.
func NewConnServer(ovl *Overlord, conn net.Conn) *ConnServer {
	return &ConnServer{
		RPCCore:    NewRPCCore(conn),
		Mode:       ModeNone,
		Command:    make(chan interface{}),
		Response:   make(chan string),
		Properties: make(map[string]interface{}),
		ovl:        ovl,
		stopListen: make(chan bool, 1),
		registered: false,
		Download:   fileDownloadContext{Data: make(chan []byte)},
	}
}

func (c *ConnServer) setProperties(prop map[string]interface{}) {
	if prop != nil {
		c.Properties = prop
	}

	addr := c.Conn.RemoteAddr().String()
	parts := strings.Split(addr, ":")
	c.Properties["ip"] = strings.Join(parts[:len(parts)-1], ":")
}

// StopListen stops ConnServer's Listen loop.
func (c *ConnServer) StopListen() {
	c.stopListen <- true
}

// Terminate terminats the connection and perform cleanup.
func (c *ConnServer) Terminate() {
	if c.registered {
		c.ovl.Unregister(c)
	}
	if c.Conn != nil {
		c.Conn.Close()
	}
	if c.wsConn != nil {
		c.wsConn.WriteMessage(websocket.CloseMessage, []byte(""))
		c.wsConn.Close()
	}
}

// writeWebsocket is a helper function for written text to websocket in the
// correct format.
func (c *ConnServer) writeLogToWS(conn *websocket.Conn, buf string) error {
	if c.Mode == ModeLogcat && c.logcat.Format == logcatTypeText {
		buf = ToVTNewLine(buf)
	}
	return conn.WriteMessage(websocket.BinaryMessage, []byte(buf))
}

// ModeForwards the input from Websocket to TCP socket.
func (c *ConnServer) forwardWSInput() {
	defer func() {
		c.stopListen <- true
	}()

	for {
		mt, payload, err := c.wsConn.ReadMessage()
		if err != nil {
			if err == io.EOF {
				log.Println("WebSocket connection terminated")
			} else {
				log.Println("Unknown error while reading from WebSocket:", err)
			}
			return
		}

		switch mt {
		case websocket.BinaryMessage, websocket.TextMessage:
			c.Conn.Write(payload)
		default:
			log.Printf("Invalid message type %d\n", mt)
			return
		}
	}
	return
}

// ModeForward the stream output to WebSocket.
func (c *ConnServer) forwardWSOutput(buffer string) {
	if c.wsConn == nil {
		c.stopListen <- true
	}
	c.wsConn.WriteMessage(websocket.BinaryMessage, []byte(buffer))
}

// ModeForward the logcat output to WebSocket.
func (c *ConnServer) forwardLogcatOutput(buffer string) {
	c.logcat.History += buffer
	if l := len(c.logcat.History); l > logBufferSize {
		c.logcat.History = c.logcat.History[l-logBufferSize : l]
	}

	var aliveWsConns []*websocket.Conn
	for _, conn := range c.logcat.WsConns[:] {
		if err := c.writeLogToWS(conn, buffer); err == nil {
			aliveWsConns = append(aliveWsConns, conn)
		} else {
			conn.Close()
		}
	}
	c.logcat.WsConns = aliveWsConns
}

func (c *ConnServer) forwardFileDownloadData(buffer []byte) {
	c.Download.Data <- buffer
}

func (c *ConnServer) processRequests(reqs []*Request) error {
	for _, req := range reqs {
		if err := c.handleRequest(req); err != nil {
			return err
		}
	}
	return nil
}

// Handle the requests from Overlord.
func (c *ConnServer) handleOverlordRequest(obj interface{}) {
	log.Printf("Received %T command from overlord\n", obj)
	switch v := obj.(type) {
	case SpawnTerminalCmd:
		c.SpawnTerminal(v.Sid, v.TtyDevice)
	case SpawnShellCmd:
		c.SpawnShell(v.Sid, v.Command)
	case ConnectLogcatCmd:
		// Write log history to newly joined client
		c.writeLogToWS(v.Conn, c.logcat.History)
		c.logcat.WsConns = append(c.logcat.WsConns, v.Conn)
	case SpawnFileCmd:
		c.SpawnFileServer(v.Sid, v.TerminalSid, v.Action, v.Filename, v.Dest,
			v.Perm, v.CheckOnly)
	case SpawnModeForwarderCmd:
		c.SpawnModeForwarder(v.Sid, v.Port)
	}
}

// Listen is the main routine for listen to socket messages.
func (c *ConnServer) Listen() {
	var reqs []*Request
	readChan, readErrChan := c.SpawnReaderRoutine()
	ticker := time.NewTicker(time.Duration(timeoutCheckInterval))

	defer c.Terminate()

	for {
		select {
		case buf := <-readChan:
			buffer := string(buf)
			// Some modes completely ignore the RPC call, process them.
			switch c.Mode {
			case ModeTerminal, ModeShell, ModeForward:
				c.forwardWSOutput(buffer)
				continue
			case ModeLogcat:
				c.forwardLogcatOutput(buffer)
				continue
			case ModeFile:
				if c.Download.Ready {
					c.forwardFileDownloadData(buf)
					continue
				}
			}

			// Only Parse the first message if we are not registered, since
			// if we are in logcat mode, we want to preserve the rest of the
			// data and forward it to the websocket.
			reqs = c.ParseRequests(buffer, !c.registered)
			if err := c.processRequests(reqs); err != nil {
				if _, ok := err.(RegistrationFailedError); ok {
					log.Printf("%s, abort\n", err)
					return
				}
				log.Println(err)
			}

			// If c.mode changed, means we just got a registration message and
			// are in a different mode.
			switch c.Mode {
			case ModeTerminal, ModeShell, ModeForward:
				// Start a goroutine to forward the WebSocket Input
				go c.forwardWSInput()
			case ModeLogcat:
				// A logcat client does not wait for ACK before sending
				// stream, so we need to forward the remaining content of the buffer
				if c.ReadBuffer != "" {
					c.forwardLogcatOutput(c.ReadBuffer)
					c.ReadBuffer = ""
				}
			}
		case err := <-readErrChan:
			if err == io.EOF {
				if c.Download.Ready {
					c.Download.Data <- nil
					return
				}
				log.Printf("connection dropped: %s\n", c.Sid)
			} else {
				log.Printf("unknown network error for %s: %s\n", c.Sid, err)
			}
			return
		case msg := <-c.Command:
			c.handleOverlordRequest(msg)
		case <-ticker.C:
			if err := c.ScanForTimeoutRequests(); err != nil {
				log.Println(err)
			}

			if c.Mode == ModeControl && c.lastPing != 0 &&
				time.Now().Unix()-c.lastPing > pingRecvTimeout {
				log.Printf("Client %s timeout\n", c.Mid)
				return
			}
		case s := <-c.stopListen:
			if s {
				return
			}
		}
	}
}

// Request handlers

func (c *ConnServer) handlePingRequest(req *Request) error {
	c.lastPing = time.Now().Unix()
	res := NewResponse(req.Rid, "pong", nil)
	return c.SendResponse(res)
}

func (c *ConnServer) handleRegisterRequest(req *Request) error {
	type RequestArgs struct {
		Sid        string                 `json:"sid"`
		Mid        string                 `json:"mid"`
		Mode       int                    `json:"mode"`
		Format     int                    `json:"format"`
		Properties map[string]interface{} `json:"properties"`
	}

	var args RequestArgs
	if err := json.Unmarshal(req.Params, &args); err != nil {
		return err
	}
	if len(args.Mid) == 0 {
		return errors.New("handleRegisterRequest: empty machine ID received")
	}
	if len(args.Sid) == 0 {
		return errors.New("handleRegisterRequest: empty session ID received")
	}

	var err error
	c.Sid = args.Sid
	c.Mid = args.Mid
	c.Mode = args.Mode
	c.logcat.Format = args.Format
	c.setProperties(args.Properties)

	c.wsConn, err = c.ovl.Register(c)
	if err != nil {
		res := NewResponse(req.Rid, err.Error(), nil)
		c.SendResponse(res)
		return RegistrationFailedError(errors.New("Register: " + err.Error()))
	}

	// Notify client of our Terminal ssesion ID
	if c.Mode == ModeTerminal && c.wsConn != nil {
		msg, err := json.Marshal(TerminalControl{"sid", c.Sid})
		if err != nil {
			log.Println("handleRegisterRequest: failed to format message")
		} else {
			c.wsConn.WriteMessage(websocket.TextMessage, msg)
		}
	}

	c.registered = true
	c.lastPing = time.Now().Unix()
	res := NewResponse(req.Rid, Success, nil)
	return c.SendResponse(res)
}

func (c *ConnServer) handleDownloadRequest(req *Request) error {
	type RequestArgs struct {
		TerminalSid string `json:"terminal_sid"`
		Filename    string `json:"filename"`
		Size        int64  `json:"size"`
	}

	var args RequestArgs
	if err := json.Unmarshal(req.Params, &args); err != nil {
		return err
	}

	c.Download.Ready = true
	c.TerminalSid = args.TerminalSid
	c.Download.Name = args.Filename
	c.Download.Size = args.Size

	c.ovl.RegisterDownloadRequest(c)

	res := NewResponse(req.Rid, Success, nil)
	return c.SendResponse(res)
}

func (c *ConnServer) handleClearToUploadRequest(req *Request) error {
	c.ovl.RegisterUploadRequest(c)
	return nil
}

func (c *ConnServer) handleRequest(req *Request) error {
	var err error
	switch req.Name {
	case "ping":
		err = c.handlePingRequest(req)
	case "register":
		err = c.handleRegisterRequest(req)
	case "request_to_download":
		err = c.handleDownloadRequest(req)
	case "clear_to_upload":
		err = c.handleClearToUploadRequest(req)
	}
	return err
}

// SendUpgradeRequest sends upgrade request to clients to trigger an upgrade.
func (c *ConnServer) SendUpgradeRequest() error {
	req := NewRequest("upgrade", nil)
	req.SetTimeout(-1)
	return c.SendRequest(req, nil)
}

// Generic handler for remote command
func (c *ConnServer) getHandler(name string) func(res *Response) error {
	return func(res *Response) error {
		if res == nil {
			c.Response <- "command timeout"
			return errors.New(name + ": command timeout")
		}

		if res.Response != Success {
			c.Response <- res.Response
			return errors.New(name + " failed: " + res.Response)
		}
		c.Response <- ""
		return nil
	}
}

// SpawnTerminal spawns a terminal connection (a ghost with mode ModeTerminal).
// sid is the session ID, which will be used as the session ID of the new ghost.
// ttyDevice is the target terminal device to open. If it's an empty string, a
// pseudo terminal will be open instead.
func (c *ConnServer) SpawnTerminal(sid, ttyDevice string) {
	params := map[string]interface{}{"sid": sid}
	if ttyDevice != "" {
		params["tty_device"] = ttyDevice
	} else {
		params["tty_device"] = nil
	}
	req := NewRequest("terminal", params)
	c.SendRequest(req, c.getHandler("SpawnTerminal"))
}

// SpawnShell spawns a shell command connection (a ghost with mode ModeShell).
// sid is the session ID, which will be used as the session ID of the new ghost.
// command is the command to execute.
func (c *ConnServer) SpawnShell(sid string, command string) {
	req := NewRequest("shell", map[string]interface{}{
		"sid": sid, "command": command})
	c.SendRequest(req, c.getHandler("SpawnShell"))
}

// SpawnFileServer Spawn a remote file connection (a ghost with mode ModeFile).
// action is either 'download' or 'upload'.
// sid is used for uploading file, indicatiting which client's working
// directory to upload to.
func (c *ConnServer) SpawnFileServer(sid, terminalSid, action, filename,
	dest string, perm int, checkOnly bool) {
	if action == "download" {
		req := NewRequest("file_download", map[string]interface{}{
			"sid": sid, "filename": filename})
		c.SendRequest(req, c.getHandler("SpawnFileServer: download"))
	} else if action == "upload" {
		req := NewRequest("file_upload", map[string]interface{}{
			"sid": sid, "terminal_sid": terminalSid, "filename": filename,
			"dest": dest, "perm": perm, "check_only": checkOnly})
		c.SendRequest(req, c.getHandler("SpawnFileServer: upload"))
	} else {
		log.Printf("SpawnFileServer: invalid file action `%s', ignored.\n", action)
	}
}

// SendClearToDownload sends "clear_to_download" request to client to start
// downloading.
func (c *ConnServer) SendClearToDownload() {
	req := NewRequest("clear_to_download", nil)
	req.SetTimeout(-1)
	c.SendRequest(req, nil)
}

// SpawnModeForwarder spawns a forwarder connection (a ghost with mode ModeForward).
// sid is the session ID, which will be used as the session ID of the new ghost.
func (c *ConnServer) SpawnModeForwarder(sid string, port int) {
	req := NewRequest("forward", map[string]interface{}{
		"sid":  sid,
		"port": port,
	})
	c.SendRequest(req, c.getHandler("SpawnModeForwarder"))
}
