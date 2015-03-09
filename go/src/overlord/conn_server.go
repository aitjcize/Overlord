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

type RegistrationFailedError error

const (
	LOG_BUFSIZ        = 1024 * 16
	PING_RECV_TIMEOUT = PING_TIMEOUT * 2
)

// Since Shell and Logcat are initiated by Overlord, there is only one observer,
// i.e. the one who requested the connection. On the other hand, logcat
// could have multiple observers, so we need to broadcast the result to all of
// them.
type ConnServer struct {
	*RPCCore
	Mode       int                    // Client mode, see constants.go
	Bridge     chan interface{}       // Channel for overlord commmand
	Cid        string                 // Client ID
	Mid        string                 // Machine ID
	Properties map[string]interface{} // Client properties
	ovl        *Overlord              // Overlord handle
	registered bool                   // Whether we are registered or not
	wsConn     *websocket.Conn        // WebSocket for Shell and Logcat
	logFormat  int                    // Log format, see constants.go
	logWsConns []*websocket.Conn      // WebSockets for logcat
	logHistory string                 // Log buffer for logcat
	stopListen chan bool              // Stop the Listen() loop
	lastPing   int64                  // Last time the client pinged
}

func NewConnServer(ovl *Overlord, conn net.Conn) *ConnServer {
	return &ConnServer{
		RPCCore:    NewRPCCore(conn),
		Mode:       NONE,
		Bridge:     make(chan interface{}),
		Properties: make(map[string]interface{}),
		ovl:        ovl,
		stopListen: make(chan bool, 1),
		registered: false,
	}
}

func (self *ConnServer) SetProperties(prop map[string]interface{}) {
	if prop != nil {
		self.Properties = prop
	}

	addr := self.Conn.RemoteAddr().String()
	parts := strings.Split(addr, ":")
	self.Properties["ip"] = strings.Join(parts[:len(parts)-1], ":")
}

func (self *ConnServer) Terminate() {
	if self.registered {
		self.ovl.Unregister(self)
	}
	if self.Conn != nil {
		self.Conn.Close()
	}
	if self.wsConn != nil {
		self.wsConn.Close()
	}
}

// writeWebsocket is a helper function for written text to websocket in the
// correct format.
func (self *ConnServer) writeLogToWS(conn *websocket.Conn, buf string) error {
	if self.logFormat == TEXT {
		buf = ToVTNewLine(buf)
	}
	return conn.WriteMessage(websocket.TextMessage, B64Encode(buf))
}

// Forwards the input from Websocket to TCP socket.
func (self *ConnServer) forwardWSInput() {
	defer func() {
		self.stopListen <- true
	}()

	for {
		mt, payload, err := self.wsConn.ReadMessage()
		if err != nil {
			if err == io.EOF {
				log.Println("WebSocket connection terminated")
			} else {
				log.Println("Unknown error while reading from WebSocket")
			}
			return
		}

		switch mt {
		case websocket.BinaryMessage:
			log.Printf("Ignoring binary message: %q\n", payload)
		case websocket.TextMessage:
			self.Conn.Write(payload)
		default:
			log.Printf("Invalid message type %d\n", mt)
			return
		}
	}
	return
}

// Forward the PTY output to WebSocket.
func (self *ConnServer) forwardTerminalOutput(buffer string) {
	if self.wsConn == nil {
		self.stopListen <- true
	}
	self.wsConn.WriteMessage(websocket.TextMessage, B64Encode(buffer))
}

// Forward the logcat output to WebSocket.
func (self *ConnServer) forwardShellOutput(buffer string) {
	if self.wsConn == nil {
		self.stopListen <- true
	}
	self.writeLogToWS(self.wsConn, buffer)
}

// Forward the logcat output to WebSocket.
func (self *ConnServer) forwardLogcatOutput(buffer string) {
	self.logHistory += buffer
	if l := len(self.logHistory); l > LOG_BUFSIZ {
		self.logHistory = self.logHistory[l-LOG_BUFSIZ : l]
	}

	var aliveWsConns []*websocket.Conn
	for _, conn := range self.logWsConns[:] {
		if err := self.writeLogToWS(conn, buffer); err == nil {
			aliveWsConns = append(aliveWsConns, conn)
		} else {
			conn.Close()
		}
	}
	self.logWsConns = aliveWsConns
}

func (self *ConnServer) ProcessRequests(reqs []*Request) error {
	for _, req := range reqs {
		if err := self.handleRequest(req); err != nil {
			return err
		}
	}
	return nil
}

// Handle the requests from Overlord.
func (self *ConnServer) handleOverlordRequest(obj interface{}) {
	log.Printf("Received %T command from overlord\n", obj)
	switch v := obj.(type) {
	case SpawnTerminalCmd:
		self.SpawnTerminal(v.Sid)
	case SpawnShellCmd:
		self.SpawnShell(v.Sid, v.Command)
	case ConnectLogcatCmd:
		// Write log history to newly joined client
		self.writeLogToWS(v.Conn, self.logHistory)
		self.logWsConns = append(self.logWsConns, v.Conn)
	}
}

