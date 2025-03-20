// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net"
	"time"

	uuid "github.com/satori/go.uuid"
)

const (
	debugRPC              = false
	messageSeparatorStr   = "\r\n"
	bufferSize            = 8192
	requestTimeoutSeconds = 60              // Number of seconds before request timeouts
	timeoutCheckInterval  = 3 * time.Second // The time between checking for timeout
)

var messageSeparator = []byte(messageSeparatorStr)

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
	Payload json.RawMessage `json:"payload"`
}

// NewRequest creats a new Request object.
// name is the name of the request.
// params is map between string and any other JSON-serializable data structure.
func NewRequest(name string, params map[string]interface{}) *Request {
	req := &Request{
		Rid:     uuid.NewV4().String(),
		Timeout: requestTimeoutSeconds,
		Name:    name,
	}
	if targs, err := json.Marshal(params); err != nil {
		panic(err)
	} else {
		req.Payload = json.RawMessage(targs)
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

// Error is a structure for storing error information.
type Error struct {
	Error string `json:"error"`
}

// Response Object.
// Implements the Message interface.
type Response struct {
	Rid     string          `json:"rid"`
	Status  string          `json:"status"`
	Payload json.RawMessage `json:"payload"`
}

// NewResponse creates a new Response object.
// rid is the request ID of the request this response is intended for.
// response is the response status text.
// params is map between string and any other JSON-serializable data structure.
func NewResponse(rid, status string, params interface{}) *Response {
	res := &Response{
		Rid:    rid,
		Status: status,
	}
	if targs, err := json.Marshal(params); err != nil {
		panic(err)
	} else {
		res.Payload = json.RawMessage(targs)
	}
	return res
}

// NewErrorResponse creates a new Response object with an error status.
func NewErrorResponse(rid, error string) *Response {
	res := &Response{
		Rid:    rid,
		Status: Failed,
	}
	if targs, err := json.Marshal(Error{Error: error}); err != nil {
		panic(err)
	} else {
		res.Payload = json.RawMessage(targs)
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
	ReadBuffer []byte               // internal read buffer
	responders map[string]Responder // response handlers

	readChan    chan []byte
	readErrChan chan error
}

// NewRPCCore creates the RPCCore object.
func NewRPCCore(conn net.Conn) *RPCCore {
	return &RPCCore{
		Conn:        conn,
		responders:  make(map[string]Responder),
		readChan:    make(chan []byte),
		readErrChan: make(chan error),
	}
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
		_, err = rpc.Conn.Write(append(msgBytes, messageSeparator...))
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

	go func() {
		for {
			buf := make([]byte, bufferSize)
			n, err := rpc.Conn.Read(buf)
			if err != nil {
				rpc.readErrChan <- err
				return
			}
			rpc.readChan <- buf[:n]
		}
	}()

	return rpc.readChan, rpc.readErrChan
}

// StopConn stops the connection and terminates the reader goroutine.
func (rpc *RPCCore) StopConn() {
	rpc.Conn.Close()

	time.Sleep(200 * time.Millisecond)

	// Drain rpc.readChan and rpc.readErrChan so that the reader goroutine can
	// exit.
	for {
		select {
		case <-rpc.readChan:
		case <-rpc.readErrChan:
		default:
			return
		}
	}
}

// ParseMessage parses a single JSON string into a Message object.
func (rpc *RPCCore) ParseMessage(msgJSON string) (Message, error) {
	var req Request
	var res Response

	err := json.Unmarshal([]byte(msgJSON), &req)
	if err != nil || len(req.Name) == 0 {
		err = json.Unmarshal([]byte(msgJSON), &res)
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
// The response message is automatically handled by the RPCCore by invoking the
// corresponding response handler.
func (rpc *RPCCore) ParseRequests(buffer []byte, single bool) []*Request {
	var reqs []*Request
	var msgsJSON [][]byte

	rpc.ReadBuffer = append(rpc.ReadBuffer, buffer...)
	if single {
		idx := bytes.Index(rpc.ReadBuffer, messageSeparator)
		if idx == -1 {
			return nil
		}
		msgsJSON = [][]byte{rpc.ReadBuffer[:idx]}
		rpc.ReadBuffer = rpc.ReadBuffer[idx+len(messageSeparator):]
	} else {
		msgs := bytes.Split(rpc.ReadBuffer, messageSeparator)
		if len(msgs) == 1 {
			return nil
		}
		rpc.ReadBuffer = msgs[len(msgs)-1]
		msgsJSON = msgs[:len(msgs)-1]
	}

	for _, msgJSON := range msgsJSON {
		if debugRPC {
			log.Printf("<----- %s", string(msgJSON))
		}
		if msg, err := rpc.ParseMessage(string(msgJSON)); err != nil {
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
