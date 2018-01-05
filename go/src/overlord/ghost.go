// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kr/pty"
	"github.com/pkg/term/termios"
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

var ghostRPCStubPort = GetenvInt("GHOST_RPC_PORT", 4499)

const (
	defaultShell         = "/bin/bash"
	pingInterval         = 10 * time.Second
	readTimeout          = 3 * time.Second
	connectTimeout       = 3 * time.Second
	retryIntervalSeconds = 2
	blockSize            = 4096
)

// Exported
const (
	RandomMID = "##random_mid##" // Random Machine ID identifier
)

// TLS modes
const (
	TLSDetect = iota
	TLSForceDisable
	TLSForceEnable
)

// Terminal resize control
const (
	controlNone  = 255 // Control State None
	controlStart = 128 // Control Start Code
	controlEnd   = 129 // Control End Code
)

// Stream control
const (
	stdinClosed = "##STDIN_CLOSED##"
)

// Registration status
const (
	statusDisconnected = "disconnected"
)

type ghostRPCStub struct {
	ghost *Ghost
}

// EmptyArgs for RPC.
type EmptyArgs struct {
}

// EmptyReply for RPC.
type EmptyReply struct {
}

func (rpcStub *ghostRPCStub) Reconnect(args *EmptyArgs, reply *EmptyReply) error {
	rpcStub.ghost.reset = true
	return nil
}

func (rpcStub *ghostRPCStub) GetStatus(args *EmptyArgs, reply *string) error {
	*reply = rpcStub.ghost.RegisterStatus
	if rpcStub.ghost.RegisterStatus == Success {
		*reply = fmt.Sprintf("%s %s", *reply, rpcStub.ghost.connectedAddr)
	}
	return nil
}

func (rpcStub *ghostRPCStub) RegisterTTY(args []string, reply *EmptyReply) error {
	rpcStub.ghost.RegisterTTY(args[0], args[1])
	return nil
}

func (rpcStub *ghostRPCStub) RegisterSession(args []string, reply *EmptyReply) error {
	rpcStub.ghost.RegisterSession(args[0], args[1])
	return nil
}

func (rpcStub *ghostRPCStub) AddToDownloadQueue(args []string, reply *EmptyReply) error {
	rpcStub.ghost.AddToDownloadQueue(args[0], args[1])
	return nil
}

// downloadInfo is a structure that we be place into download queue.
// In our case since we always execute 'ghost --download' in our pseudo
// terminal so ttyName will always have the form /dev/pts/X
type downloadInfo struct {
	Ttyname  string
	Filename string
}

// fileOperation is a structure for storing file upload/download intent.
type fileOperation struct {
	Action   string
	Filename string
	Perm     int
}

type fileUploadContext struct {
	Ready bool
	Data  chan []byte
}

type tlsSettings struct {
	Enabled     bool        // TLS enabled or not
	tlsCertFile string      // TLS certificate in PEM format
	verify      bool        // Wether or not to verify the certificate
	Config      *tls.Config // TLS configuration
}

func newTLSSettings(tlsCertFile string, verify bool) *tlsSettings {
	return &tlsSettings{false, tlsCertFile, verify, nil}
}

func (t *tlsSettings) updateContext() {
	if !t.Enabled {
		t.Config = nil
		return
	}

	if t.verify {
		config := &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
			RootCAs:            nil,
		}
		if t.tlsCertFile != "" {
			log.Println("TLSSettings: using user-supplied ca-certificate")
			cert, err := ioutil.ReadFile(t.tlsCertFile)
			if err != nil {
				log.Fatalln(err)
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(cert)
			config.RootCAs = caCertPool
		} else {
			log.Println("TLSSettings: using built-in ca-certificates")
		}
		t.Config = config
	} else {
		log.Println("TLSSettings: skipping TLS verification!!!")
		t.Config = &tls.Config{InsecureSkipVerify: true}
	}
}

func (t *tlsSettings) SetEnabled(enabled bool) {
	status := "True"
	if !enabled {
		status = "False"
	}

	log.Println("TLSSettings: enabled:", status)
	if enabled != t.Enabled {
		t.Enabled = enabled
		t.updateContext()
	}
}

// Ghost type is the main context for storing the ghost state.
type Ghost struct {
	*RPCCore
	addrs           []string               // List of possible Overlord addresses
	server          *rpc.Server            // RPC server handle
	connectedAddr   string                 // Current connected Overlord address
	tls             *tlsSettings           // TLS settings
	mode            int                    // mode, see constants.go
	mid             string                 // Machine ID
	sid             string                 // Session ID
	terminalSid     string                 // Associated terminal session ID
	ttyName2Sid     map[string]string      // Mapping between ttyName and Sid
	terminalSid2Pid map[string]int         // Mapping between terminalSid and pid
	propFile        string                 // Properties file filename
	properties      map[string]interface{} // Client properties
	RegisterStatus  string                 // Register status from server response
	reset           bool                   // Whether to reset the connection
	quit            bool                   // Whether to quit the connection
	readChan        chan []byte            // The incoming data channel
	readErrChan     chan error             // The incoming data error channel
	pauseLanDisc    bool                   // Stop LAN discovery
	ttyDevice       string                 // Terminal device to open
	shellCommand    string                 // Shell command to execute
	fileOp          fileOperation          // File operation name
	downloadQueue   chan downloadInfo      // Download queue
	uploadContext   fileUploadContext      // File upload context
	port            int                    // Port number to forward
	tlsMode         int                    // TLS mode
}

