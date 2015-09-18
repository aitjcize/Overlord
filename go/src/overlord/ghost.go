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
	"strconv"
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

// An structure that we be place into download queue.
// In our case since we always execute 'ghost --download' in our pseudo
// terminal so ttyName will always have the form /dev/pts/X
type DownloadInfo struct {
	Ttyname  string
	Filename string
}

type FileOperation struct {
	Action   string
	Filename string
	Pid      int
}

type FileUploadContext struct {
	Ready bool
	Data  chan []byte
}

type Ghost struct {
	*RPCCore
	addrs           []string               // List of possible Overlord addresses
	server          *rpc.Server            // RPC server handle
	connectedAddr   string                 // Current connected Overlord address
	mode            int                    // mode, see constants.go
	mid             string                 // Machine ID
	sid             string                 // Session ID
	terminalSid     string                 // Associated terminal session ID
	ttyName2Sid     map[string]string      // Mapping between ttyName and Sid
	terminalSid2Pid map[string]int         // Mapping between terminalSid and pid
	propFile        string                 // Properties file filename
	properties      map[string]interface{} // Client properties
	reset           bool                   // Whether to reset the connection
	quit            bool                   // Whether to quit the connection
	readChan        chan []byte            // The incoming data channel
	readErrChan     chan error             // The incoming data error channel
	pauseLanDisc    bool                   // Stop LAN discovery
	ttyDevice       string                 // Terminal device to open
	shellCommand    string                 // Shell command to execute
	fileOperation   FileOperation          // File operation name
	downloadQueue   chan DownloadInfo      // Download queue
	upload          FileUploadContext      // File upload context
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
		RPCCore:         NewRPCCore(nil),
		addrs:           addrs,
		mode:            mode,
		mid:             finalMid,
		sid:             uuid.NewV4().String(),
		ttyName2Sid:     make(map[string]string),
		terminalSid2Pid: make(map[string]int),
		properties:      make(map[string]interface{}),
		reset:           false,
		quit:            false,
		pauseLanDisc:    false,
		downloadQueue:   make(chan DownloadInfo),
		upload:          FileUploadContext{Data: make(chan []byte)},
	}
}

func (self *Ghost) SetSid(sid string) *Ghost {
	self.sid = sid
	return self
}

func (self *Ghost) SetTerminalSid(sid string) *Ghost {
	self.terminalSid = sid
	return self
}

func (self *Ghost) SetPropFile(propFile string) *Ghost {
	self.propFile = propFile
	return self
}

func (self *Ghost) SetTtyDevice(ttyDevice string) *Ghost {
	self.ttyDevice = ttyDevice
	return self
}

func (self *Ghost) SetCommand(command string) *Ghost {
	self.shellCommand = command
	return self
}

