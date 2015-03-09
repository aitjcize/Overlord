// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"code.google.com/p/go-shlex"
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kr/pty"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	DEFAULT_SHELL  = "/bin/bash"
	DIAL_TIMEOUT   = 3
	OVERLORD_IP    = "localhost"
	PING_INTERVAL  = 10
	PING_TIMEOUT   = 10
	RETRY_INTERVAL = 2
	READ_TIMEOUT   = 3
)

type Ghost struct {
	*RPCCore
	addrs        []string               // List of possible Overlord addresses
	mid          string                 // Machine ID
	cid          string                 // Client ID
	mode         int                    // mode, see constants.go
	properties   map[string]interface{} // Client properties
	reset        bool                   // Whether to reset the connection
	quit         bool                   // Whether to quit the connection
	readChan     chan string            // The incoming data channel
	readErrChan  chan error             // The incoming data error channel
	pauseLanDisc bool                   // Stop LAN discovery
	shellCommand string                 // filename to cat in logcat mode
}

func NewGhost(addrs []string, mode int) *Ghost {
	mid, err := GetMachineID()
	if err != nil {
		panic(err)
	}
	return &Ghost{
		RPCCore:      NewRPCCore(nil),
		addrs:        addrs,
		mid:          mid,
		cid:          uuid.New(),
		mode:         mode,
		properties:   make(map[string]interface{}),
		reset:        false,
		quit:         false,
		pauseLanDisc: false,
	}
}

func (self *Ghost) SetCid(cid string) *Ghost {
	self.cid = cid
	return self
}

func (self *Ghost) SetCommand(command string) *Ghost {
	self.shellCommand = command
	return self
}

func (self *Ghost) LoadPropertiesFromFile(filename string) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Printf("LoadPropertiesFromFile: %s\n", err)
		return
	}

	if err := json.Unmarshal(bytes, &self.properties); err != nil {
		log.Printf("LoadPropertiesFromFile: %s\n", err)
		return
	}
}

func (self *Ghost) handleTerminalRequest(req *Request) error {
	type RequestParams struct {
		Sid string `json:"sid"`
	}

	var params RequestParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}

	go func() {
		log.Printf("Received terminal command, Terminal %s spawned\n", params.Sid)
		g := NewGhost(self.addrs, TERMINAL).SetCid(params.Sid)
		g.Start(true)
	}()

	res := NewResponse(req.Rid, SUCCESS, nil)
	return self.SendResponse(res)
}

func (self *Ghost) handleShellRequest(req *Request) error {
	type RequestParams struct {
		Sid string `json:"sid"`
		Cmd string `json:"command"`
	}

	var params RequestParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}

	go func() {
		log.Printf("Received shell command: %s, shell %s spawned\n", params.Cmd, params.Sid)
		g := NewGhost(self.addrs, SHELL).SetCid(params.Sid).SetCommand(params.Cmd)
		g.Start(true)
	}()

	res := NewResponse(req.Rid, SUCCESS, nil)
	return self.SendResponse(res)
}

func (self *Ghost) handleRequest(req *Request) error {
	var err error
	switch req.Name {
	case "shell":
		err = self.handleShellRequest(req)
	case "terminal":
		err = self.handleTerminalRequest(req)
	default:
		err = errors.New(`Received unregistered command "` + req.Name + `", ignoring`)
	}
	return err
}

func (self *Ghost) ProcessRequests(reqs []*Request) error {
	for _, req := range reqs {
		if err := self.handleRequest(req); err != nil {
			return err
		}
	}
	return nil
}

func (self *Ghost) Ping() error {
	pingHandler := func(res *Response) error {
		if res == nil {
			self.reset = true
			return errors.New("Ping timeout")
		}
		return nil
	}
	req := NewRequest("ping", nil)
	req.SetTimeout(PING_TIMEOUT)
	return self.SendRequest(req, pingHandler)
}