// NewGhost creates a Ghost object.
func NewGhost(addrs []string, tls *tlsSettings, mode int, mid string) *Ghost {
	var (
		finalMid string
		err      error
	)

	if mid == RandomMID {
		finalMid = uuid.Must(uuid.NewV4()).String()
	} else if mid != "" {
		finalMid = mid
	} else {
		finalMid, err = GetMachineID()
		if err != nil {
			log.Fatalln("Unable to get machine ID:", err)
		}
	}
	return &Ghost{
		RPCCore:         NewRPCCore(nil),
		addrs:           addrs,
		tls:             tls,
		mode:            mode,
		mid:             finalMid,
		sid:             uuid.Must(uuid.NewV4()).String(),
		ttyName2Sid:     make(map[string]string),
		terminalSid2Pid: make(map[string]int),
		properties:      make(map[string]interface{}),
		RegisterStatus:  statusDisconnected,
		reset:           false,
		quit:            false,
		pauseLanDisc:    false,
		downloadQueue:   make(chan downloadInfo),
		uploadContext:   fileUploadContext{Data: make(chan []byte)},
	}
}

// SetSid sets the Session ID for the Ghost instance.
func (ghost *Ghost) SetSid(sid string) *Ghost {
	ghost.sid = sid
	return ghost
}

// SetTerminalSid sets the terminal session ID for the Ghost instance.
func (ghost *Ghost) SetTerminalSid(sid string) *Ghost {
	ghost.terminalSid = sid
	return ghost
}

// SetPropFile sets the property file filename.
func (ghost *Ghost) SetPropFile(propFile string) *Ghost {
	ghost.propFile = propFile
	return ghost
}

// SetTtyDevice sets the TTY device name to open.
func (ghost *Ghost) SetTtyDevice(ttyDevice string) *Ghost {
	ghost.ttyDevice = ttyDevice
	return ghost
}

// SetShellCommand sets the shell comamnd to execute.
func (ghost *Ghost) SetShellCommand(command string) *Ghost {
	ghost.shellCommand = command
	return ghost
}

// SetFileOp sets the file operation to perform.
func (ghost *Ghost) SetFileOp(operation, filename string, perm int) *Ghost {
	ghost.fileOp.Action = operation
	ghost.fileOp.Filename = filename
	ghost.fileOp.Perm = perm
	return ghost
}

// SetModeForwardPort sets the port to forward.
func (ghost *Ghost) SetModeForwardPort(port int) *Ghost {
	ghost.port = port
	return ghost
}

// SetTLSMode sets the mode of tls detection.
func (ghost *Ghost) SetTLSMode(mode int) *Ghost {
	ghost.tlsMode = mode
	return ghost
}

func (ghost *Ghost) existsInAddr(target string) bool {
	for _, x := range ghost.addrs {
		if target == x {
			return true
		}
	}
	return false
}

func (ghost *Ghost) loadProperties() {
	if ghost.propFile == "" {
		return
	}

	bytes, err := ioutil.ReadFile(ghost.propFile)
	if err != nil {
		log.Printf("loadProperties: %s\n", err)
		return
	}

	if err := json.Unmarshal(bytes, &ghost.properties); err != nil {
		log.Printf("loadProperties: %s\n", err)
		return
	}
}

func (ghost *Ghost) tlsEnabled(addr string) (bool, error) {
	conn, err := net.DialTimeout("tcp", addr, connectTimeout)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	colonPos := strings.LastIndex(addr, ":")
	tlsConn := tls.Client(conn, &tls.Config{
		// Allow any certificate since we only want to check if server talks TLS.
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		RootCAs:            nil,
		ServerName:         addr[:colonPos],
	})
	defer tlsConn.Close()

	handshakeTimeout := false

	// Close the connection to stop handshake if it's taking too long.
	go func() {
		time.Sleep(connectTimeout)
		conn.Close()
		handshakeTimeout = true
	}()

	err = tlsConn.Handshake()
	if err != nil || handshakeTimeout {
		return false, nil
	}
	return true, nil
}