func (self *Ghost) SetFileOp(operation, filename string, pid int) *Ghost {
	self.fileOperation.Action = operation
	self.fileOperation.Filename = filename
	self.fileOperation.Pid = pid
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

func (self *Ghost) LoadProperties() {
	if self.propFile == "" {
		return
	}

	bytes, err := ioutil.ReadFile(self.propFile)
	if err != nil {
		log.Printf("LoadProperties: %s\n", err)
		return
	}

	if err := json.Unmarshal(bytes, &self.properties); err != nil {
		log.Printf("LoadProperties: %s\n", err)
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
		return errors.New(fmt.Sprintf("Upgrade: %s", err))
	}
	exeFile.Close()

	err = os.Chmod(exePath, 0755)
	if err != nil {
		return errors.New(fmt.Sprintf("Upgrade: %s", err))
	}

	log.Println("Upgrade: restarting ghost...")
	os.Args[0] = exePath
	return syscall.Exec(exePath, os.Args, os.Environ())
}

func (self *Ghost) handleTerminalRequest(req *Request) error {
	type RequestParams struct {
		Sid       string `json:"sid"`
		TtyDevice string `json:"tty_device"`
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
		g := NewGhost(addrs, TERMINAL, RANDOM_MID).SetSid(params.Sid).SetTtyDevice(
			params.TtyDevice)
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
		g := NewGhost(addrs, SHELL, RANDOM_MID).SetSid(params.Sid).SetCommand(params.Cmd)
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
		log.Println("Received file_download command, file agent spawned")
		addrs := []string{self.connectedAddr}
		g := NewGhost(addrs, FILE, RANDOM_MID).SetSid(params.Sid).SetFileOp(
			"download", params.Filename, 0)
		g.Start(false, false)
	}()

	res := NewResponse(req.Rid, SUCCESS, nil)
	return self.SendResponse(res)
}

func (self *Ghost) handleFileUploadRequest(req *Request) error {
	type RequestParams struct {
		Sid         string `json:"sid"`
		TerminalSid string `json:"terminal_sid"`
		Filename    string `json:"filename"`
	}

	var params RequestParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}

	pid, ok := self.terminalSid2Pid[params.TerminalSid]
	if !ok {
		pid = 0
	}

	go func() {
		log.Println("Received file_upload command, file agent spawned")
		addrs := []string{self.connectedAddr}
		g := NewGhost(addrs, FILE, RANDOM_MID).SetSid(params.Sid).SetFileOp(
			"upload", params.Filename, pid)
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

	file, err := os.Open(self.fileOperation.Filename)
	if err != nil {
		return err
	}
	defer file.Close()

	io.Copy(self.Conn, file)
	return nil
}

func (self *Ghost) StartUploadServer() error {
	log.Println("StartUploadServer: started")

	defer func() {
		log.Println("StartUploadServer: terminated")
	}()

	// Get the client's working dir, which is our target upload dir
	target_dir := os.Getenv("HOME")
	if target_dir == "" {
		target_dir = "/tmp"
	}

	var err error
	if self.fileOperation.Pid != 0 {
		target_dir, err = os.Readlink(fmt.Sprintf("/proc/%d/cwd",
			self.fileOperation.Pid))
		if err != nil {
			return err
		}
	}

	file, err := os.Create(filepath.Join(target_dir, self.fileOperation.Filename))
	if err != nil {
		return err
	}
	defer file.Close()

	for {
		buffer := <-self.upload.Data
		if buffer == nil {
			break
		}
		file.Write(buffer)
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
	case "file_upload":
		err = self.handleFileUploadRequest(req)
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

func (self *Ghost) HandleTTYControl(tty *os.File, control_string string) error {
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

// Spawn a TTY server and forward I/O to the TCP socket.
func (self *Ghost) SpawnTTYServer(res *Response) error {
	log.Println("SpawnTTYServer: started")

	var tty *os.File
	var err error
	stopConn := make(chan bool, 1)

	defer func() {
		self.quit = true
		if tty != nil {
			tty.Close()
		}
		log.Println("SpawnTTYServer: terminated")
	}()

	if self.ttyDevice == "" {
		// No TTY device specified, open a PTY (pseudo terminal) instead.
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
		tty, err = pty.Start(cmd)
		if err != nil {
			return errors.New(`SpawnTTYServer: Cannot start "` + shell + `", abort`)
		}

		defer func() {
			cmd.Process.Kill()
		}()

		// Register the mapping of sid and ttyName
		ttyName, err := PtsName(tty)
		if err != nil {
			return err
		}

		client, err := GhostRPCServer()

		// Ghost could be launched without RPC server, ignore registraion
		if err == nil {
			err = client.Call("rpc.RegisterTTY", []string{self.sid, ttyName},
				&EmptyReply{})
			if err != nil {
				return err
			}

			err = client.Call("rpc.RegisterSession", []string{
				self.sid, strconv.Itoa(cmd.Process.Pid)}, &EmptyReply{})
			if err != nil {
				return err
			}
		}

		go func() {
			io.Copy(self.Conn, tty)
			cmd.Wait()
			stopConn <- true
		}()
	} else {
		// Open a TTY device
		tty, err = os.OpenFile(self.ttyDevice, os.O_RDWR, 0)
		if err != nil {
			return err
		}

		go func() {
			io.Copy(self.Conn, tty)
			stopConn <- true
		}()
	}

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
						err := self.HandleTTYControl(tty, control_buffer.String())
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
				log.Println("SpawnTTYServer: connection dropped")
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

	cmd := exec.Command(DEFAULT_SHELL, "-c", self.shellCommand)
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
	if self.fileOperation.Action == "download" {
		fi, err := os.Stat(self.fileOperation.Filename)
		if err != nil {
			return err
		}

		req := NewRequest("request_to_download", map[string]interface{}{
			"terminal_sid": self.terminalSid,
			"filename":     filepath.Base(self.fileOperation.Filename),
			"size":         fi.Size(),
		})

		return self.SendRequest(req, nil)
	} else if self.fileOperation.Action == "upload" {
		self.upload.Ready = true
		req := NewRequest("clear_to_upload", nil)
		req.SetTimeout(-1)
		err := self.SendRequest(req, nil)
		if err != nil {
			return err
		}
		go self.StartUploadServer()
		return nil
	} else {
		return errors.New("InitiateFileOperation: unknown file operation, ignored")
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
				"sid":        self.sid,
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
					if err := self.Upgrade(); err != nil {
						log.Println(err)
					}
					self.pauseLanDisc = true
				}
				return nil
			}

			var handler ResponseHandler
			switch self.mode {
			case AGENT:
				handler = registered
			case TERMINAL:
				handler = self.SpawnTTYServer
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
	g := NewGhost(addrs, FILE, RANDOM_MID).SetTerminalSid(
		self.ttyName2Sid[info.Ttyname]).SetFileOp("download", info.Filename, 0)
	g.Start(false, false)
}

// Reset all states for a new connection.
func (self *Ghost) Reset() {
	self.ClearRequests()
	self.reset = false
	self.LoadProperties()
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
			if self.upload.Ready {
				if self.ReadBuffer != "" {
					// Write the left over from previous ReadBuffer
					self.upload.Data <- []byte(self.ReadBuffer)
					self.ReadBuffer = ""
				}
				self.upload.Data <- buffer
				continue
			}
			reqs := self.ParseRequests(string(buffer), false)
			if self.quit {
				return nil
			}
			if err := self.ProcessRequests(reqs); err != nil {
				log.Println(err)
			}
		case err := <-readErrChan:
			if err == io.EOF {
				if self.upload.Ready {
					self.upload.Data <- nil
					self.quit = true
					return nil
				}
				return errors.New("Connection dropped")
			} else {
				return err
			}
		case info := <-self.downloadQueue:
			self.InitiateDownload(info)
		case <-pingTicker.C:
			// When upload is in progress, we don't want to send any ping message.
			if !self.upload.Ready {
				self.Ping()
			}
		case <-reqTicker.C:
			err := self.ScanForTimeoutRequests()
			if self.reset {
				if err == nil {
					err = errors.New("reset request")
				}
				return err
			}
		}
	}
}

func (self *Ghost) RegisterTTY(session_id, ttyName string) {
	self.ttyName2Sid[ttyName] = session_id
}

func (self *Ghost) RegisterSession(session_id, pidStr string) {
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		panic(err)
	}
	self.terminalSid2Pid[session_id] = pid
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
		log.Printf("LAN discovery: %s, abort\n", err)
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
	log.Printf("SID: %s\n", self.sid)

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
		log.Printf("error: %s\n", err)
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
	if err != nil {
		goto fail
	}
	os.Exit(0)

fail:
	log.Println(err)
	os.Exit(1)
}

func StartGhost(args []string, mid string, noLanDisc bool, noRPCServer bool,
	propFile string, download string, reset bool) {
	var addrs []string

	if reset {
		client, err := GhostRPCServer()
		if err != nil {
			log.Printf("error: %s\n", err)
			os.Exit(1)
		}

		err = client.Call("rpc.Reconnect", &EmptyArgs{}, &EmptyReply{})
		if err != nil {
			log.Printf("Reset: %s\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if download != "" {
		DownloadFile(download)
	}

	if len(args) >= 1 {
		addrs = append(addrs, fmt.Sprintf("%s:%d", args[0], OVERLORD_PORT))
	}
	addrs = append(addrs, fmt.Sprintf("%s:%d", LOCALHOST, OVERLORD_PORT))

	g := NewGhost(addrs, AGENT, mid)
	g.SetPropFile(propFile)
	go g.Start(!noLanDisc, !noRPCServer)

	ticker := time.NewTicker(time.Duration(60 * time.Second))

	for {
		select {
		case <-ticker.C:
			log.Printf("Num of Goroutines: %d\n", runtime.NumGoroutine())
		}
	}
}
