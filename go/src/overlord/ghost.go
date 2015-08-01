// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flynn/go-shlex"
	"github.com/kr/pty"
	"github.com/satori/go.uuid"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	GHOST_RPC_PORT = 4499
	DEFAULT_SHELL  = "/bin/bash"
	DIAL_TIMEOUT   = 3
	LOCALHOST      = "localhost"
	PING_INTERVAL  = 10
	PING_TIMEOUT   = 10
	RETRY_INTERVAL = 2
	READ_TIMEOUT   = 3
	RANDOM_MID     = "##random_mid##"
	BLOCK_SIZE     = 4096
)

type DownloadInfo struct {
	Ttyname  string
	Filename string
}

type Ghost struct {
	*RPCCore
	addrs         []string               // List of possible Overlord addresses
	sessionMap    map[string]string      // Mapping between ttyName and bid
	server        *rpc.Server            // RPC server handle
	connectedAddr string                 // Current connected Overlord address
	mid           string                 // Machine ID
	cid           string                 // Client ID
	bid           string                 // Browser ID
	mode          int                    // mode, see constants.go
	properties    map[string]interface{} // Client properties
	reset         bool                   // Whether to reset the connection
	quit          bool                   // Whether to quit the connection
	readChan      chan []byte            // The incoming data channel
	readErrChan   chan error             // The incoming data error channel
	pauseLanDisc  bool                   // Stop LAN discovery
	shellCommand  string                 // Shell command to execute
	fileOperation string                 // File operation name
	fileFilename  string                 // File operation filename
	downloadQueue chan DownloadInfo      // Download queue
}

func NewGhost(addrs []string, mode int, mid string) *Ghost {
	var finalMid string
	var err error

	if mid == RANDOM_MID {
		finalMid = uuid.NewV4().String()
	} else if mid != "" {
		finalMid = mid
	} else {
		finalMid, err = GetMachineID()
		if err != nil {
			panic(err)
		}
	}
	return &Ghost{
		RPCCore:       NewRPCCore(nil),
		sessionMap:    make(map[string]string),
		addrs:         addrs,
		mid:           finalMid,
		cid:           uuid.NewV4().String(),
		mode:          mode,
		properties:    make(map[string]interface{}),
		reset:         false,
		quit:          false,
		pauseLanDisc:  false,
		downloadQueue: make(chan DownloadInfo),
	}
}

func (self *Ghost) SetCid(cid string) *Ghost {
	self.cid = cid
	return self
}

func (self *Ghost) SetBid(bid string) *Ghost {
	self.bid = bid
	return self
}

func (self *Ghost) SetCommand(command string) *Ghost {
	self.shellCommand = command
	return self
}

func (self *Ghost) SetFileOp(operation, filename string) *Ghost {
	self.fileOperation = operation
	self.fileFilename = filename
	return self
}