// Upgrade starts the upgrade sequence of the ghost instance.
func (ghost *Ghost) Upgrade() error {
	log.Println("Upgrade: initiating upgrade sequence...")

	exePath, err := GetExecutablePath()
	if err != nil {
		return errors.New("Upgrade: can not find executable path")
	}

	var buffer bytes.Buffer
	var client http.Client

	parts := strings.Split(ghost.connectedAddr, ":")
	httpsEnabled, err := ghost.tlsEnabled(
		fmt.Sprintf("%s:%d", parts[0], OverlordHTTPPort))
	if err != nil {
		return errors.New("Upgrade: failed to connect to Overlord HTTP server, " +
			"abort")
	}

	if ghost.tls.Enabled && !httpsEnabled {
		return errors.New("Upgrade: TLS enforced but found Overlord HTTP server " +
			"without TLS enabled! Possible mis-configuration or DNS/IP spoofing " +
			"detected, abort")
	}

	proto := "http"
	if httpsEnabled {
		proto = "https"
	}
	url := fmt.Sprintf("%s://%s:%d/upgrade/ghost.%s", proto, parts[0],
		OverlordHTTPPort, GetPlatformString())

	if httpsEnabled {
		tr := &http.Transport{TLSClientConfig: ghost.tls.Config}
		client = http.Client{Transport: tr, Timeout: connectTimeout}
	} else {
		client = http.Client{Timeout: connectTimeout}
	}

	// Download the sha1sum for ghost for verification
	resp, err := client.Get(url + ".sha1")
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
	resp2, err := client.Get(url)
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
		return fmt.Errorf("Upgrade: %s", err)
	}
	exeFile.Close()

	err = os.Chmod(exePath, 0755)
	if err != nil {
		return fmt.Errorf("Upgrade: %s", err)
	}

	log.Println("Upgrade: restarting ghost...")
	os.Args[0] = exePath
	return syscall.Exec(exePath, os.Args, os.Environ())
}

func (ghost *Ghost) handleTerminalRequest(req *Request) error {
	type RequestParams struct {
		Sid       string `json:"sid"`
		TtyDevice string `json:"tty_device"`
	}

	var params RequestParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}

	go func() {
		log.Printf("Received terminal command, Terminal agent %s spawned\n", params.Sid)
		addrs := []string{ghost.connectedAddr}
		// Terminal sessions are identified with session ID, thus we don't care
		// machine ID and can make them random.
		g := NewGhost(addrs, ghost.tls, ModeTerminal, RandomMID).SetSid(
			params.Sid).SetTtyDevice(params.TtyDevice)
		g.Start(false, false)
	}()

	res := NewResponse(req.Rid, Success, nil)
	return ghost.SendResponse(res)
}

func (ghost *Ghost) handleShellRequest(req *Request) error {
	type RequestParams struct {
		Sid string `json:"sid"`
		Cmd string `json:"command"`
	}

	var params RequestParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}

	go func() {
		log.Printf("Received shell command: %s, Shell agent %s spawned\n", params.Cmd, params.Sid)
		addrs := []string{ghost.connectedAddr}
		// Shell sessions are identified with session ID, thus we don't care
		// machine ID and can make them random.
		g := NewGhost(addrs, ghost.tls, ModeShell, RandomMID).SetSid(
			params.Sid).SetShellCommand(params.Cmd)
		g.Start(false, false)
	}()

	res := NewResponse(req.Rid, Success, nil)
	return ghost.SendResponse(res)
}

func (ghost *Ghost) handleFileDownloadRequest(req *Request) error {
	type RequestParams struct {
		Sid      string `json:"sid"`
		Filename string `json:"filename"`
	}

	var params RequestParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}

	filename := params.Filename
	if !strings.HasPrefix(filename, "/") {
		home := os.Getenv("HOME")
		if home == "" {
			home = "/tmp"
		}
		filename = filepath.Join(home, filename)
	}

	f, err := os.Open(filename)
	if err != nil {
		res := NewResponse(req.Rid, err.Error(), nil)
		return ghost.SendResponse(res)
	}
	f.Close()

	go func() {
		log.Printf("Received file_download command, File agent %s spawned\n", params.Sid)
		addrs := []string{ghost.connectedAddr}
		g := NewGhost(addrs, ghost.tls, ModeFile, RandomMID).SetSid(
			params.Sid).SetFileOp("download", filename, 0)
		g.Start(false, false)
	}()

	res := NewResponse(req.Rid, Success, nil)
	return ghost.SendResponse(res)
}

