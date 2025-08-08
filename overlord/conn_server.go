// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// RegistrationFailedError indicates an registration fail error.
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
	Response    chan *Response         // Channel for reponsing overlord command
	Sid         string                 // Session ID
	Mid         string                 // Machine ID
	TerminalSid string                 // Associated terminal session ID
	Properties  map[string]interface{} // Client properties
	ovl         *Overlord              // Overlord handle
	registered  bool                   // Whether we are registered or not
	wsConn      *websocket.Conn        // WebSocket for Terminal and Shell
	logcat      logcatContext          // Logcat context
	Download    fileDownloadContext    // File download context
	stopListen  chan struct{}          // Stop the Listen() loop
	lastPing    int64                  // Last time the client pinged
}

// NewConnServer create a ConnServer object.
func NewConnServer(ovl *Overlord, conn net.Conn) *ConnServer {
	return &ConnServer{
		RPCCore:    NewRPCCore(conn),
		Mode:       ModeNone,
		Command:    make(chan interface{}),
		Response:   make(chan *Response),
		Properties: make(map[string]interface{}),
		ovl:        ovl,
		stopListen: make(chan struct{}),
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
	close(c.stopListen)
}

// Terminate terminats the connection and perform cleanup.
func (c *ConnServer) Terminate() {
	if c.registered {
		c.ovl.Unregister(c)
	}
	c.StopConn()
	if c.wsConn != nil {
		if err := c.wsConn.WriteMessage(websocket.CloseMessage, []byte("")); err != nil {
			log.Printf("Failed to write close message: %v", err)
		}
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
	defer c.StopListen()

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
			if _, err := c.Conn.Write(payload); err != nil {
				log.Printf("Failed to write to connection: %v", err)
			}
		default:
			log.Printf("Invalid message type %d\n", mt)
			return
		}
	}
}

// ModeForward the stream output to WebSocket.
func (c *ConnServer) forwardWSOutput(buf []byte) {
	if c.wsConn == nil {
		c.StopListen()
	}
	if err := c.wsConn.WriteMessage(websocket.BinaryMessage, buf); err != nil {
		log.Printf("Failed to write binary message: %v", err)
	}
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
	case ListTreeCmd:
		c.ListTree(v.Path)
	case FstatCmd:
		c.Fstat(v.Path)
	case CreateSymlinkCmd:
		c.CreateSymlink(v.Target, v.Dest)
	case MkdirCmd:
		c.Mkdir(v.Path, v.Perm)
	case ConnectLogcatCmd:
		// Write log history to newly joined client
		if err := c.writeLogToWS(v.Conn, c.logcat.History); err != nil {
			log.Printf("Failed to write log to WebSocket: %v", err)
		}
		c.logcat.WsConns = append(c.logcat.WsConns, v.Conn)
	case SpawnFileCmd:
		c.SpawnFileServer(v.Sid, v.TerminalSid, v.Action, v.Filename, v.Dest,
			v.Perm)
	case SpawnModeForwarderCmd:
		c.SpawnModeForwarder(v.Sid, v.Host, v.Port)
	}
}

// Listen is the main routine for listen to socket messages.
// Helper functions for ConnServer.Listen
func (c *ConnServer) handleDirectModeOutput(buf []byte) bool {
	// Some modes completely ignore the RPC call, process them.
	switch c.Mode {
	case ModeTerminal, ModeShell, ModeForward:
		c.forwardWSOutput(buf)
		return true
	case ModeLogcat:
		c.forwardLogcatOutput(string(buf))
		return true
	case ModeFile:
		if c.Download.Ready {
			c.forwardFileDownloadData(buf)
			return true
		}
	}
	return false
}

func (c *ConnServer) processIncomingData(buf []byte) error {
	// Only Parse the first message if we are not registered, since
	// if we are in logcat mode, we want to preserve the rest of the
	// data and forward it to the websocket.
	reqs := c.ParseRequests(buf, !c.registered)
	return c.processRequests(reqs)
}