func (self *Ghost) ExistsInAddr(target string) bool {
	for _, x := range self.addrs {
		if target == x {
			return true
		}
	}
	return false
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

func (self *Ghost) closeSockets() {
}

func (self *Ghost) Upgrade() error {
	log.Println("Upgrade: initiating upgrade sequence...")

	exePath, err := GetExecutablePath()
	if err != nil {
		return errors.New("Upgrade: can not find executable path")
	}

	var buffer bytes.Buffer
	parts := strings.Split(self.connectedAddr, ":")
	url := fmt.Sprintf("http://%s:%d/upgrade/ghost.%s", parts[0],
		OVERLORD_HTTP_PORT, GetArchString())

	// Download the sha1sum for ghost for verification
	resp, err := http.Get(url + ".sha1")
	if err != nil || resp.StatusCode != 200 {
		return errors.New("Upgrade: failed to download sha1sum file, abort")
	}
	sha1sumBytes := make([]byte, 40)
	resp.Body.Read(sha1sumBytes)
	sha1sum := strings.Trim(string(sha1sumBytes), "\n ")
	defer resp.Body.Close()

	// Compare the current version of ghost, if sha1 is the same, skip upgrading
	currentSha1sum, _ := GetFileSha1(exePath)

	if currentSha1sum == sha1sum {
		log.Println("Upgrade: ghost is already up-to-date, skipping upgrade")
		return nil
	}

	// Download upgrade version of ghost
	resp2, err := http.Get(url)
	if err != nil || resp2.StatusCode != 200 {
		return errors.New("Upgrade: failed to download upgrade, abort")
	}
	defer resp2.Body.Close()

	_, err = buffer.ReadFrom(resp2.Body)
	if err != nil {
		return errors.New("Upgrade: failed to write upgrade onto disk, abort")
	}

	// Compare SHA1 sum
	if sha1sum != fmt.Sprintf("%x", sha1.Sum(buffer.Bytes())) {
		return errors.New("Upgrade: sha1sum mismatch, abort")
	}

	os.Remove(exePath)
	exeFile, err := os.Create(exePath)
	if err != nil {
		return errors.New("Upgrade: can not open ghost executable for writing")
	}
	_, err = buffer.WriteTo(exeFile)
	if err != nil {
		return errors.New(fmt.Sprintf("Upgrade: %s", err.Error()))
	}
	exeFile.Close()

	err = os.Chmod(exePath, 0755)
	if err != nil {
		return errors.New(fmt.Sprintf("Upgrade: %s", err.Error()))
	}

	log.Println("Upgrade: restarting ghost...")
	os.Args[0] = exePath
	return syscall.Exec(exePath, os.Args, os.Environ())
}

func (self *Ghost) handleTerminalRequest(req *Request) error {
	type RequestParams struct {
		Sid string `json:"sid"`
		Bid string `json:"bid"`
	}

	var params RequestParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}

	go func() {
		log.Printf("Received terminal command, Terminal %s spawned\n", params.Sid)
		addrs := []string{self.connectedAddr}
		// Terminal sessions are identified with session ID, thus we don't care
		// machine ID and can make them random.
		g := NewGhost(addrs, TERMINAL, RANDOM_MID).SetCid(params.Sid).SetBid(params.Bid)
		g.Start(false, false)
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
		addrs := []string{self.connectedAddr}
		// Shell sessions are identified with session ID, thus we don't care
		// machine ID and can make them random.
		g := NewGhost(addrs, SHELL, RANDOM_MID).SetCid(params.Sid).SetCommand(params.Cmd)
		g.Start(false, false)
	}()

	res := NewResponse(req.Rid, SUCCESS, nil)
	return self.SendResponse(res)
}

func (self *Ghost) handleFileDownloadRequest(req *Request) error {
	type RequestParams struct {
		Sid      string `json:"sid"`
		Filename string `json:"filename"`
	}

	var params RequestParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}

	go func() {
		log.Printf("Received file_download command, file agent spawned\n")
		addrs := []string{self.connectedAddr}
		g := NewGhost(addrs, FILE, RANDOM_MID).SetCid(params.Sid).SetFileOp(
			"download", params.Filename)
		g.Start(false, false)
	}()

	res := NewResponse(req.Rid, SUCCESS, nil)
	return self.SendResponse(res)
}

func (self *Ghost) StartDownloadServer() error {
	log.Println("StartDownloadServer: started")

	defer func() {
		self.quit = true
		self.Conn.Close()
		log.Println("StartDownloadServer: terminated")
	}()

	file, err := os.Open(self.fileFilename)
	if err != nil {
		return err
	}

	for {
		data := make([]byte, BLOCK_SIZE)
		count, err := file.Read(data)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		self.Conn.Write(data[:count])
	}

	return nil
}