func (ghost *Ghost) handleFileUploadRequest(req *Request) error {
	type RequestParams struct {
		Sid         string `json:"sid"`
		TerminalSid string `json:"terminal_sid"`
		Filename    string `json:"filename"`
		Dest        string `json:"dest"`
		Perm        int    `json:"perm"`
		CheckOnly   bool   `json:"check_only"`
	}

	var params RequestParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}

	targetDir := os.Getenv("HOME")
	if targetDir == "" {
		targetDir = "/tmp"
	}

	destPath := params.Dest
	if destPath != "" {
		if !filepath.IsAbs(destPath) {
			destPath = filepath.Join(targetDir, destPath)
		}

		st, err := os.Stat(destPath)
		if err == nil && st.Mode().IsDir() {
			destPath = filepath.Join(destPath, params.Filename)
		}
	} else {
		if params.TerminalSid != "" {
			if pid, ok := ghost.terminalSid2Pid[params.TerminalSid]; ok {
				cwd, err := GetProcessWorkingDirectory(pid)
				if err == nil {
					targetDir = cwd
				}
			}
		}
		destPath = filepath.Join(targetDir, params.Filename)
	}

	os.MkdirAll(filepath.Dir(destPath), 0755)

	f, err := os.Create(destPath)
	if err != nil {
		res := NewResponse(req.Rid, err.Error(), nil)
		return ghost.SendResponse(res)
	}
	f.Close()

	// If not check_only, spawn ModeFile mode ghost agent to handle upload
	if !params.CheckOnly {
		go func() {
			log.Printf("Received file_upload command, File agent %s spawned\n",
				params.Sid)
			addrs := []string{ghost.connectedAddr}
			g := NewGhost(addrs, ghost.tls, ModeFile, RandomMID).SetSid(
				params.Sid).SetFileOp("upload", destPath, params.Perm)
			g.Start(false, false)
		}()
	}

	res := NewResponse(req.Rid, Success, nil)
	return ghost.SendResponse(res)
}

func (ghost *Ghost) handleModeForwardRequest(req *Request) error {
	type RequestParams struct {
		Sid  string `json:"sid"`
		Port int    `json:"port"`
	}

	var params RequestParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}

	go func() {
		log.Printf("Received forward command, ModeForward agent %s spawned\n", params.Sid)
		addrs := []string{ghost.connectedAddr}
		g := NewGhost(addrs, ghost.tls, ModeForward, RandomMID).SetSid(
			params.Sid).SetModeForwardPort(params.Port)
		g.Start(false, false)
	}()

	res := NewResponse(req.Rid, Success, nil)
	return ghost.SendResponse(res)
}

// StartDownloadServer starts the download server.
func (ghost *Ghost) StartDownloadServer() error {
	log.Println("StartDownloadServer: started")

	defer func() {
		ghost.quit = true
		ghost.Conn.Close()
		log.Println("StartDownloadServer: terminated")
	}()

	file, err := os.Open(ghost.fileOp.Filename)
	if err != nil {
		return err
	}
	defer file.Close()

	io.Copy(ghost.Conn, file)
	return nil
}

// StartUploadServer starts the upload server.
func (ghost *Ghost) StartUploadServer() error {
	log.Println("StartUploadServer: started")

	defer func() {
		log.Println("StartUploadServer: terminated")
	}()

	filePath := ghost.fileOp.Filename
	dirPath := filepath.Dir(filePath)
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		os.MkdirAll(dirPath, 0755)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	for {
		buffer := <-ghost.uploadContext.Data
		if buffer == nil {
			break
		}
		file.Write(buffer)
	}

	if ghost.fileOp.Perm > 0 {
		file.Chmod(os.FileMode(ghost.fileOp.Perm))
	}

	return nil
}

func (ghost *Ghost) handleRequest(req *Request) error {
	var err error
	switch req.Name {
	case "upgrade":
		err = ghost.Upgrade()
	case "terminal":
		err = ghost.handleTerminalRequest(req)
	case "shell":
		err = ghost.handleShellRequest(req)
	case "file_download":
		err = ghost.handleFileDownloadRequest(req)
	case "clear_to_download":
		err = ghost.StartDownloadServer()
	case "file_upload":
		err = ghost.handleFileUploadRequest(req)
	case "forward":
		err = ghost.handleModeForwardRequest(req)
	default:
		err = errors.New(`Received unregistered command "` + req.Name + `", ignoring`)
	}
	return err
}

func (ghost *Ghost) processRequests(reqs []*Request) error {
	for _, req := range reqs {
		if err := ghost.handleRequest(req); err != nil {
			return err
		}
	}
	return nil
}

// Ping sends a ping message to the overlord server.
func (ghost *Ghost) Ping() error {
	pingHandler := func(res *Response) error {
		if res == nil {
			ghost.reset = true
			return errors.New("Ping timeout")
		}
		return nil
	}
	req := NewRequest("ping", nil)
	req.SetTimeout(pingTimeout)
	return ghost.SendRequest(req, pingHandler)
}