// Spawn a PTY server and forward I/O to the TCP socket.
func (self *Ghost) SpawnPTYServer(res *Response) error {
	log.Println("SpawnPTYServer: started")

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = DEFAULT_SHELL
	}

	home := os.Getenv("HOME")
	if home == "" {
		home = "/"
	}

	os.Chdir(home)
	cmd := exec.Command(shell)
	tty, err := pty.Start(cmd)
	if err != nil {
		return errors.New(`SpawnPTYServer: Cannot start "` + shell + `", abort`)
	}

	defer func() {
		self.quit = true
		cmd.Process.Kill()
		tty.Close()
		log.Println("SpawnPTYServer: terminated")
	}()

	stopConn := make(chan bool, 1)

	go func() {
		io.Copy(self.Conn, tty)
		cmd.Wait()
		stopConn <- true
	}()

	for {
		select {
		case buffer := <-self.readChan:
			tty.Write([]byte(buffer))
		case err := <-self.readErrChan:
			if err == io.EOF {
				log.Println("SpawnPTYServer: connection dropped")
				return nil
			} else {
				return err
			}
		case s := <-stopConn:
			if s {
				return nil
			}
		}
	}

	return nil
}

// Spawn a Shell server and forward input/output from/to the TCP socket.
func (self *Ghost) SpawnShellServer(res *Response) error {
	log.Println("SpawnShellServer: started")

	var err error

	defer func() {
		if err != nil {
			self.Conn.Write([]byte(err.Error()))
		}
		self.quit = true
		self.Conn.Close()
		log.Println("SpawnShellServer: terminated")
	}()

	parts, err := shlex.Split(self.shellCommand)
	cmd_name, err := exec.LookPath(parts[0])
	if err != nil {
		return err
	}

	cmd := exec.Command(cmd_name, parts[1:]...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	stopConn := make(chan bool, 1)

	go io.Copy(self.Conn, stdout)
	go func() {
		io.Copy(self.Conn, stderr)
		cmd.Wait()
		stopConn <- true
	}()

	if err = cmd.Start(); err != nil {
		return err
	}

	for {
		select {
		case buf := <-self.readChan:
			stdin.Write([]byte(buf))
		case err := <-self.readErrChan:
			if err == io.EOF {
				cmd.Process.Kill()
				return errors.New("SpawnShellServer: connection dropped")
			} else {
				return err
			}
		case s := <-stopConn:
			if s {
				return nil
			}
		}
	}

	return nil
}

// Register existent to Overlord.
func (self *Ghost) Register() error {
	for _, addr := range self.addrs {
		log.Printf("Trying %s ...\n", addr)
		conn, err := net.DialTimeout("tcp", addr, DIAL_TIMEOUT*time.Second)
		if err == nil {
			log.Println("Connection established, registering...")
			self.Conn = conn
			req := NewRequest("register", map[string]interface{}{
				"mid":        self.mid,
				"cid":        self.cid,
				"mode":       self.mode,
				"properties": self.properties,
			})

			registered := func(res *Response) error {
				if res == nil {
					self.reset = true
					return errors.New("Register request timeout")
				} else {
					log.Printf("Registered with Overlord at %s", addr)
					self.pauseLanDisc = true
				}
				return nil
			}

			var handler ResponseHandler
			switch self.mode {
			case AGENT:
				handler = registered
			case TERMINAL:
				handler = self.SpawnPTYServer
			case SHELL:
				handler = self.SpawnShellServer
			}
			err = self.SendRequest(req, handler)
			return nil
		}
	}

	return errors.New("Cannot connect to any server")
}

// Reset all states for a new connection.
func (self *Ghost) Reset() {
	self.ClearRequests()
	self.reset = false
}

// Main routine for listen to socket messages.
func (self *Ghost) Listen() error {
	readChan, readErrChan := self.SpawnReaderRoutine()
	pingTicker := time.NewTicker(time.Duration(PING_INTERVAL * time.Second))
	reqTicker := time.NewTicker(time.Duration(TIMEOUT_CHECK_SECS * time.Second))

	self.readChan = readChan
	self.readErrChan = readErrChan

	defer func() {
		self.Conn.Close()
		self.pauseLanDisc = false
	}()

	for {
		select {
		case buffer := <-readChan:
			reqs := self.ParseRequests(buffer, false)
			if self.quit {
				return nil
			}
			if err := self.ProcessRequests(reqs); err != nil {
				log.Println(err)
				continue
			}
		case err := <-readErrChan:
			if err == io.EOF {
				return errors.New("Connection dropped")
			} else {
				return err
			}
		case <-pingTicker.C:
			self.Ping()
		case <-reqTicker.C:
			err := self.ScanForTimeoutRequests()
			if self.reset {
				return err
			}
		}
	}
}

// Start listening to LAN discovery message.
func (self *Ghost) StartLanDiscovery() {
	log.Println("LAN discovery: started")
	buf := make([]byte, BUFSIZ)
	conn, err := net.ListenPacket("udp", fmt.Sprintf(":%d", OVERLORD_LD_PORT))
	if err != nil {
		log.Printf("LAN discovery: %s, abort\n", err.Error())
		return
	}

	defer func() {
		conn.Close()
		log.Println("LAN discovery: stopped")
	}()

	for {
		conn.SetReadDeadline(time.Now().Add(READ_TIMEOUT * time.Second))
		_, remote, err := conn.ReadFrom(buf)

		if self.pauseLanDisc {
			log.Println("LAN discovery: paused")
			ticker := time.NewTicker(READ_TIMEOUT * time.Second)
		waitLoop:
			for {
				select {
				case <-ticker.C:
					if !self.pauseLanDisc {
						break waitLoop
					}
				}
			}
			log.Println("LAN discovery: resumed")
			continue
		}

		if err != nil {
			continue
		}

		// LAN discovery packet format: "OVERLOARD :port"
		data := strings.Split(string(buf), " ")
		parts := strings.Split(remote.String(), ":")
		remoteAddr := parts[0] + data[1]

		if data[0] == "OVERLORD" {
			found := false
			for _, addr := range self.addrs {
				if addr == remoteAddr {
					found = true
					break
				}
			}
			if !found {
				log.Printf("LAN discovery: got overlord address %s", remoteAddr)
				self.addrs = append(self.addrs, remoteAddr)
			}
		}
	}
}

// ScanGateWay scans currenty netowrk gateway and add it into addrs if not
// already exist.
func (self *Ghost) ScanGateway() {
	exists := func(ips []string, target string) bool {
		for _, x := range ips {
			if target == x {
				return true
			}
		}
		return false
	}

	if gateways, err := GetGateWayIP(); err == nil {
		for _, gw := range gateways {
			addr := fmt.Sprintf("%s:%d", gw, OVERLORD_PORT)
			if !exists(self.addrs, addr) {
				self.addrs = append(self.addrs, addr)
			}
		}
	}
}

// Bootstrap and start the client.
func (self *Ghost) Start(noLanDisc bool) {
	log.Printf("%s started\n", ModeStr(self.mode))
	log.Printf("MID: %s\n", self.mid)
	log.Printf("CID: %s\n", self.cid)
	// Only control channel should perform LAN discovery

	if !noLanDisc {
		go self.StartLanDiscovery()
	}

	for {
		self.ScanGateway()
		err := self.Register()
		if err == nil {
			err = self.Listen()
		}
		if self.quit {
			break
		}
		self.Reset()
		log.Printf("%s, retrying in %ds\n", err, RETRY_INTERVAL)
		time.Sleep(RETRY_INTERVAL * time.Second)
	}
}

func StartGhost(args []string, noLanDisc bool, propFile string) {
	var addrs []string

	if len(args) >= 1 {
		addrs = append(addrs, fmt.Sprintf("%s:%d", args[0], OVERLORD_PORT))
	}
	addrs = append(addrs, fmt.Sprintf("%s:%d", OVERLORD_IP, OVERLORD_PORT))

	g := NewGhost(addrs, AGENT)
	if propFile != "" {
		g.LoadPropertiesFromFile(propFile)
	}
	go g.Start(false)

	ticker := time.NewTicker(time.Duration(60 * time.Second))

	for {
		select {
		case <-ticker.C:
			log.Printf("Num of Goroutines: %d\n", runtime.NumGoroutine())
		}
	}
}