func (self *Ghost) handleRequest(req *Request) error {
	var err error
	switch req.Name {
	case "upgrade":
		err = self.Upgrade()
	case "terminal":
		err = self.handleTerminalRequest(req)
	case "shell":
		err = self.handleShellRequest(req)
	case "file_download":
		err = self.handleFileDownloadRequest(req)
	case "clear_to_download":
		err = self.StartDownloadServer()
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

func (self *Ghost) HandlePTYControl(tty *os.File, control_string string) error {
	// Terminal Command for ghost
	// Implements the Message interface.
	type TerminalCommand struct {
		Command string          `json:"command"`
		Params  json.RawMessage `json:"params"`
	}

	// winsize stores the Height and Width of a terminal.
	type winsize struct {
		height uint16
		width  uint16
	}

	var control TerminalCommand
	err := json.Unmarshal([]byte(control_string), &control)
	if err != nil {
		log.Println("mal-formed JSON request, ignored")
		return nil
	}

	command := control.Command
	if command == "resize" {
		var params []int
		err := json.Unmarshal([]byte(control.Params), &params)
		if err != nil || len(params) != 2 {
			log.Println("mal-formed JSON request, ignored")
			return nil
		}
		ws := &winsize{width: uint16(params[1]), height: uint16(params[0])}
		syscall.Syscall(syscall.SYS_IOCTL, tty.Fd(),
			uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(ws)))
	} else {
		return errors.New("Invalid request command " + command)
	}
	return nil
}

func (self *Ghost) getTTYName() (string, error) {
	ttyName, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/0", os.Getpid()))
	if err != nil {
		return "", err
	}
	return ttyName, nil
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

	// Add ghost executable to PATH
	exePath, err := GetExecutablePath()
	os.Setenv("PATH", fmt.Sprintf("%s:%s", os.Getenv("PATH"),
		filepath.Dir(exePath)))

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

	// Register the mapping of browser_id and ttyName
	ttyName, err := PtsName(tty)
	if err != nil {
		return err
	}

	client, err := GhostRPCServer()
	err = client.Call("rpc.RegisterTTY", []string{self.bid, ttyName},
		&EmptyReply{})
	if err != nil {
		return err
	}

	stopConn := make(chan bool, 1)

	go func() {
		io.Copy(self.Conn, tty)
		cmd.Wait()
		stopConn <- true
	}()

	var control_buffer bytes.Buffer
	var write_buffer bytes.Buffer
	control_state := CONTROL_NONE

	for {
		select {
		case buffer := <-self.readChan:
			write_buffer.Reset()
			for len(buffer) > 0 {
				if control_state != CONTROL_NONE {
					index := bytes.IndexByte(buffer, CONTROL_END)
					if index != -1 {
						control_buffer.Write(buffer[:index])
						err := self.HandlePTYControl(tty, control_buffer.String())
						control_state = CONTROL_NONE
						control_buffer.Reset()
						if err != nil {
							return err
						}
						buffer = buffer[index+1:]
					} else {
						control_buffer.Write(buffer)
						buffer = buffer[0:0]
					}
				} else {
					index := bytes.IndexByte(buffer, CONTROL_START)
					if index != -1 {
						control_state = CONTROL_START
						write_buffer.Write(buffer[:index])
						buffer = buffer[index+1:]
					} else {
						write_buffer.Write(buffer)
						buffer = buffer[0:0]
					}
				}
			}
			if write_buffer.Len() != 0 {
				tty.Write(write_buffer.Bytes())
			}
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
			self.Conn.Write([]byte(err.Error() + "\n"))
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

// Initiate file operation.
// The operation could either be 'download' or 'upload'
// This function starts handshake with overlord then execute download sequence.
func (self *Ghost) InitiateFileOperation(res *Response) error {
	if self.fileOperation == "download" {
		fi, err := os.Stat(self.fileFilename)
		if err != nil {
			return err
		}

		req := NewRequest("request_to_download", map[string]interface{}{
			"bid":      self.bid,
			"filename": filepath.Base(self.fileFilename),
			"size":     fi.Size(),
		})

		req.SetTimeout(PING_TIMEOUT)
		return self.SendRequest(req, nil)
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
					self.connectedAddr = addr
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
			case FILE:
				handler = self.InitiateFileOperation
			}
			err = self.SendRequest(req, handler)
			return nil
		}
	}

	return errors.New("Cannot connect to any server")
}

// Initiate a client-side download request
func (self *Ghost) InitiateDownload(info DownloadInfo) {
	addrs := []string{self.connectedAddr}
	g := NewGhost(addrs, FILE, RANDOM_MID).SetBid(
		self.sessionMap[info.Ttyname]).SetFileOp("download", info.Filename)
	g.Start(false, false)
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
			reqs := self.ParseRequests(string(buffer), false)
			if self.quit {
				return nil
			}
			if err := self.ProcessRequests(reqs); err != nil {
				log.Println(err)
			}
		case err := <-readErrChan:
			if err == io.EOF {
				return errors.New("Connection dropped")
			} else {
				return err
			}
		case info := <-self.downloadQueue:
			self.InitiateDownload(info)
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

func (self *Ghost) RegisterTTY(brower_id, ttyName string) {
	self.sessionMap[ttyName] = brower_id
}

func (self *Ghost) AddToDownloadQueue(ttyName, filename string) {
	self.downloadQueue <- DownloadInfo{ttyName, filename}
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
		n, remote, err := conn.ReadFrom(buf)

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

		// LAN discovery packet format: "OVERLOARD [host]:port"
		data := strings.Split(string(buf[:n]), " ")
		if data[0] != "OVERLORD" {
			continue
		}

		overlordAddrParts := strings.Split(data[1], ":")
		remoteAddrParts := strings.Split(remote.String(), ":")

		var remoteAddr string
		if strings.Trim(overlordAddrParts[0], " ") == "" {
			remoteAddr = remoteAddrParts[0] + ":" + overlordAddrParts[1]
		} else {
			remoteAddr = data[1]
		}

		if !self.ExistsInAddr(remoteAddr) {
			log.Printf("LAN discovery: got overlord address %s", remoteAddr)
			self.addrs = append(self.addrs, remoteAddr)
		}
	}
}

// ServeHTTP method for serving JSON-RPC over HTTP.
func (self *Ghost) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var conn, _, err = w.(http.Hijacker).Hijack()
	if err != nil {
		log.Print("rpc hijacking ", req.RemoteAddr, ": ", err.Error())
		return
	}
	io.WriteString(conn, "HTTP/1.1 200\n")
	io.WriteString(conn, "Content-Type: application/json-rpc\n\n")
	self.server.ServeCodec(jsonrpc.NewServerCodec(conn))
}

// Starts a local RPC server used for communication between ghost instances.
func (self *Ghost) StartRPCServer() {
	log.Println("RPC Server: started")

	ghostRPC := NewGhostRPC(self)
	self.server = rpc.NewServer()
	self.server.RegisterName("rpc", ghostRPC)

	http.Handle("/", self)
	err := http.ListenAndServe(fmt.Sprintf("localhost:%d", GHOST_RPC_PORT), nil)
	if err != nil {
		panic(err)
	}
}

// ScanGateWay scans currenty netowrk gateway and add it into addrs if not
// already exist.
func (self *Ghost) ScanGateway() {
	if gateways, err := GetGateWayIP(); err == nil {
		for _, gw := range gateways {
			addr := fmt.Sprintf("%s:%d", gw, OVERLORD_PORT)
			if !self.ExistsInAddr(addr) {
				self.addrs = append(self.addrs, addr)
			}
		}
	}
}

// Bootstrap and start the client.
func (self *Ghost) Start(lanDisc bool, RPCServer bool) {
	log.Printf("%s started\n", ModeStr(self.mode))
	log.Printf("MID: %s\n", self.mid)
	log.Printf("CID: %s\n", self.cid)

	if lanDisc {
		go self.StartLanDiscovery()
	}

	if RPCServer {
		go self.StartRPCServer()
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

// Returns a GhostRPC client object which can be used to call GhostRPC methods.
func GhostRPCServer() (*rpc.Client, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", GHOST_RPC_PORT))
	if err != nil {
		return nil, err
	}

	io.WriteString(conn, "GET / HTTP/1.1\n\n")

	_, err = http.ReadResponse(bufio.NewReader(conn), nil)
	if err == nil {
		return jsonrpc.NewClient(conn), nil
	}
	return nil, err
}

// Add a file to the download queue, which would be pickup by the ghost
// control channel instance and perform download.
func DownloadFile(filename string) {
	client, err := GhostRPCServer()
	if err != nil {
		log.Printf("error: %s\n", err.Error())
		os.Exit(1)
	}

	var ttyName string

	absPath, err := filepath.Abs(filename)
	if err != nil {
		goto fail
	}
	_, err = os.Stat(absPath)
	if err != nil {
		goto fail
	}
	_, err = os.Open(absPath)
	if err != nil {
		goto fail
	}
	ttyName, err = TtyName(os.Stdout)
	if err != nil {
		goto fail
	}

	err = client.Call("rpc.AddToDownloadQueue", []string{ttyName, absPath},
		&EmptyReply{})

	os.Exit(0)

fail:
	log.Println(err.Error())
	os.Exit(1)
}

func StartGhost(args []string, mid string, noLanDisc bool, noRPCServer bool,
	propFile string, download string) {
	var addrs []string

	if download != "" {
		DownloadFile(download)
	}

	if len(args) >= 1 {
		addrs = append(addrs, fmt.Sprintf("%s:%d", args[0], OVERLORD_PORT))
	}
	addrs = append(addrs, fmt.Sprintf("%s:%d", LOCALHOST, OVERLORD_PORT))

	g := NewGhost(addrs, AGENT, mid)
	if propFile != "" {
		g.LoadPropertiesFromFile(propFile)
	}
	go g.Start(!noLanDisc, !noRPCServer)

	ticker := time.NewTicker(time.Duration(60 * time.Second))

	for {
		select {
		case <-ticker.C:
			log.Printf("Num of Goroutines: %d\n", runtime.NumGoroutine())
		}
	}
}
