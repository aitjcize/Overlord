// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"code.google.com/p/go-shlex"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"net"
	"time"
)

type RegistrationFailedError error

const (
	LOG_BUFSIZ        = 1024 * 16
	PING_RECV_TIMEOUT = PING_TIMEOUT * 2
)

// Since Shell and Logcat are initiated by Overlord, there is only one observer,
// i.e. the one who requested the connection. On the other hand, Simple-logcat
// could have multiple observers, so we need to broadcast the result to all of
// them.
type ConnServer struct {
	*RPCCore
	ovl         *Overlord         // Overlord handle
	registered  bool              // Whether we are registered or not
	mode        int               // Client mode, see constants.go for definition
	bridge      chan interface{}  // channel for receiving commmand from overlord
	stop_listen chan bool         // Stop the Listen() loop
	cid         string            // Client ID
	mid         string            // Machine ID
	wsconn      *websocket.Conn   // WebSocket for Shell and Logcat
	logFormat   int               // Log format, see constants.go for definition
	slogWsconns []*websocket.Conn // WebSockets for Simple-logcat
	slogHistory string            // Log buffer for logcat
	last_ping   int64             // Last time the client pinged
}

func NewConnServer(ovl *Overlord, conn net.Conn) *ConnServer {
	return &ConnServer{
		RPCCore:     NewRPCCore(conn),
		ovl:         ovl,
		mode:        NONE,
		bridge:      make(chan interface{}),
		stop_listen: make(chan bool, 1),
		registered:  false,
	}
}