// Main routine for listen to socket messages.
func (self *ConnServer) Listen() {
	var reqs []*Request
	readChan, readErrChan := self.SpawnReaderRoutine()
	ticker := time.NewTicker(time.Duration(TIMEOUT_CHECK_SECS * time.Second))

	defer self.Terminate()

	for {
		select {
		case buffer := <-readChan:
			switch self.Mode {
			case TERMINAL:
				self.forwardTerminalOutput(buffer)
			case SHELL:
				self.forwardShellOutput(buffer)
			case LOGCAT:
				self.forwardLogcatOutput(buffer)
			default:
				// Only Parse the first message if we are not registered, since
				// if we are in logcat mode, we want to preserve the rest of the
				// data and forward it to the websocket.
				reqs = self.ParseRequests(buffer, !self.registered)
				if err := self.ProcessRequests(reqs); err != nil {
					if _, ok := err.(RegistrationFailedError); ok {
						log.Printf("%s, abort", err)
						return
					} else {
						log.Println(err)
					}
				}

				// If self.mode changed, means we just got a registration message and
				// are in a different mode.
				switch self.Mode {
				case TERMINAL, SHELL:
					// Start a goroutine to forward the WebSocket Input
					go self.forwardWSInput()
				case LOGCAT:
					// A logcat client does not wait for ACK before sending
					// stream, so we need to forward the remaining content of the buffer
					if self.ReadBuffer != "" {
						self.forwardLogcatOutput(self.ReadBuffer)
						self.ReadBuffer = ""
					}
				}
			}
		case err := <-readErrChan:
			if err == io.EOF {
				log.Printf("connection dropped: %s\n", self.Mid)
			} else {
				log.Printf("unknown network error for %s: %s\n", self.Mid, err.Error())
			}
			return
		case msg := <-self.Bridge:
			self.handleOverlordRequest(msg)
		case <-ticker.C:
			if err := self.ScanForTimeoutRequests(); err != nil {
				log.Println(err)
			}

			if self.Mode == AGENT && self.lastPing != 0 &&
				time.Now().Unix()-self.lastPing > PING_RECV_TIMEOUT {
				log.Printf("Client %s timeout\n", self.Mid)
				return
			}
		case s := <-self.stopListen:
			if s {
				return
			}
		}
	}
}

// Request handlers

func (self *ConnServer) handlePingRequest(req *Request) error {
	self.lastPing = time.Now().Unix()
	res := NewResponse(req.Rid, "pong", nil)
	return self.SendResponse(res)
}

func (self *ConnServer) handleRegisterRequest(req *Request) error {
	type RequestArgs struct {
		Cid        string                 `json:"cid"`
		Mid        string                 `json:"mid"`
		Mode       int                    `json:"mode"`
		Format     int                    `json:"format"`
		Properties map[string]interface{} `json:"properties"`
	}

	var args RequestArgs
	if err := json.Unmarshal(req.Params, &args); err != nil {
		return err
	} else {
		if len(args.Mid) == 0 {
			return errors.New("handleRegisterRequest: Empty machine ID received")
		}
		if len(args.Cid) == 0 {
			return errors.New("handleRegisterRequest: Empty client ID received")
		}
	}

	var err error
	self.Cid = args.Cid
	self.Mid = args.Mid
	self.Mode = args.Mode
	self.logFormat = args.Format
	self.SetProperties(args.Properties)

	self.wsConn, err = self.ovl.Register(self)
	if err != nil {
		return RegistrationFailedError(err)
	}

	self.registered = true
	self.lastPing = time.Now().Unix()
	res := NewResponse(req.Rid, SUCCESS, nil)
	return self.SendResponse(res)
}

func (self *ConnServer) handleRequest(req *Request) error {
	var err error
	switch req.Name {
	case "ping":
		err = self.handlePingRequest(req)
	case "register":
		err = self.handleRegisterRequest(req)
	}
	return err
}

// Spawn a remote terminal connection (a ghost with mode TERMINAL).
// sid is the session ID, which will be used as the client ID of the new ghost.
func (self *ConnServer) SpawnTerminal(sid string) {
	handler := func(res *Response) error {
		if res == nil {
			return errors.New("SpawnTerminal: command timeout")
		}

		if res.Response != SUCCESS {
			return errors.New("SpawnTerminal failed: " + res.Response)
		}
		return nil
	}

	req := NewRequest("terminal", map[string]interface{}{"sid": sid})
	self.SendRequest(req, handler)
}

// Spawn a remote shell command connection (a ghost with mode SHELL).
// sid is the session ID, which will be used as the client ID of the new ghost.
// command is the command to execute.
func (self *ConnServer) SpawnShell(sid string, command string) {
	handler := func(res *Response) error {
		if res == nil {
			return errors.New("SpawnShell: command timeout ")
		}
		if res.Response != SUCCESS {
			return errors.New("SpawnShell failed: " + res.Response)
		}
		return nil
	}

	req := NewRequest("shell", map[string]interface{}{
		"sid": sid, "command": command})
	self.SendRequest(req, handler)
}
