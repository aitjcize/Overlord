// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
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
	addrs          []string               // List of possible Overlord addresses
	mid            string                 // Machine ID
	cid            string                 // Client ID
	mode           int                    // mode, see constants.go
	properties     map[string]interface{} // Client properties
	reset          bool                   // Whether to reset the connection
	quit           bool                   // Whether to quit the connection
	readChan       chan string            // The incoming data channel
	readErrChan    chan error             // The incoming data error channel
	pauseLanDisc   bool                   // Stop LAN discovery
	logcatFilename string                 // filename to cat in logcat mode
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

func (self *Ghost) SetFilename(filename string) *Ghost {
	self.logcatFilename = filename
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
		g.Start(false)
	}()

	res := NewResponse(req.Rid, SUCCESS, nil)
	return self.SendResponse(res)
}

func (self *Ghost) handleLogcatRequest(req *Request) error {
	type RequestParams struct {
		Sid      string `json:"sid"`
		Filename string `json:"filename"`
	}

	var params RequestParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}

	go func() {
		log.Printf("Received logcat command: %s, Logcat %s spawned\n", params.Filename, params.Sid)
		g := NewGhost(self.addrs, LOGCAT).SetCid(params.Sid).SetFilename(params.Filename)
		g.Start(false)
	}()

	res := NewResponse(req.Rid, SUCCESS, nil)
	return self.SendResponse(res)
}

func (self *Ghost) handleShellRequest(req *Request) error {
	type RequestParams struct {
		Cmd  string   `json:"cmd"`
		Args []string `json:"args"`
	}

	var params RequestParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}

	log.Printf("Received shell command: %s, executing\n", params.Cmd)

	var (
		res    *Response
		cmd    *exec.Cmd
		output []byte
		errMsg string
	)

	cmdPath, err := exec.LookPath(params.Cmd)
	if err == nil {
		log.Printf("Executing command: %s %s\n", params.Cmd, params.Args)
		cmd = exec.Command(cmdPath, params.Args...)
		output, err = cmd.CombinedOutput()
	} else {
		errMsg = err.Error()
		log.Printf("shell command error: %s\n", errMsg)
	}

	res = NewResponse(req.Rid, SUCCESS,
		map[string]interface{}{"output": string(output), "err_msg": errMsg})
	return self.SendResponse(res)
}

func (self *Ghost) handleRequest(req *Request) error {
	var err error
	switch req.Name {
	case "shell":
		err = self.handleShellRequest(req)
	case "terminal":
		err = self.handleTerminalRequest(req)
	case "logcat":
		err = self.handleLogcatRequest(req)
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

// Spawn a Logcat server and forward output to the TCP socket.
func (self *Ghost) SpawnLogcatServer(res *Response) error {
	log.Println("SpawnLogcatServer: started")

	defer func() {
		self.quit = true
		self.Conn.Close()
		log.Println("SpawnLogcatServer: terminated")
	}()

	tail, err := exec.LookPath("tail")
	if err != nil {
		return err
	}

	cmd := exec.Command(tail, "-n", "+0", "-f", self.logcatFilename)
	stdout, err1 := cmd.StdoutPipe()
	stderr, err2 := cmd.StderrPipe()
	if err1 != nil || err2 != nil {
		return err
	}

	go io.Copy(self.Conn, stdout)
	go func() {
		io.Copy(self.Conn, stderr)
		cmd.Wait()
	}()

	if err := cmd.Start(); err != nil {
		return err
	}

	// For monitoring the connection status.
	// If we get a read error, it's most likely the connection is closed.
	// In that case we can kill the "tail" command and terminate the io.Copy
	// goroutine.
	for {
		select {
		case _ = <-self.readChan:
			continue
		case err := <-self.readErrChan:
			if err == io.EOF {
				cmd.Process.Kill()
				return errors.New("SpawnLogcatServer: connection dropped")
			} else {
				return err
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
			case LOGCAT:
				handler = self.SpawnLogcatServer
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
	go g.Start(noLanDisc)

	ticker := time.NewTicker(time.Duration(60 * time.Second))

	for {
		select {
		case <-ticker.C:
			log.Printf("Num of Goroutines: %d\n", runtime.NumGoroutine())
		}
	}
}