func (c *ConnServer) handleModeTransition() {
	// If c.mode changed, means we just got a registration message and
	// are in a different mode.
	switch c.Mode {
	case ModeTerminal, ModeShell, ModeForward:
		// Start a goroutine to forward the WebSocket Input
		go c.forwardWSInput()
	case ModeLogcat:
		// A logcat client does not wait for ACK before sending
		// stream, so we need to forward the remaining content of the buffer
		if len(c.ReadBuffer) > 0 {
			c.forwardLogcatOutput(string(c.ReadBuffer))
			c.ReadBuffer = nil
		}
	}
}

func (c *ConnServer) handleReadError(err error) bool {
	if err == io.EOF {
		if c.Download.Ready {
			c.Download.Data <- nil
			return true
		}
		log.Printf("connection dropped: %s\n", c.Sid)
	} else {
		log.Printf("unknown network error for %s: %s\n", c.Sid, err)
	}
	return true
}

func (c *ConnServer) handleTimeoutCheck() bool {
	if err := c.ScanForTimeoutRequests(); err != nil {
		log.Println(err)
	}

	if c.Mode == ModeControl && c.lastPing != 0 &&
		time.Now().Unix()-c.lastPing > pingRecvTimeout {
		log.Printf("Client %s timeout\n", c.Mid)
		return true
	}
	return false
}

func (c *ConnServer) Listen() {
	readChan, readErrChan := c.SpawnReaderRoutine()
	ticker := time.NewTicker(timeoutCheckInterval)

	defer c.Terminate()

	for {
		select {
		case buf := <-readChan:
			if c.handleDirectModeOutput(buf) {
				continue
			}

			if err := c.processIncomingData(buf); err != nil {
				if _, ok := err.(RegistrationFailedError); ok {
					log.Printf("%s, abort\n", err)
					return
				}
				log.Println(err)
			}

			c.handleModeTransition()
		case err := <-readErrChan:
			if c.handleReadError(err) {
				return
			}
		case msg := <-c.Command:
			c.handleOverlordRequest(msg)
		case <-ticker.C:
			if c.handleTimeoutCheck() {
				return
			}
		case <-c.stopListen:
			return
		}
	}
}

// Request handlers

func (c *ConnServer) handlePingRequest(req *Request) error {
	c.lastPing = time.Now().Unix()
	return c.SendResponse(NewResponse(req.Rid, Success, nil))
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
	if err := json.Unmarshal(req.Payload, &args); err != nil {
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
		if sendErr := c.SendResponse(NewErrorResponse(req.Rid, err.Error())); sendErr != nil {
			log.Printf("Failed to send error response: %v", sendErr)
		}
		return RegistrationFailedError(errors.New("Register: " + err.Error()))
	}

	// Notify client of our Terminal ssesion ID
	if c.Mode == ModeTerminal && c.wsConn != nil {
		msg, err := json.Marshal(TerminalControl{"sid", c.Sid})
		if err != nil {
			log.Println("handleRegisterRequest: failed to format message")
		} else {
			if err := c.wsConn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("Failed to write text message: %v", err)
			}
		}
	}

	c.registered = true
	c.lastPing = time.Now().Unix()
	return c.SendResponse(NewResponse(req.Rid, Success, nil))
}

func (c *ConnServer) handleDownloadRequest(req *Request) error {
	type RequestArgs struct {
		TerminalSid string `json:"terminal_sid"`
		Filename    string `json:"filename"`
		Size        int64  `json:"size"`
	}

	var args RequestArgs
	if err := json.Unmarshal(req.Payload, &args); err != nil {
		return err
	}

	c.Download.Ready = true
	c.TerminalSid = args.TerminalSid
	c.Download.Name = args.Filename
	c.Download.Size = args.Size

	c.ovl.RegisterDownloadRequest(c)
	return c.SendResponse(NewResponse(req.Rid, Success, nil))
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
			c.Response <- NewErrorResponse(res.Rid, "command timeout")
			return errors.New(name + ": command timeout")
		}

		c.Response <- res
		if res.Status != Success {
			return errors.New(name + " failed: " + res.Status)
		}
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
	if err := c.SendRequest(req, c.getHandler("SpawnTerminal")); err != nil {
		log.Printf("Failed to send terminal request: %v", err)
	}
}

