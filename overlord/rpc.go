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
	debugRPC              = false
	messageSeparator      = "\r\n"
	bufferSize            = 8192
	requestTimeoutSeconds = 60              // Number of seconds before request timeouts
	timeoutCheckInterval  = 3 * time.Second // The time between checking for timeout
)

// Message is the interface which defines a sendable message.
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

// NewRequest creats a new Request object.
// name is the name of the request.
// params is map between string and any other JSON-serializable data structure.
func NewRequest(name string, params map[string]interface{}) *Request {
	req := &Request{
		Rid:     uuid.Must(uuid.NewV4()).String(),
		Timeout: requestTimeoutSeconds,
		Name:    name,
	}
	if targs, err := json.Marshal(params); err != nil {
		panic(err)
	} else {
		req.Params = json.RawMessage(targs)
	}
	return req
}

// SetTimeout sets the timeout of request.
// The default timeout is is defined in requestTimeoutSeconds.
func (r *Request) SetTimeout(timeout int64) {
	r.Timeout = timeout
}

// Marshal mashels the Request.
func (r *Request) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

// Response Object.
// Implements the Message interface.
type Response struct {
	Rid      string          `json:"rid"`
	Response string          `json:"response"`
	Params   json.RawMessage `json:"params"`
}

// NewResponse creates a new Response object.
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

// Marshal marshals the Response.
func (r *Response) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

// ResponseHandler is the function type of the response handler.
// if res is nil, means that the response timeout.
type ResponseHandler func(res *Response) error

// Responder is The structure that stores the response handler information.
type Responder struct {
	RequestTime int64           // Time of request
	Timeout     int64           // Timeout in seconds
	Handler     ResponseHandler // The corresponding request handler
}

// RPCCore is the core implementation of the TCP-based 2-way RPC protocol.
type RPCCore struct {
	Conn       net.Conn             // handle to the TCP connection
	ReadBuffer string               // internal read buffer
	responders map[string]Responder // response handlers
}

// NewRPCCore creates the RPCCore object.
func NewRPCCore(conn net.Conn) *RPCCore {
	return &RPCCore{Conn: conn, responders: make(map[string]Responder)}
}

// SendMessage sends a message.
func (rpc *RPCCore) SendMessage(msg Message) error {
	if rpc.Conn == nil {
		return errors.New("SendMessage failed, connection not established")
	}
	var err error
	var msgBytes []byte

	if msgBytes, err = msg.Marshal(); err == nil {
		if debugRPC {
			log.Printf("-----> %s\n", string(msgBytes))
		}
		_, err = rpc.Conn.Write(append(msgBytes, []byte(messageSeparator)...))
	}
	return err
}

// SendRequest sends a Request.
func (rpc *RPCCore) SendRequest(req *Request, handler ResponseHandler) error {
	err := rpc.SendMessage(req)
	if err == nil && req.Timeout >= 0 {
		res := Responder{time.Now().Unix(), req.Timeout, handler}
		rpc.responders[req.Rid] = res
	}
	return err
}

// SendResponse sends a Response.
func (rpc *RPCCore) SendResponse(res *Response) error {
	return rpc.SendMessage(res)
}

func (rpc *RPCCore) handleResponse(res *Response) error {
	defer delete(rpc.responders, res.Rid)

	if responder, ok := rpc.responders[res.Rid]; ok {
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

// SpawnReaderRoutine spawnes a goroutine that actively read from the socket.
// This function returns two channels. The first one is the channel that
// send the content from the socket, and the second channel send an error
// object if there is one.
func (rpc *RPCCore) SpawnReaderRoutine() (chan []byte, chan error) {
	readChan := make(chan []byte, 1)
	readErrChan := make(chan error, 1)

	go func() {
		for {
			buf := make([]byte, bufferSize)
			n, err := rpc.Conn.Read(buf)
			if err != nil {
				readErrChan <- err
				return
			}
			readChan <- buf[:n]
		}
	}()

	return readChan, readErrChan
}

// ParseMessage parses a single JSON string into a Message object.
func (rpc *RPCCore) ParseMessage(msgJSON string) (Message, error) {
	var req Request
	var res Response

	err := json.Unmarshal([]byte(msgJSON), &req)
	if err != nil || len(req.Name) == 0 {
		err := json.Unmarshal([]byte(msgJSON), &res)
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

// ParseRequests parses a buffer from SpawnReaderRoutine into Request objects.
// The response message is automatically handled by the RPCCore itrpc by
// invoking the corresponding response handler.
func (rpc *RPCCore) ParseRequests(buffer string, single bool) []*Request {
	var reqs []*Request
	var msgsJSON []string

	rpc.ReadBuffer += buffer
	if single {
		idx := strings.Index(rpc.ReadBuffer, messageSeparator)
		if idx == -1 {
			return nil
		}
		msgsJSON = []string{rpc.ReadBuffer[:idx]}
		rpc.ReadBuffer = rpc.ReadBuffer[idx+2:]
	} else {
		msgs := strings.Split(rpc.ReadBuffer, messageSeparator)
		if len(msgs) == 1 {
			return nil
		}
		rpc.ReadBuffer = msgs[len(msgs)-1]
		msgsJSON = msgs[:len(msgs)-1]
	}

	for _, msgJSON := range msgsJSON {
		if debugRPC {
			log.Printf("<----- " + msgJSON)
		}
		if msg, err := rpc.ParseMessage(msgJSON); err != nil {
			log.Printf("Message parse failed: %s\n", err)
			continue
		} else {
			switch m := msg.(type) {
			case *Request:
				reqs = append(reqs, m)
			case *Response:
				err := rpc.handleResponse(m)
				if err != nil {
					log.Printf("Response error: %s\n", err)
				}
			}
		}
	}
	return reqs
}

// ScanForTimeoutRequests scans for timeout requests.
func (rpc *RPCCore) ScanForTimeoutRequests() error {
	for rid, res := range rpc.responders {
		if time.Now().Unix()-res.RequestTime > res.Timeout {
			if res.Handler != nil {
				if err := res.Handler(nil); err != nil {
					delete(rpc.responders, rid)
					return err
				}
			} else {
				log.Printf("Request %s timeout\n", rid)
			}
			delete(rpc.responders, rid)
		}
	}
	return nil
}

// ClearRequests clear all the requests.
func (rpc *RPCCore) ClearRequests() {
	rpc.responders = make(map[string]Responder)
}