func (ghost *Ghost) handleTTYControl(tty *os.File, controlString string) error {
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
	err := json.Unmarshal([]byte(controlString), &control)
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

func (ghost *Ghost) getTTYName() (string, error) {
	ttyName, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/0", os.Getpid()))
	if err != nil {
		return "", err
	}
	return ttyName, nil
}

// SpawnTTYServer Spawns a TTY server and forward I/O to the TCP socket.
func (ghost *Ghost) SpawnTTYServer(res *Response) error {
	log.Println("SpawnTTYServer: started")

	var tty *os.File
	var err error
	stopConn := make(chan bool, 1)

	defer func() {
		ghost.quit = true
		if tty != nil {
			tty.Close()
		}
		ghost.Conn.Close()
		log.Println("SpawnTTYServer: terminated")
	}()

	if ghost.ttyDevice == "" {
		// No TTY device specified, open a PTY (pseudo terminal) instead.
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = defaultShell
		}

		home := os.Getenv("HOME")
		if home == "" {
			home = "/root"
		}

		// Add ghost executable to PATH
		exePath, err := GetExecutablePath()
		if err == nil {
			os.Setenv("PATH", fmt.Sprintf("%s:%s", filepath.Dir(exePath),
				os.Getenv("PATH")))
		}

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
		ttyName, err := termios.Ptsname(tty.Fd())
		if err != nil {
			return err
		}

		client, err := ghostRPCStubServer()

		// Ghost could be launched without RPC server, ignore registraion
		if err == nil {
			err = client.Call("rpc.RegisterTTY", []string{ghost.sid, ttyName},
				&EmptyReply{})
			if err != nil {
				return err
			}

			err = client.Call("rpc.RegisterSession", []string{
				ghost.sid, strconv.Itoa(cmd.Process.Pid)}, &EmptyReply{})
			if err != nil {
				return err
			}
		}

		go func() {
			io.Copy(ghost.Conn, tty)
			cmd.Wait()
			stopConn <- true
		}()
	} else {
		// Open a TTY device
		tty, err = os.OpenFile(ghost.ttyDevice, os.O_RDWR, 0)
		if err != nil {
			return err
		}

		var term syscall.Termios
		err := termios.Tcgetattr(tty.Fd(), &term)
		if err != nil {
			return nil
		}

		termios.Cfmakeraw(&term)
		term.Iflag &= (syscall.IXON | syscall.IXOFF)
		term.Cflag |= syscall.CLOCAL
		term.Ispeed = syscall.B115200
		term.Ospeed = syscall.B115200

		if err = termios.Tcsetattr(tty.Fd(), termios.TCSANOW, &term); err != nil {
			return err
		}

		go func() {
			io.Copy(ghost.Conn, tty)
			stopConn <- true
		}()
	}

	var controlBuffer bytes.Buffer
	var writeBuffer bytes.Buffer
	controlState := controlNone

	processBuffer := func(buffer []byte) error {
		writeBuffer.Reset()
		for len(buffer) > 0 {
			if controlState != controlNone {
				index := bytes.IndexByte(buffer, controlEnd)
				if index != -1 {
					controlBuffer.Write(buffer[:index])
					err := ghost.handleTTYControl(tty, controlBuffer.String())
					controlState = controlNone
					controlBuffer.Reset()
					if err != nil {
						return err
					}
					buffer = buffer[index+1:]
				} else {
					controlBuffer.Write(buffer)
					buffer = buffer[0:0]
				}
			} else {
				index := bytes.IndexByte(buffer, controlStart)
				if index != -1 {
					controlState = controlStart
					writeBuffer.Write(buffer[:index])
					buffer = buffer[index+1:]
				} else {
					writeBuffer.Write(buffer)
					buffer = buffer[0:0]
				}
			}
		}
		if writeBuffer.Len() != 0 {
			tty.Write(writeBuffer.Bytes())
		}
		return nil
	}

	if ghost.ReadBuffer != "" {
		processBuffer([]byte(ghost.ReadBuffer))
		ghost.ReadBuffer = ""
	}

	for {
		select {
		case buffer := <-ghost.readChan:
			err := processBuffer(buffer)
			if err != nil {
				log.Println("SpawnTTYServer:", err)
			}
		case err := <-ghost.readErrChan:
			if err == io.EOF {
				log.Println("SpawnTTYServer: connection terminated")
				return nil
			}
			return err
		case s := <-stopConn:
			if s {
				return nil
			}
		}
	}

	return nil
}