func (self *ConnServer) Terminate() {
	if self.registered {
		self.ovl.Unregister(self)
	}
	if self.Conn != nil {
		self.Conn.Close()
	}
	if self.wsconn != nil {
		self.wsconn.Close()
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
func (self *ConnServer) forwardWSTerminalInput() {
	defer func() {
		self.stop_listen <- true
	}()

	for {
		mt, payload, err := self.wsconn.ReadMessage()
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

// Forwards the input from Websocket to TCP socket.
func (self *ConnServer) monitorLogcatWS() {
	defer func() {
		log.Println("WebSocket connection terminated")
		self.stop_listen <- true
	}()

	for {
		_, _, err := self.wsconn.ReadMessage()
		if err != nil {
			return
		}
	}
	return
}

// Forward the PTY output to WebSocket.
func (self *ConnServer) forwardTerminalOutput(buffer string) {
	if self.wsconn == nil {
		self.stop_listen <- true
	}
	self.wsconn.WriteMessage(websocket.TextMessage, B64Encode(buffer))
}

// Forward the logcat output to WebSocket.
func (self *ConnServer) forwardLogcatOutput(buffer string) {
	if self.wsconn == nil {
		self.stop_listen <- true
	}
	self.writeLogToWS(self.wsconn, buffer)
}

// Forward the logcat output to WebSocket.
func (self *ConnServer) forwardSimpleLogcatOutput(buffer string) {
	self.slogHistory += buffer
	if l := len(self.slogHistory); l > LOG_BUFSIZ {
		self.slogHistory = self.slogHistory[l-LOG_BUFSIZ : l]
	}

	var alive_wsconns []*websocket.Conn
	for _, conn := range self.slogWsconns[:] {
		if err := self.writeLogToWS(conn, buffer); err == nil {
			alive_wsconns = append(alive_wsconns, conn)
		} else {
			conn.Close()
		}
	}
	self.slogWsconns = alive_wsconns
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
	case SpawnLogcatCmd:
		self.SpawnLogcat(v.Sid, v.Filename)
	case ConnectLogcatCmd:
		// Write log history to newly joined client
		self.writeLogToWS(v.Conn, self.slogHistory)
		self.slogWsconns = append(self.slogWsconns, v.Conn)
	case ShellCmd:
		self.ExecuteCommand(v)
	}
}

// Main routine for listen to socket messages.
func (self *ConnServer) Listen() {
	var reqs []*Request
	read_chan, read_err_chan := self.SpawnReaderRoutine()
	ticker := time.NewTicker(time.Duration(TIMEOUT_CHECK_SECS * time.Second))

	defer self.Terminate()

	for {
		select {
		case buffer := <-read_chan:
			switch self.mode {
			case TERMINAL:
				self.forwardTerminalOutput(buffer)
			case LOGCAT:
				self.forwardLogcatOutput(buffer)
			case SLOGCAT:
				self.forwardSimpleLogcatOutput(buffer)
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
				switch self.mode {
				case TERMINAL:
					// Start a goroutine to forward the WebSocket Input
					go self.forwardWSTerminalInput()
				case LOGCAT:
					go self.monitorLogcatWS()
				case SLOGCAT:
					// A simple-logcat client does not wait for ACK before sending
					// stream, so we need to forward the remaining content of the buffer
					if self.ReadBuffer != "" {
						self.forwardSimpleLogcatOutput(self.ReadBuffer)
						self.ReadBuffer = ""
					}
				}
			}
		case err := <-read_err_chan:
			if err == io.EOF {
				log.Printf("connection dropped: %s\n", self.mid)
			} else {
				log.Printf("unknown network error for %s: %s\n", self.mid, err.Error())
			}
			return
		case msg := <-self.bridge:
			self.handleOverlordRequest(msg)
		case <-ticker.C:
			if err := self.ScanForTimeoutRequests(); err != nil {
				log.Println(err)
			}

			if self.mode == AGENT && self.last_ping != 0 &&
				time.Now().Unix()-self.last_ping > PING_RECV_TIMEOUT {
				log.Printf("Client %s timeout\n", self.mid)
				return
			}
		case s := <-self.stop_listen:
			if s {
				return
			}
		}
	}
}

// Request handlers

func (self *ConnServer) handlePingRequest(req *Request) error {
	self.last_ping = time.Now().Unix()
	res := NewResponse(req.Rid, "pong", nil)
	return self.SendResponse(res)
}

func (self *ConnServer) handleRegisterRequest(req *Request) error {
	type RequestArgs struct {
		Cid    string `json:"cid"`
		Mid    string `json:"mid"`
		Mode   int    `json:"mode"`
		Format int    `json:"format"`
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
	self.cid = args.Cid
	self.mid = args.Mid
	self.mode = args.Mode
	self.logFormat = args.Format

	self.wsconn, err = self.ovl.Register(self)
	if err != nil {
		return RegistrationFailedError(err)
	}

	self.registered = true
	self.last_ping = time.Now().Unix()
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

// Spawn a remote Logcat connection (a ghost with mode LOGCAT).
// sid is the session ID, which will be used as the client ID of the new ghost.
// filename is the name of the file that you want to log.
func (self *ConnServer) SpawnLogcat(sid string, filename string) {
	handler := func(res *Response) error {
		if res == nil {
			return errors.New("SpawnLogcat: command timeout ")
		}
		if res.Response != SUCCESS {
			return errors.New("SpawnLogcat failed: " + res.Response)
		}
		return nil
	}

	req := NewRequest("logcat", map[string]interface{}{
		"sid": sid, "filename": filename})
	self.SendRequest(req, handler)
}

// Execute a remote command.
// If timeout is 0, the default timeout value REQUEST_TIMEOUT_SECS is used.
func (self *ConnServer) ExecuteCommand(cmd ShellCmd) {
	handler := func(res *Response) error {
		if res == nil {
			return errors.New("ExecuteCommand: timeout")
		}

		cmd.Output <- res.Params
		return nil
	}

	parts, err := shlex.Split(cmd.Command)
	if err != nil {
		cmd.Output <- []byte(`{"output": "", "err_msg": "parse error: ` + err.Error() + `"}`)
		return
	}
	req := NewRequest("shell", map[string]interface{}{
		"cmd":  parts[0],
		"args": parts[1:],
	})

	self.SendRequest(req, handler)
}