// SpawnShell spawns a shell command connection (a ghost with mode ModeShell).
// sid is the session ID, which will be used as the session ID of the new ghost.
// command is the command to execute.
func (c *ConnServer) SpawnShell(sid string, command string) {
	req := NewRequest("shell", map[string]interface{}{
		"sid": sid, "command": command})
	if err := c.SendRequest(req, c.getHandler("SpawnShell")); err != nil {
		log.Printf("Failed to send shell request: %v", err)
	}
}

// ListTree handles a request to list directory contents recursively
func (c *ConnServer) ListTree(path string) {
	// Create a request to send to the ghost client
	req := NewRequest("list_tree", map[string]interface{}{
		"path": path,
	})
	if err := c.SendRequest(req, c.getHandler("ListTree")); err != nil {
		log.Printf("Failed to send list tree request: %v", err)
	}
}

// Fstat handles a request to get the stat of a file.
func (c *ConnServer) Fstat(path string) {
	req := NewRequest("fstat", map[string]interface{}{
		"path": path,
	})
	if err := c.SendRequest(req, c.getHandler("Fstat")); err != nil {
		log.Printf("Failed to send fstat request: %v", err)
	}
}

// CreateSymlink handles a request to create a symlink.
func (c *ConnServer) CreateSymlink(target, dest string) {
	req := NewRequest("create_symlink", map[string]interface{}{
		"target": target,
		"dest":   dest,
	})
	if err := c.SendRequest(req, c.getHandler("CreateSymlink")); err != nil {
		log.Printf("Failed to send create symlink request: %v", err)
	}
}

// Mkdir handles a request to create a directory.
func (c *ConnServer) Mkdir(path string, perm int) {
	req := NewRequest("mkdir", map[string]interface{}{
		"path": path,
		"perm": perm,
	})
	if err := c.SendRequest(req, c.getHandler("Mkdir")); err != nil {
		log.Printf("Failed to send mkdir request: %v", err)
	}
}

// SpawnFileServer Spawn a remote file connection (a ghost with mode ModeFile).
// action is either 'download' or 'upload'.
// sid is used for uploading file, indicatiting which client's working
// directory to upload to.
func (c *ConnServer) SpawnFileServer(sid, terminalSid, action, filename,
	dest string, perm int) {
	switch action {
	case FileOpDownload:
		req := NewRequest("file_download", map[string]interface{}{
			"sid": sid, "filename": filename})
		if err := c.SendRequest(req, c.getHandler("SpawnFileServer: download")); err != nil {
			log.Printf("Failed to send file download request: %v", err)
		}
	case FileOpUpload:
		req := NewRequest("file_upload", map[string]interface{}{
			"sid": sid, "terminal_sid": terminalSid, "filename": filename,
			"dest": dest, "perm": perm})
		if err := c.SendRequest(req, c.getHandler("SpawnFileServer: upload")); err != nil {
			log.Printf("Failed to send file upload request: %v", err)
		}
	default:
		log.Printf("SpawnFileServer: invalid file action `%s', ignored.\n", action)
	}
}

// SendClearToDownload sends "clear_to_download" request to client to start
// downloading.
func (c *ConnServer) SendClearToDownload() {
	req := NewRequest("clear_to_download", nil)
	req.SetTimeout(-1)
	if err := c.SendRequest(req, nil); err != nil {
		log.Printf("Failed to send clear to download request: %v", err)
	}
}

// SpawnModeForwarder spawns a forwarder connection (a ghost with mode
// ModeForward). sid is the session ID, which will be used as the session ID of
// the new ghost.
func (c *ConnServer) SpawnModeForwarder(sid string, host string, port int) {
	req := NewRequest("forward", map[string]interface{}{
		"sid":  sid,
		"host": host,
		"port": port,
	})
	if err := c.SendRequest(req, c.getHandler("SpawnModeForwarder")); err != nil {
		log.Printf("Failed to send mode forwarder request: %v", err)
	}
}