// SpawnShellServer spawns a Shell server and forward input/output from/to the TCP socket.
func (ghost *Ghost) SpawnShellServer(res *Response) error {
	log.Println("SpawnShellServer: started")

	var err error

	defer func() {
		ghost.quit = true
		if err != nil {
			ghost.Conn.Write([]byte(err.Error() + "\n"))
		}
		ghost.Conn.Close()
		log.Println("SpawnShellServer: terminated")
	}()

	// Execute shell command from HOME directory
	home := os.Getenv("HOME")
	if home == "" {
		home = "/tmp"
	}
	os.Chdir(home)

	// Add ghost executable to PATH
	exePath, err := GetExecutablePath()
	if err == nil {
		os.Setenv("PATH", fmt.Sprintf("%s:%s", os.Getenv("PATH"),
			filepath.Dir(exePath)))
	}

	cmd := exec.Command(defaultShell, "-c", ghost.shellCommand)
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

	if ghost.ReadBuffer != "" {
		stdin.Write([]byte(ghost.ReadBuffer))
		ghost.ReadBuffer = ""
	}

	go io.Copy(ghost.Conn, stdout)
	go func() {
		io.Copy(ghost.Conn, stderr)
		stopConn <- true
	}()

	if err = cmd.Start(); err != nil {
		return err
	}

	defer func() {
		time.Sleep(100 * time.Millisecond) // Wait for process to terminate

		process := (*PollableProcess)(cmd.Process)
		_, err = process.Poll()
		// Check if the process is terminated. If not, send SIGlogcatTypeVT100 to the process,
		// then wait for 1 second.  Send another SIGKILL to make sure the process is
		// terminated.
		if err != nil {
			cmd.Process.Signal(syscall.SIGTERM)
			time.Sleep(time.Second)
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	for {
		select {
		case buf := <-ghost.readChan:
			if len(buf) >= len(stdinClosed)*2 {
				idx := bytes.Index(buf, []byte(stdinClosed+stdinClosed))
				if idx != -1 {
					stdin.Write(buf[:idx])
					stdin.Close()
					continue
				}
			}
			stdin.Write(buf)
		case err := <-ghost.readErrChan:
			if err == io.EOF {
				log.Println("SpawnShellServer: connection terminated")
				return nil
			}
			log.Printf("SpawnShellServer: %s\n", err)
			return err
		case s := <-stopConn:
			if s {
				return nil
			}
		}
	}

	return nil
}

// InitiatefileOperation initiates a file operation.
// The operation could either be 'download' or 'upload'
// This function starts handshake with overlord then execute download sequence.
func (ghost *Ghost) InitiatefileOperation(res *Response) error {
	if ghost.fileOp.Action == "download" {
		fi, err := os.Stat(ghost.fileOp.Filename)
		if err != nil {
			return err
		}

		req := NewRequest("request_to_download", map[string]interface{}{
			"terminal_sid": ghost.terminalSid,
			"filename":     filepath.Base(ghost.fileOp.Filename),
			"size":         fi.Size(),
		})

		return ghost.SendRequest(req, nil)
	} else if ghost.fileOp.Action == "upload" {
		ghost.uploadContext.Ready = true
		req := NewRequest("clear_to_upload", nil)
		req.SetTimeout(-1)
		err := ghost.SendRequest(req, nil)
		if err != nil {
			return err
		}
		go ghost.StartUploadServer()
		return nil
	} else {
		return errors.New("InitiatefileOperation: unknown file operation, ignored")
	}
	return nil
}

// SpawnPortModeForwardServer spawns a port forwarding server and forward I/O to
// the TCP socket.
func (ghost *Ghost) SpawnPortModeForwardServer(res *Response) error {
	log.Println("SpawnPortModeForwardServer: started")

	var err error

	defer func() {
		ghost.quit = true
		if err != nil {
			ghost.Conn.Write([]byte(err.Error() + "\n"))
		}
		ghost.Conn.Close()
		log.Println("SpawnPortModeForwardServer: terminated")
	}()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", ghost.port),
		connectTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	stopConn := make(chan bool, 1)

	if ghost.ReadBuffer != "" {
		conn.Write([]byte(ghost.ReadBuffer))
		ghost.ReadBuffer = ""
	}

	go func() {
		io.Copy(ghost.Conn, conn)
		stopConn <- true
	}()

	for {
		select {
		case buf := <-ghost.readChan:
			conn.Write(buf)
		case err := <-ghost.readErrChan:
			if err == io.EOF {
				log.Println("SpawnPortModeForwardServer: connection terminated")
				return nil
			}
			return err
		case s := <-stopConn:
			if s {
				return nil
			}
		}
	}

	return nil
}

// Register existent to Overlord.
func (ghost *Ghost) Register() error {
	for _, addr := range ghost.addrs {
		var (
			conn net.Conn
			err  error
		)

		log.Printf("Trying %s ...\n", addr)
		ghost.Reset()

		// Check if server has TLS enabled.
		// Only control channel needs to determine if TLS is enabled. Other mode
		// should use the tlsSettings passed in when it was spawned.
		if ghost.mode == ModeControl {
			var enabled bool

			switch ghost.tlsMode {
			case TLSDetect:
				enabled, err = ghost.tlsEnabled(addr)
				if err != nil {
					continue
				}
			case TLSForceEnable:
				enabled = true
			case TLSForceDisable:
				enabled = false
			}

			ghost.tls.SetEnabled(enabled)
		}

		conn, err = net.DialTimeout("tcp", addr, connectTimeout)
		if err != nil {
			continue
		}

		log.Println("Connection established, registering...")
		if ghost.tls.Enabled {
			colonPos := strings.LastIndex(addr, ":")
			config := ghost.tls.Config
			config.ServerName = addr[:colonPos]
			conn = tls.Client(conn, config)
		}

		ghost.Conn = conn
		req := NewRequest("register", map[string]interface{}{
			"mid":        ghost.mid,
			"sid":        ghost.sid,
			"mode":       ghost.mode,
			"properties": ghost.properties,
		})

		registered := func(res *Response) error {
			if res == nil {
				ghost.reset = true
				return errors.New("Register request timeout")
			} else if res.Response != Success {
				log.Println("Register:", res.Response)
			} else {
				log.Printf("Registered with Overlord at %s", addr)
				ghost.connectedAddr = addr
				if err := ghost.Upgrade(); err != nil {
					log.Println(err)
				}
				ghost.pauseLanDisc = true
			}
			ghost.RegisterStatus = res.Response
			return nil
		}

		var handler ResponseHandler
		switch ghost.mode {
		case ModeControl:
			handler = registered
		case ModeTerminal:
			handler = ghost.SpawnTTYServer
		case ModeShell:
			handler = ghost.SpawnShellServer
		case ModeFile:
			handler = ghost.InitiatefileOperation
		case ModeForward:
			handler = ghost.SpawnPortModeForwardServer
		}
		err = ghost.SendRequest(req, handler)
		return nil
	}

	return errors.New("Cannot connect to any server")
}

// InitiateDownload initiates a client-initiated download request.
func (ghost *Ghost) InitiateDownload(info downloadInfo) {
	go func() {
		addrs := []string{ghost.connectedAddr}
		g := NewGhost(addrs, ghost.tls, ModeFile, RandomMID).SetTerminalSid(
			ghost.ttyName2Sid[info.Ttyname]).SetFileOp("download", info.Filename, 0)
		g.Start(false, false)
	}()
}

// Reset all states for a new connection.
func (ghost *Ghost) Reset() {
	ghost.ClearRequests()
	ghost.reset = false
	ghost.loadProperties()
	ghost.RegisterStatus = statusDisconnected
}

// Listen is the main routine for listen to socket messages.
func (ghost *Ghost) Listen() error {
	readChan, readErrChan := ghost.SpawnReaderRoutine()
	pingTicker := time.NewTicker(time.Duration(pingInterval))
	reqTicker := time.NewTicker(time.Duration(timeoutCheckInterval))

	ghost.readChan = readChan
	ghost.readErrChan = readErrChan

	defer func() {
		ghost.Conn.Close()
		ghost.pauseLanDisc = false
	}()

	for {
		select {
		case buffer := <-readChan:
			if ghost.uploadContext.Ready {
				if ghost.ReadBuffer != "" {
					// Write the leftover from previous ReadBuffer
					ghost.uploadContext.Data <- []byte(ghost.ReadBuffer)
					ghost.ReadBuffer = ""
				}
				ghost.uploadContext.Data <- buffer
				continue
			}
			reqs := ghost.ParseRequests(string(buffer), ghost.RegisterStatus != Success)
			if ghost.quit {
				return nil
			}
			if err := ghost.processRequests(reqs); err != nil {
				log.Println(err)
			}
		case err := <-readErrChan:
			if err == io.EOF {
				if ghost.uploadContext.Ready {
					ghost.uploadContext.Data <- nil
					ghost.quit = true
					return nil
				}
				return errors.New("Connection dropped")
			}
			return err
		case info := <-ghost.downloadQueue:
			ghost.InitiateDownload(info)
		case <-pingTicker.C:
			if ghost.mode == ModeControl {
				ghost.Ping()
			}
		case <-reqTicker.C:
			err := ghost.ScanForTimeoutRequests()
			if ghost.reset {
				if err == nil {
					err = errors.New("reset request")
				}
				return err
			}
		}
	}
}

// RegisterTTY register the TTY to a session.
func (ghost *Ghost) RegisterTTY(sesssionID, ttyName string) {
	ghost.ttyName2Sid[ttyName] = sesssionID
}

// RegisterSession register the PID to a session.
func (ghost *Ghost) RegisterSession(sesssionID, pidStr string) {
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		panic(err)
	}
	ghost.terminalSid2Pid[sesssionID] = pid
}

// AddToDownloadQueue adds a downloadInfo to the download queue
func (ghost *Ghost) AddToDownloadQueue(ttyName, filename string) {
	ghost.downloadQueue <- downloadInfo{ttyName, filename}
}

// StartLanDiscovery starts listening to LAN discovery message.
func (ghost *Ghost) StartLanDiscovery() {
	log.Println("LAN discovery: started")
	buf := make([]byte, bufferSize)
	conn, err := net.ListenPacket("udp", fmt.Sprintf(":%d", OverlordLDPort))
	if err != nil {
		log.Printf("LAN discovery: %s, abort\n", err)
		return
	}

	defer func() {
		conn.Close()
		log.Println("LAN discovery: stopped")
	}()

	for {
		conn.SetReadDeadline(time.Now().Add(readTimeout))
		n, remote, err := conn.ReadFrom(buf)

		if ghost.pauseLanDisc {
			log.Println("LAN discovery: paused")
			ticker := time.NewTicker(readTimeout)
		waitLoop:
			for {
				select {
				case <-ticker.C:
					if !ghost.pauseLanDisc {
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

		if !ghost.existsInAddr(remoteAddr) {
			log.Printf("LAN discovery: got overlord address %s", remoteAddr)
			ghost.addrs = append(ghost.addrs, remoteAddr)
		}
	}
}

// ServeHTTP method for serving JSON-RPC over HTTP.
func (ghost *Ghost) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var conn, _, err = w.(http.Hijacker).Hijack()
	if err != nil {
		log.Print("rpc hijacking ", req.RemoteAddr, ": ", err.Error())
		return
	}
	io.WriteString(conn, "HTTP/1.1 200\n")
	io.WriteString(conn, "Content-Type: application/json-rpc\n\n")
	ghost.server.ServeCodec(jsonrpc.NewServerCodec(conn))
}

// StartRPCServer starts a local RPC server used for communication between
// ghost instances.
func (ghost *Ghost) StartRPCServer() {
	log.Println("RPC Server: started")

	ghost.server = rpc.NewServer()
	ghost.server.RegisterName("rpc", &ghostRPCStub{ghost})

	http.Handle("/", ghost)
	err := http.ListenAndServe(fmt.Sprintf("localhost:%d", ghostRPCStubPort), nil)
	if err != nil {
		log.Fatalf("Unable to listen at port %d: %s\n", ghostRPCStubPort, err)
	}
}

// ScanGateway scans currenty netowrk gateway and add it into addrs if not
// already exist.
func (ghost *Ghost) ScanGateway() {
	if gateways, err := GetGateWayIP(); err == nil {
		for _, gw := range gateways {
			addr := fmt.Sprintf("%s:%d", gw, OverlordPort)
			if !ghost.existsInAddr(addr) {
				ghost.addrs = append(ghost.addrs, addr)
			}
		}
	}
}

// Start bootstraps and start the client.
func (ghost *Ghost) Start(lanDisc bool, RPCServer bool) {
	log.Printf("%s started\n", ModeStr(ghost.mode))
	log.Printf("MID: %s\n", ghost.mid)
	log.Printf("SID: %s\n", ghost.sid)

	if lanDisc {
		go ghost.StartLanDiscovery()
	}

	if RPCServer {
		go ghost.StartRPCServer()
	}

	for {
		ghost.ScanGateway()
		err := ghost.Register()
		if err == nil {
			err = ghost.Listen()
		}
		if ghost.quit {
			break
		}
		log.Printf("%s, retrying in %ds\n", err, retryIntervalSeconds)
		time.Sleep(retryIntervalSeconds * time.Second)
		ghost.Reset()
	}
}

// Returns a ghostRPCStub client object which can be used to call ghostRPCStub methods.
func ghostRPCStubServer() (*rpc.Client, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", ghostRPCStubPort))
	if err != nil {
		return nil, err
	}

	io.WriteString(conn, "GET / HTTP/1.1\nHost: localhost\n\n")
	_, err = http.ReadResponse(bufio.NewReader(conn), nil)
	if err == nil {
		return jsonrpc.NewClient(conn), nil
	}
	return nil, err
}

// DownloadFile adds a file to the download queue, which would be pickup by the
// ghost control channel instance and perform download.
func DownloadFile(filename string) {
	client, err := ghostRPCStubServer()
	if err != nil {
		log.Printf("error: %s\n", err)
		os.Exit(1)
	}

	var ttyName string
	var f *os.File

	absPath, err := filepath.Abs(filename)
	if err != nil {
		goto fail
	}

	_, err = os.Stat(absPath)
	if err != nil {
		goto fail
	}

	f, err = os.Open(absPath)
	if err != nil {
		goto fail
	}
	f.Close()

	ttyName, err = Ttyname(os.Stdout.Fd())
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

// StartGhost starts the Ghost client.
func StartGhost(args []string, mid string, noLanDisc bool, noRPCServer bool,
	tlsCertFile string, verify bool, propFile string, download string,
	reset bool, status bool, tlsMode int) {
	var addrs []string

	if status {
		client, err := ghostRPCStubServer()
		if err != nil {
			log.Printf("error: %s\n", err)
			os.Exit(1)
		}

		var reply string
		err = client.Call("rpc.GetStatus", &EmptyArgs{}, &reply)
		if err != nil {
			log.Printf("GetStatus: %s\n", err)
			os.Exit(1)
		}
		fmt.Println(reply)
		os.Exit(0)
	}

	if reset {
		client, err := ghostRPCStubServer()
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
		addrs = append(addrs, fmt.Sprintf("%s:%d", args[0], OverlordPort))
	}
	addrs = append(addrs, fmt.Sprintf("localhost:%d", OverlordPort))

	tlsSettings := newTLSSettings(tlsCertFile, verify)

	if propFile != "" {
		var err error
		propFile, err = filepath.Abs(propFile)
		if err != nil {
			log.Println("propFile:", err)
			os.Exit(1)
		}
	}

	g := NewGhost(addrs, tlsSettings, ModeControl, mid)
	g.SetPropFile(propFile).SetTLSMode(tlsMode)
	go g.Start(!noLanDisc, !noRPCServer)

	ticker := time.NewTicker(time.Duration(60 * time.Second))

	for {
		select {
		case <-ticker.C:
			log.Printf("Num of Goroutines: %d\n", runtime.NumGoroutine())
		}
	}
}
