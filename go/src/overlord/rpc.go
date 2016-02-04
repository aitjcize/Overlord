// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"encoding/json"
	"errors"
	"github.com/satori/go.uuid"
	"log"
	"net"
	"strings"
	"time"
)

const (
	DEBUG_RPC            = false
	SEP                  = "\r\n"
	BUFSIZ               = 8192
	REQUEST_TIMEOUT_SECS = 60 // Number of seconds before request timeouts
	TIMEOUT_CHECK_SECS   = 3  // The time between checking for timeout
)

// The interface which defines a sendable message.
type Message interface {
	Marshal() ([]byte, error)
}

// Request Object.
// Implements the Message interface.
// If Timeout < 0, then the response can be omitted.
type Request struct {
	Rid     string          `json:"rid"`
	Timeout int64           `json:"timeout"`
	Name    string          `json:"name"`
	Params  json.RawMessage `json:"params"`
}

// Create a new Request object.
// name is the name of the request.
// params is map between string and any other JSON-serializable data structure.
func NewRequest(name string, params map[string]interface{}) *Request {
	req := &Request{
		Rid:     uuid.NewV4().String(),
		Timeout: REQUEST_TIMEOUT_SECS,
		Name:    name,
	}
	if targs, err := json.Marshal(params); err != nil {
		panic(err)
	} else {
		req.Params = json.RawMessage(targs)
	}
	return req
}

// Set the timeout of request.
// The default timeout is is defined in REQUEST_TIMEOUT_SECS.
func (self *Request) SetTimeout(timeout int64) {
	self.Timeout = timeout
}

func (self *Request) Marshal() ([]byte, error) {
	return json.Marshal(self)
}

// Response Object.
// Implements the Message interface.
type Response struct {
	Rid      string          `json:"rid"`
	Response string          `json:"response"`
	Params   json.RawMessage `json:"params"`
}

// Create a new Response object.
// rid is the request ID of the request this response is intended for.
// response is the reponse status text.
// params is map between string and any other JSON-serializable data structure.
func NewResponse(rid, response string, params map[string]interface{}) *Response {
	res := &Response{
		Rid:      rid,
		Response: response,
	}
	if targs, err := json.Marshal(params); err != nil {
		panic(err)
	} else {
		res.Params = json.RawMessage(targs)
	}
	return res
}

func (self *Response) Marshal() ([]byte, error) {
	return json.Marshal(self)
}

// The function type of the response handler.
// if res is nil, means that the response timeout.
type ResponseHandler func(res *Response) error

// The structure that stores the response handler information.
type Responder struct {
	RequestTime int64           // Time of request
	Timeout     int64           // Timeout in seconds
	Handler     ResponseHandler // The corresponding request handler
}

// RPCCore is the core implementation of the TCP-based 2-way RPC protocol.
type RPCCore struct {
	Conn       *BufferedConn        // handle to the TCP connection
	responders map[string]Responder // response handlers
}

func NewRPCCore(conn net.Conn) *RPCCore {
	return &RPCCore{Conn: NewBufferedConn(conn),
		responders: make(map[string]Responder)}
}

func (self *RPCCore) SendMessage(msg Message) error {
	if self.Conn == nil {
		return errors.New("SendMessage failed, connection not established")
	}
	var err error
	var msgBytes []byte

	if msgBytes, err = msg.Marshal(); err == nil {
		if DEBUG_RPC {
			log.Printf("-----> %s\n", string(msgBytes))
		}
		_, err = self.Conn.Write(append(msgBytes, []byte(SEP)...))
	}
	return err
}

func (self *RPCCore) SendRequest(req *Request, handler ResponseHandler) error {
	err := self.SendMessage(req)
	if err == nil && req.Timeout >= 0 {
		res := Responder{time.Now().Unix(), req.Timeout, handler}
		self.responders[req.Rid] = res
	}
	return err
}

func (self *RPCCore) SendResponse(res *Response) error {
	return self.SendMessage(res)
}

func (self *RPCCore) handleResponse(res *Response) error {
	defer delete(self.responders, res.Rid)

	if responder, ok := self.responders[res.Rid]; ok {
		if responder.Handler != nil {
			if err := responder.Handler(res); err != nil {
				return err
			}
		}
	} else {
		return errors.New("Received unsolicited response, ignored")
	}
	return nil
}

// Spawnes a goroutine that actively read from the socket.
// This function returns two channels. The first one is the channel that
// send the content from the socket, and the second channel send an error
// object if there is one.
func (self *RPCCore) SpawnReaderRoutine() (chan []byte, chan error) {
	readChan := make(chan []byte)
	readErrChan := make(chan error, 1)

	go func() {
		for {
			buf := make([]byte, BUFSIZ)
			n, err := self.Conn.Read(buf)
			readChan <- buf[:n]
			if err != nil {
				if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					continue
				}
				readErrChan <- err
				return
			}
		}
	}()

	return readChan, readErrChan
}

// Parses a single JSON string into a Message object.
func (self *RPCCore) ParseMessage(msgJson string) (Message, error) {
	var req Request
	var res Response

	err := json.Unmarshal([]byte(msgJson), &req)
	if err != nil || len(req.Name) == 0 {
		err := json.Unmarshal([]byte(msgJson), &res)
		if err != nil {
			err = errors.New("mal-formed JSON request, ignored")
		} else {
			return &res, nil
		}
	} else {
		return &req, nil
	}

	return nil, err
}

// Parses a buffer from SpawnReaderRoutine into Request objects.
// The response message is automatically handled by the RPCCore itself by
// invoking the corresponding response handler.
func (self *RPCCore) ParseRequests(buffer string, single bool) []*Request {
	var reqs []*Request
	var writeback string
	var msgsJson []string

	if single {
		idx := strings.Index(buffer, SEP)
		if idx == -1 {
			self.Conn.UnRead([]byte(buffer))
			return nil
		}
		msgsJson = []string{buffer[:idx]}
		self.Conn.UnRead([]byte(buffer[idx+2:]))
	} else {
		msgs := strings.Split(buffer, SEP)
		self.Conn.UnRead([]byte(msgs[len(msgs)-1]))
		msgsJson = msgs[:len(msgs)-1]
	}

	for _, msgJson := range msgsJson {
		if DEBUG_RPC {
			log.Printf("<----- " + msgJson)
		}
		if msg, err := self.ParseMessage(msgJson); err != nil {
			writeback += msgJson + SEP
			log.Printf("Message parse failed: %s\n", err)
			continue
		} else {
			switch m := msg.(type) {
			case *Request:
				reqs = append(reqs, m)
			case *Response:
				err := self.handleResponse(m)
				if err != nil {
					log.Printf("Response error: %s\n", err)
				}
			}
		}
	}
	return reqs
}

// Scan for timeout requests.
func (self *RPCCore) ScanForTimeoutRequests() error {
	for rid, res := range self.responders {
		if time.Now().Unix()-res.RequestTime > res.Timeout {
			if res.Handler != nil {
				if err := res.Handler(nil); err != nil {
					delete(self.responders, rid)
					return err
				}
			} else {
				log.Printf("Request %s timeout\n", rid)
			}
			delete(self.responders, rid)
		}
	}
	return nil
}

func (self *RPCCore) ClearRequests() {
	self.responders = make(map[string]Responder)
}
