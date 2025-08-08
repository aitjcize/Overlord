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
	"io"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/pkg/term/termios"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/sys/unix"
)

var ghostRPCStubPort = GetenvInt("GHOST_RPC_PORT", 4499)

const (
	defaultShell         = "/bin/sh"
	pingInterval         = 10 * time.Second
	readTimeout          = 3 * time.Second
	connectTimeout       = 10 * time.Second
	httpRequestTimeout   = 30 * time.Second
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

// Stream control
const (
	stdinClosed = "##STDIN_CLOSED##"
)

// Registration status
const (
	statusDisconnected = "disconnected"
)

// Escape sequence regex
var escapeSeqRe = regexp.MustCompile(`\x1b\[([0-9;?]*)([A-Za-z])`)

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
			cert, err := os.ReadFile(t.tlsCertFile)
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
	ttyName2Sid     sync.Map               // Mapping between ttyName and Sid
	terminalSid2Pid sync.Map               // Mapping between terminalSid and pid
	allowlist       string                 // Allowlist for permission control
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
	host            string                 // Host to forward
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
		finalMid = uuid.NewV4().String()
	} else if mid != "" {
		finalMid = mid
	} else {
		finalMid, err = GetMachineID()
		if err != nil {
			log.Fatalln("Unable to get machine ID:", err)
		}
	}
	return &Ghost{
		RPCCore:        NewRPCCore(nil),
		addrs:          addrs,
		tls:            tls,
		mode:           mode,
		mid:            finalMid,
		sid:            uuid.NewV4().String(),
		properties:     make(map[string]interface{}),
		RegisterStatus: statusDisconnected,
		reset:          false,
		quit:           false,
		pauseLanDisc:   false,
		downloadQueue:  make(chan downloadInfo),
		uploadContext:  fileUploadContext{Data: make(chan []byte)},
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

// SetForwardTarget sets the host and port to forward.
func (ghost *Ghost) SetForwardTarget(host string, port int) *Ghost {
	ghost.host = host
	ghost.port = port
	return ghost
}

// SetTLSMode sets the mode of tls detection.
func (ghost *Ghost) SetTLSMode(mode int) *Ghost {
	ghost.tlsMode = mode
	return ghost
}

// SetAllowlist sets the allowlist of users and groups that can access this ghost
func (ghost *Ghost) SetAllowlist(allowlist string) *Ghost {
	ghost.allowlist = allowlist
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
	ghost.properties = make(map[string]interface{})

	if ghost.propFile != "" {
		bytes, err := os.ReadFile(ghost.propFile)
		if err != nil {
			log.Printf("loadProperties: %s\n", err)
		} else {
			if err := json.Unmarshal(bytes, &ghost.properties); err != nil {
				log.Printf("loadProperties: %s\n", err)
			}
		}
	}

	if ghost.allowlist != "" {
		// Check if allowlist exists in properties file
		if existingAllowlist, ok := ghost.properties["allowlist"]; ok &&
			existingAllowlist != nil {
			log.Printf("Warning: Overwriting existing allowlist from properties " +
				"file with command line allowlist value")
		}

		// Split the allowlist string and convert to an array
		allowedEntities := []string{}
		for _, entity := range strings.Split(ghost.allowlist, ",") {
			trimmedEntity := strings.TrimSpace(entity)
			if trimmedEntity != "" {
				if trimmedEntity != "anyone" && !strings.Contains(trimmedEntity, "/") {
					trimmedEntity = "u/" + trimmedEntity
				}
				allowedEntities = append(allowedEntities, trimmedEntity)
			}
		}
		ghost.properties["allowlist"] = allowedEntities
	}

	// Default allowlist to current user
	if ghost.properties["allowlist"] == nil || len(ghost.properties["allowlist"].([]string)) == 0 {
		ghost.properties["allowlist"] = []string{"u/" + getCurrentUser()}
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
// Helper functions for Upgrade
func (ghost *Ghost) validateUpgradeConnection() (bool, error) {
	httpsEnabled, err := ghost.tlsEnabled(ghost.connectedAddr)
	if err != nil {
		return false, errors.New("Upgrade: failed to connect to Overlord HTTP server, " +
			"abort")
	}

	if ghost.tls.Enabled && !httpsEnabled {
		return false, errors.New("Upgrade: TLS enforced but found Overlord HTTP server " +
			"without TLS enabled! Possible mis-configuration or DNS/IP spoofing " +
			"detected, abort")
	}

	return httpsEnabled, nil
}

func (ghost *Ghost) createUpgradeClient(httpsEnabled bool) http.Client {
	if httpsEnabled {
		tr := &http.Transport{TLSClientConfig: ghost.tls.Config}
		return http.Client{Transport: tr, Timeout: httpRequestTimeout}
	}
	return http.Client{Timeout: httpRequestTimeout}
}

func (ghost *Ghost) downloadSha1Sum(client http.Client, url string) (string, error) {
	resp, err := client.Get(url + ".sha1")
	if err != nil || resp.StatusCode != 200 {
		return "", errors.New("Upgrade: failed to download sha1sum file, abort")
	}
	defer resp.Body.Close()

	sha1sumBytes := make([]byte, 40)
	if _, err := resp.Body.Read(sha1sumBytes); err != nil {
		log.Printf("Failed to read response body: %v", err)
		return "", err
	}
	return strings.Trim(string(sha1sumBytes), "\n "), nil
}

func (ghost *Ghost) downloadUpgrade(client http.Client, url string) (bytes.Buffer, error) {
	var buffer bytes.Buffer
	resp, err := client.Get(url)
	if err != nil || resp.StatusCode != 200 {
		return buffer, errors.New("Upgrade: failed to download upgrade, abort")
	}
	defer resp.Body.Close()

	_, err = buffer.ReadFrom(resp.Body)
	if err != nil {
		return buffer, errors.New("Upgrade: failed to download upgrade, abort")
	}
	return buffer, nil
}

func (ghost *Ghost) installUpgrade(exePath string, buffer bytes.Buffer) error {
	os.Remove(exePath)
	exeFile, err := os.Create(exePath)
	if err != nil {
		return errors.New("Upgrade: can not open ghost executable for writing")
	}
	defer exeFile.Close()

	_, err = buffer.WriteTo(exeFile)
	if err != nil {
		return fmt.Errorf("Upgrade: %s", err)
	}

	err = os.Chmod(exePath, 0755)
	if err != nil {
		return fmt.Errorf("Upgrade: %s", err)
	}
	return nil
}

func (ghost *Ghost) Upgrade() error {
	log.Println("Upgrade: initiating upgrade sequence...")

	exePath, err := os.Executable()
	if err != nil {
		return errors.New("Upgrade: can not find executable path")
	}

	httpsEnabled, err := ghost.validateUpgradeConnection()
	if err != nil {
		return err
	}

	proto := "http"
	if httpsEnabled {
		proto = "https"
	}
	url := fmt.Sprintf("%s://%s/upgrade/ghost.%s", proto, ghost.connectedAddr,
		GetPlatformString())

	client := ghost.createUpgradeClient(httpsEnabled)

	sha1sum, err := ghost.downloadSha1Sum(client, url)
	if err != nil {
		return err
	}

	// Compare the current version of ghost, if sha1 is the same, skip upgrading
	currentSha1sum, _ := GetFileSha1(exePath)
	if currentSha1sum == sha1sum {
		log.Println("Upgrade: ghost is already up-to-date, skipping upgrade")
		return nil
	}

	buffer, err := ghost.downloadUpgrade(client, url)
	if err != nil {
		return err
	}

	// Compare SHA1 sum
	if sha1sum != fmt.Sprintf("%x", sha1.Sum(buffer.Bytes())) {
		return errors.New("Upgrade: sha1sum mismatch, abort")
	}

	if err := ghost.installUpgrade(exePath, buffer); err != nil {
		return err
	}

	log.Println("Upgrade: restarting ghost...")
	os.Args[0] = exePath
	err = syscall.Exec(exePath, os.Args, os.Environ())
	if err != nil {
		return fmt.Errorf("Upgrade: exec: %s", err)
	}
	return nil
}

func (ghost *Ghost) handleTerminalRequest(req *Request) error {
	type RequestPayload struct {
		Sid       string `json:"sid"`
		TtyDevice string `json:"tty_device"`
	}

	var params RequestPayload
	if err := json.Unmarshal(req.Payload, &params); err != nil {
		return err
	}

	go func() {
		log.Printf("Received terminal command, Terminal agent %s spawned\n", params.Sid)
		addrs := []string{ghost.connectedAddr}
		// Terminal sessions are identified with session ID, thus we don't care
		// machine ID and can make them random.
		g := NewGhost(addrs, ghost.tls, ModeTerminal, RandomMID).
			SetSid(params.Sid).
			SetAllowlist(ghost.allowlist).
			SetTtyDevice(params.TtyDevice)
		g.Start(false, false)
	}()

	res := NewResponse(req.Rid, Success, nil)
	return ghost.SendResponse(res)
}

func (ghost *Ghost) handleShellRequest(req *Request) error {
	type RequestPayload struct {
		Sid string `json:"sid"`
		Cmd string `json:"command"`
	}

	var params RequestPayload
	if err := json.Unmarshal(req.Payload, &params); err != nil {
		return err
	}

	go func() {
		log.Printf("Received shell command: %s, Shell agent %s spawned\n",
			params.Cmd, params.Sid)
		addrs := []string{ghost.connectedAddr}
		// Shell sessions are identified with session ID, thus we don't care
		// machine ID and can make them random.
		g := NewGhost(addrs, ghost.tls, ModeShell, RandomMID).
			SetSid(params.Sid).
			SetAllowlist(ghost.allowlist).
			SetShellCommand(params.Cmd)
		g.Start(false, false)
	}()

	res := NewResponse(req.Rid, Success, nil)
	return ghost.SendResponse(res)
}

func (ghost *Ghost) handleListTreeRequest(req *Request) error {
	type RequestPayload struct {
		Path string `json:"path"`
	}

	var params RequestPayload
	if err := json.Unmarshal(req.Payload, &params); err != nil {
		return ghost.SendResponse(NewErrorResponse(req.Rid, "invalid_params"))
	}

	if !filepath.IsAbs(params.Path) {
		home, err := os.UserHomeDir()
		if err != nil {
			return ghost.SendResponse(NewErrorResponse(req.Rid, err.Error()))
		}
		params.Path = filepath.Join(home, params.Path)
	}

	entries, err := ghost.ListTree(params.Path)
	if err != nil {
		log.Printf("ListTree: %s\n", err)
		return ghost.SendResponse(NewErrorResponse(req.Rid, err.Error()))
	}
	return ghost.SendResponse(
		NewResponse(req.Rid, Success, entries))
}

func (ghost *Ghost) handleFstatRequest(req *Request) error {
	type RequestPayload struct {
		Path string `json:"path"`
	}

	var params RequestPayload
	if err := json.Unmarshal(req.Payload, &params); err != nil {
		return ghost.SendResponse(NewErrorResponse(req.Rid, "invalid_params"))
	}

	if !filepath.IsAbs(params.Path) {
		home, err := os.UserHomeDir()
		if err != nil {
			return ghost.SendResponse(NewErrorResponse(req.Rid, err.Error()))
		}
		params.Path = filepath.Join(home, params.Path)
	}

	fileInfo, err := os.Stat(params.Path)
	result := map[string]interface{}{
		"exists": err == nil,
	}
	if err == nil {
		result["is_dir"] = fileInfo.IsDir()
		result["perm"] = fileInfo.Mode().Perm()
		result["size"] = fileInfo.Size()
		result["mtime"] = fileInfo.ModTime().Unix()
		isSymlink := (fileInfo.Mode() & os.ModeSymlink) != 0
		result["is_symlink"] = isSymlink

		if isSymlink {
			target, err := os.Readlink(params.Path)
			if err == nil {
				result["link_target"] = target
			}
		}
	} else if os.IsNotExist(err) {
		// File doesn't exist, but this is not an error condition
		// We already set exists: false
	} else {
		return ghost.SendResponse(NewErrorResponse(req.Rid, err.Error()))
	}
	return ghost.SendResponse(NewResponse(req.Rid, Success, result))
}

func (ghost *Ghost) handleFileDownloadRequest(req *Request) error {
	type RequestPayload struct {
		Sid      string `json:"sid"`
		Filename string `json:"filename"`
	}

	var params RequestPayload
	if err := json.Unmarshal(req.Payload, &params); err != nil {
		return err
	}

	filename := params.Filename
	if !strings.HasPrefix(filename, "/") {
		home := os.Getenv("HOME")
		if home == "" {
			home = TmpDir
		}
		filename = filepath.Join(home, filename)
	}

	f, err := os.Open(filename)
	if err != nil {
		res := NewErrorResponse(req.Rid, err.Error())
		return ghost.SendResponse(res)
	}
	f.Close()

	go func() {
		log.Printf("Received file_download command, File agent %s spawned\n", params.Sid)
		addrs := []string{ghost.connectedAddr}
		g := NewGhost(addrs, ghost.tls, ModeFile, RandomMID).
			SetSid(params.Sid).
			SetAllowlist(ghost.allowlist).
			SetFileOp("download", filename, 0)
		g.Start(false, false)
	}()

	res := NewResponse(req.Rid, Success, nil)
	return ghost.SendResponse(res)
}

func (ghost *Ghost) handleFileUploadRequest(req *Request) error {
	type RequestPayload struct {
		Sid         string `json:"sid"`
		TerminalSid string `json:"terminal_sid"`
		Filename    string `json:"filename"`
		Dest        string `json:"dest"`
		Perm        int    `json:"perm"`
		CheckOnly   bool   `json:"check_only"`
	}

	var params RequestPayload
	if err := json.Unmarshal(req.Payload, &params); err != nil {
		return err
	}

	targetDir := os.Getenv("HOME")
	if targetDir == "" {
		targetDir = TmpDir
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
			if pid, ok := ghost.terminalSid2Pid.Load(params.TerminalSid); ok {
				cwd, err := GetProcessWorkingDirectory(pid.(int))
				if err == nil {
					targetDir = cwd
				}
			}
		}
		destPath = filepath.Join(targetDir, params.Filename)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		log.Printf("Failed to create directory: %v", err)
		return err
	}

	f, err := os.Create(destPath)
	if err != nil {
		res := NewErrorResponse(req.Rid, err.Error())
		return ghost.SendResponse(res)
	}
	f.Close()

	if err := os.Chmod(destPath, os.FileMode(params.Perm)); err != nil {
		log.Printf("Failed to set file permissions: %v", err)
		// Don't return error here as the file was created successfully
	}

	// If not check_only, spawn ModeFile mode ghost agent to handle upload
	if !params.CheckOnly {
		go func() {
			log.Printf("Received file_upload command, File agent %s spawned\n",
				params.Sid)
			addrs := []string{ghost.connectedAddr}
			g := NewGhost(addrs, ghost.tls, ModeFile, RandomMID).
				SetSid(params.Sid).
				SetAllowlist(ghost.allowlist).
				SetFileOp("upload", destPath, params.Perm)
			g.Start(false, false)
		}()
	}

	res := NewResponse(req.Rid, Success, nil)
	return ghost.SendResponse(res)
}

func (ghost *Ghost) handleModeForwardRequest(req *Request) error {
	type RequestPayload struct {
		Sid  string `json:"sid"`
		Host string `json:"host"`
		Port int    `json:"port"`
	}

	var params RequestPayload
	if err := json.Unmarshal(req.Payload, &params); err != nil {
		return err
	}

	go func() {
		log.Printf("Received forward command, ModeForward agent %s spawned\n", params.Sid)
		addrs := []string{ghost.connectedAddr}
		g := NewGhost(addrs, ghost.tls, ModeForward, RandomMID).
			SetSid(params.Sid).
			SetAllowlist(ghost.allowlist).
			SetForwardTarget(params.Host, params.Port)
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

	if _, err := io.Copy(ghost.Conn, file); err != nil {
		log.Printf("Failed to copy file to connection: %v", err)
		return err
	}
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
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			log.Printf("Failed to create directory %s: %v", dirPath, err)
			return err
		}
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
		if _, err := file.Write(buffer); err != nil {
			log.Printf("Failed to write to file: %v", err)
			return err
		}
	}

	if ghost.fileOp.Perm > 0 {
		if err := file.Chmod(os.FileMode(ghost.fileOp.Perm)); err != nil {
			log.Printf("Failed to set file permissions: %v", err)
			// Don't return error here as the file was written successfully
		}
	}

	return nil
}

func (ghost *Ghost) handleCreateSymlinkRequest(req *Request) error {
	type RequestPayload struct {
		Target string `json:"target"`
		Dest   string `json:"dest"`
	}

	var params RequestPayload
	if err := json.Unmarshal(req.Payload, &params); err != nil {
		return ghost.SendResponse(NewErrorResponse(req.Rid, "invalid_params"))
	}

	// Create parent directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(params.Dest), 0755); err != nil {
		return ghost.SendResponse(NewErrorResponse(req.Rid, err.Error()))
	}

	// Remove existing file/link if it exists
	if _, err := os.Lstat(params.Dest); err == nil {
		if err := os.Remove(params.Dest); err != nil {
			return ghost.SendResponse(NewErrorResponse(req.Rid, err.Error()))
		}
	}

	// Create the symlink
	if err := os.Symlink(params.Target, params.Dest); err != nil {
		return ghost.SendResponse(NewErrorResponse(req.Rid, err.Error()))
	}

	return ghost.SendResponse(NewResponse(req.Rid, Success, nil))
}

func (ghost *Ghost) handleMkdirRequest(req *Request) error {
	type RequestPayload struct {
		Path string `json:"path"`
		Perm int    `json:"perm"`
	}

	var params RequestPayload
	if err := json.Unmarshal(req.Payload, &params); err != nil {
		return ghost.SendResponse(NewErrorResponse(req.Rid, "invalid_params"))
	}

	if !filepath.IsAbs(params.Path) {
		home, err := os.UserHomeDir()
		if err != nil {
			return ghost.SendResponse(NewErrorResponse(req.Rid, err.Error()))
		}
		params.Path = filepath.Join(home, params.Path)
	}

	// Create the directory with the specified permissions
	if err := os.MkdirAll(params.Path, os.FileMode(params.Perm)); err != nil {
		return ghost.SendResponse(NewErrorResponse(req.Rid, err.Error()))
	}

	// Set the permissions again as MkdirAll might not set them correctly for all directories
	if err := os.Chmod(params.Path, os.FileMode(params.Perm)); err != nil {
		return ghost.SendResponse(NewErrorResponse(req.Rid, err.Error()))
	}

	return ghost.SendResponse(NewResponse(req.Rid, Success, nil))
}

func (ghost *Ghost) handleRequest(req *Request) error {
	var err error
	switch req.Name {
	case CmdUpgrade:
		err = ghost.Upgrade()
	case CmdTerminal:
		err = ghost.handleTerminalRequest(req)
	case CmdShell:
		err = ghost.handleShellRequest(req)
	case CmdListTree:
		err = ghost.handleListTreeRequest(req)
	case CmdFstat:
		err = ghost.handleFstatRequest(req)
	case CmdFileDownload:
		err = ghost.handleFileDownloadRequest(req)
	case CmdClearToDownload:
		err = ghost.StartDownloadServer()
	case CmdFileUpload:
		err = ghost.handleFileUploadRequest(req)
	case CmdForward:
		err = ghost.handleModeForwardRequest(req)
	case CmdCreateSymlink:
		err = ghost.handleCreateSymlinkRequest(req)
	case CmdMkdir:
		err = ghost.handleMkdirRequest(req)
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

func (ghost *Ghost) handleTTYControl(tty *os.File, data []byte) ([]byte, error) {
	// Parse ANSI escape sequences
	matches := escapeSeqRe.FindSubmatch(data)
	if len(matches) == 0 {
		// Consume the first two bytes so we won't process it again.
		if _, err := tty.Write(data[:2]); err != nil {
			log.Printf("Failed to write to tty: %v", err)
			return data[2:], err
		}
		return data[2:], nil
	}

	args := string(matches[1])
	command := string(matches[2])

	if command == "t" {
		params := strings.Split(args, ";")
		if len(params) >= 3 && params[0] == "8" {
			// Window size in characters
			rows, _ := strconv.Atoi(params[1])
			cols, _ := strconv.Atoi(params[2])

			log.Printf("Terminal resize request received: rows=%d, cols=%d", rows, cols)

			ws := &struct {
				Row    uint16
				Col    uint16
				Xpixel uint16
				Ypixel uint16
			}{
				Row:    uint16(rows),
				Col:    uint16(cols),
				Xpixel: 0,
				Ypixel: 0,
			}

			ret, _, err := syscall.Syscall(
				syscall.SYS_IOCTL,
				tty.Fd(),
				syscall.TIOCSWINSZ,
				uintptr(unsafe.Pointer(ws)),
			)
			if ret == ^uintptr(0) {
				return nil, fmt.Errorf("handleTTYControl: TIOCSWINSZ failed: %v", err)
			}
			return data[len(matches[0]):], nil
		}
	}
	if _, err := tty.Write(matches[0]); err != nil {
		log.Printf("Failed to write to tty: %v", err)
		return data[len(matches[0]):], err
	}
	return data[len(matches[0]):], nil
}

// SpawnTTYServer Spawns a TTY server and forward I/O to the TCP socket.
// Helper functions for SpawnTTYServer
func (ghost *Ghost) setupPTYEnvironment() (string, string) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = defaultShell
	}

	home := os.Getenv("HOME")
	if home == "" {
		home = "/root"
	}

	// Add ghost executable to PATH
	exePath, err := os.Executable()
	if err == nil {
		os.Setenv("PATH", fmt.Sprintf("%s:%s", filepath.Dir(exePath),
			os.Getenv("PATH")))
	}

	if err := os.Chdir(home); err != nil {
		log.Printf("Failed to change directory to %s: %v", home, err)
		// Continue execution as this is not critical
	}

	return shell, home
}

func (ghost *Ghost) startPTYProcess(shell string) (*os.File, *exec.Cmd, error) {
	cmd := exec.Command(shell)
	tty, err := pty.Start(cmd)
	if err != nil {
		return nil, nil, errors.New(`SpawnTTYServer: Cannot start "` + shell + `", abort`)
	}

	// Register the mapping of sid and ttyName
	ttyName, err := termios.Ptsname(tty.Fd())
	if err != nil {
		return tty, cmd, err
	}

	client, err := ghostRPCStubServer()

	// Ghost could be launched without RPC server, ignore registration
	if err == nil {
		err = client.Call("rpc.RegisterTTY", []string{ghost.sid, ttyName},
			&EmptyReply{})
		if err != nil {
			return tty, cmd, err
		}

		err = client.Call("rpc.RegisterSession", []string{
			ghost.sid, strconv.Itoa(cmd.Process.Pid)}, &EmptyReply{})
		if err != nil {
			return tty, cmd, err
		}
	}

	return tty, cmd, nil
}

func (ghost *Ghost) configureTTYDevice(tty *os.File) error {
	var term unix.Termios
	err := termios.Tcgetattr(tty.Fd(), &term)
	if err != nil {
		return nil
	}

	termios.Cfmakeraw(&term)
	term.Iflag &^= (unix.IXON | unix.IXOFF) // Disable software flow control
	term.Cflag |= unix.CLOCAL               // Ignore modem control lines
	term.Cflag &^= unix.CRTSCTS             // Disable hardware flow control

	return termios.Tcsetattr(tty.Fd(), termios.TCSANOW, &term)
}

func (ghost *Ghost) createTTYInputFeeder(tty *os.File) func([]byte) error {
	return func(buffer []byte) error {
		for {
			escapeStart := bytes.Index(buffer, []byte{0x1b, '['})
			if escapeStart == -1 {
				break
			}

			if _, err := tty.Write(buffer[:escapeStart]); err != nil {
				log.Printf("Failed to write to tty: %v", err)
			}
			rest, err := ghost.handleTTYControl(tty, buffer[escapeStart:])
			if err != nil {
				log.Printf("SpawnTTYServer: Error handling TTY control: %v", err)
				return err
			}
			buffer = rest
		}

		_, err := tty.Write(buffer)
		if err != nil {
			log.Printf("SpawnTTYServer: Error writing to TTY: %v", err)
			return err
		}
		return nil
	}
}

// Additional helper functions for SpawnTTYServer
func (ghost *Ghost) setupPTYConnection(stopConn chan struct{}) (*os.File, *exec.Cmd, error) {
	// No TTY device specified, open a PTY (pseudo terminal) instead.
	shell, _ := ghost.setupPTYEnvironment()
	tty, cmd, err := ghost.startPTYProcess(shell)
	if err != nil {
		return nil, nil, err
	}

	go func() {
		if _, err := io.Copy(ghost.Conn, tty); err != nil {
			log.Printf("Error copying from tty to connection: %v", err)
		}
		if err := cmd.Wait(); err != nil {
			log.Printf("Error waiting for command: %v", err)
		}
		close(stopConn)
	}()

	return tty, cmd, nil
}

func (ghost *Ghost) setupTTYDeviceConnection(stopConn chan struct{}) (*os.File, error) {
	// Open a TTY device
	tty, err := os.OpenFile(ghost.ttyDevice, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	if err = ghost.configureTTYDevice(tty); err != nil {
		return nil, err
	}

	go func() {
		if _, err := io.Copy(ghost.Conn, tty); err != nil {
			log.Printf("Error copying from tty to connection: %v", err)
		}
		close(stopConn)
	}()

	return tty, nil
}

func (ghost *Ghost) runTTYMainLoop(tty *os.File, stopConn chan struct{}) error {
	feedInput := ghost.createTTYInputFeeder(tty)

	if err := feedInput(ghost.ReadBuffer); err != nil {
		log.Printf("Error feeding input to TTY: %v", err)
	}

	for {
		select {
		case buffer := <-ghost.readChan:
			if err := feedInput(buffer); err != nil {
				return err
			}
		case err := <-ghost.readErrChan:
			if err == io.EOF {
				log.Println("SpawnTTYServer: connection terminated")
				return nil
			}
			return err
		case <-stopConn:
			return nil
		}
	}
}

func (ghost *Ghost) SpawnTTYServer(res *Response) error {
	log.Println("SpawnTTYServer: started")

	var tty *os.File
	var cmd *exec.Cmd
	var err error
	stopConn := make(chan struct{})

	defer func() {
		ghost.quit = true
		if tty != nil {
			tty.Close()
		}
		// Kill the PTY process if it exists
		if cmd != nil && cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil {
				log.Printf("Failed to kill process: %v", err)
			}
		}
		ghost.Conn.Close()
		log.Println("SpawnTTYServer: terminated")
	}()

	if ghost.ttyDevice == "" {
		tty, cmd, err = ghost.setupPTYConnection(stopConn)
	} else {
		tty, err = ghost.setupTTYDeviceConnection(stopConn)
	}

	if err != nil {
		return err
	}

	return ghost.runTTYMainLoop(tty, stopConn)
}

// Helper functions for SpawnShellServer
func (ghost *Ghost) setupShellEnvironment() {
	// Execute shell command from HOME directory
	home := os.Getenv("HOME")
	if home == "" {
		home = "/tmp"
	}
	if err := os.Chdir(home); err != nil {
		log.Printf("Failed to change directory to %s: %v", home, err)
	}

	// Add ghost executable to PATH
	exePath, err := os.Executable()
	if err == nil {
		os.Setenv("PATH", fmt.Sprintf("%s:%s", os.Getenv("PATH"),
			filepath.Dir(exePath)))
	}
}

func (ghost *Ghost) setupShellCommand(stopConn chan struct{}) (*exec.Cmd, io.WriteCloser, error) {
	cmd := exec.Command(defaultShell, "-c", ghost.shellCommand)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}

	// Handle initial buffer
	if len(ghost.ReadBuffer) > 0 {
		if _, err := stdin.Write(ghost.ReadBuffer); err != nil {
			log.Printf("Failed to write to stdin: %v", err)
		}
		ghost.ReadBuffer = nil
	}

	// Start output copying goroutines
	go func() {
		if _, err := io.Copy(ghost.Conn, stdout); err != nil {
			log.Printf("Error copying stdout to connection: %v", err)
		}
	}()
	go func() {
		if _, err := io.Copy(ghost.Conn, stderr); err != nil {
			log.Printf("Error copying stderr to connection: %v", err)
		}
		close(stopConn)
	}()

	return cmd, stdin, nil
}

func (ghost *Ghost) cleanupShellProcess(cmd *exec.Cmd) {
	time.Sleep(100 * time.Millisecond) // Wait for process to terminate

	process := NewPollableProcess(cmd.Process)
	_, err := process.Poll()
	// Check if the process is terminated. If not, send SIGTERM to
	// the process, then wait for 1 second.  Send another SIGKILL to make sure
	// the process is terminated.
	if err != nil {
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			log.Printf("Failed to send SIGTERM: %v", err)
		}
		time.Sleep(time.Second)
		if err := cmd.Process.Kill(); err != nil {
			log.Printf("Failed to kill process: %v", err)
		}
		if err := cmd.Wait(); err != nil {
			log.Printf("Error waiting for command: %v", err)
		}
	}
}

func (ghost *Ghost) handleShellInput(stdin io.WriteCloser, buf []byte) {
	if len(buf) >= len(stdinClosed)*2 {
		idx := bytes.Index(buf, []byte(stdinClosed+stdinClosed))
		if idx != -1 {
			if _, err := stdin.Write(buf[:idx]); err != nil {
				log.Printf("Failed to write to stdin: %v", err)
			}
			stdin.Close()
			return
		}
	}
	if _, err := stdin.Write(buf); err != nil {
		log.Printf("Failed to write to stdin: %v", err)
	}
}

// SpawnShellServer spawns a Shell server and forward input/output from/to the
// TCP socket.
func (ghost *Ghost) SpawnShellServer(res *Response) error {
	log.Println("SpawnShellServer: started")

	var err error

	defer func() {
		ghost.quit = true
		if err != nil {
			if _, werr := ghost.Conn.Write([]byte(err.Error() + "\n")); werr != nil {
				log.Printf("Failed to write error to connection: %v", werr)
			}
		}
		ghost.Conn.Close()
		log.Println("SpawnShellServer: terminated")
	}()

	ghost.setupShellEnvironment()

	stopConn := make(chan struct{})

	cmd, stdin, err := ghost.setupShellCommand(stopConn)
	if err != nil {
		return err
	}

	if err = cmd.Start(); err != nil {
		return err
	}

	defer ghost.cleanupShellProcess(cmd)

	for {
		select {
		case buf := <-ghost.readChan:
			ghost.handleShellInput(stdin, buf)
		case err := <-ghost.readErrChan:
			if err == io.EOF {
				log.Println("SpawnShellServer: connection terminated")
				return nil
			}
			log.Printf("SpawnShellServer: %s\n", err)
			return err
		case <-stopConn:
			return nil
		}
	}
}

// InitiatefileOperation initiates a file operation.
// The operation could either be 'download' or 'upload'
// This function starts handshake with overlord then execute download sequence.
func (ghost *Ghost) InitiatefileOperation(res *Response) error {
	switch ghost.fileOp.Action {
	case "download":
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
	case "upload":
		ghost.uploadContext.Ready = true
		req := NewRequest("clear_to_upload", nil)
		req.SetTimeout(-1)
		err := ghost.SendRequest(req, nil)
		if err != nil {
			return err
		}
		go func() {
			if err := ghost.StartUploadServer(); err != nil {
				log.Printf("Error starting upload server: %v", err)
			}
		}()
		return nil
	default:
		return errors.New("InitiatefileOperation: unknown file operation, ignored")
	}
}

// SpawnPortForwardServer spawns a port forwarding server and forward I/O to
// the TCP socket.
func (ghost *Ghost) SpawnPortForwardServer(res *Response) error {
	log.Println("SpawnPortForwardServer: started")

	var err error

	defer func() {
		ghost.quit = true
		if err != nil {
			if _, werr := ghost.Conn.Write([]byte(err.Error() + "\n")); werr != nil {
				log.Printf("Failed to write error to connection: %v", werr)
			}
		}
		ghost.Conn.Close()
		log.Println("SpawnPortForwardServer: terminated")
	}()

	conn, err := net.DialTimeout("tcp",
		fmt.Sprintf("%s:%d", ghost.host, ghost.port), connectTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	stopConn := make(chan struct{})

	if len(ghost.ReadBuffer) > 0 {
		if _, err := conn.Write(ghost.ReadBuffer); err != nil {
			log.Printf("Failed to write to connection: %v", err)
		}
		ghost.ReadBuffer = nil
	}

	go func() {
		if _, err := io.Copy(ghost.Conn, conn); err != nil {
			log.Printf("Error copying from connection: %v", err)
		}
		close(stopConn)
	}()

	for {
		select {
		case buf := <-ghost.readChan:
			if _, err := conn.Write(buf); err != nil {
				log.Printf("Failed to write to connection: %v", err)
			}
		case err := <-ghost.readErrChan:
			if err == io.EOF {
				log.Println("SpawnPortForwardServer: connection terminated")
				return nil
			}
			return err
		case <-stopConn:
			return nil
		}
	}
}

// Register existent to Overlord.
// Helper functions for Register
func (ghost *Ghost) determineTLSSettings(addr string) error {
	// Check if server has TLS enabled.
	// Only control channel needs to determine if TLS is enabled. Other mode
	// should use the tlsSettings passed in when it was spawned.
	if ghost.mode == ModeControl {
		var enabled bool
		var err error

		switch ghost.tlsMode {
		case TLSDetect:
			enabled, err = ghost.tlsEnabled(addr)
			if err != nil {
				return err
			}
		case TLSForceEnable:
			enabled = true
		case TLSForceDisable:
			enabled = false
		}

		ghost.tls.SetEnabled(enabled)
	}
	return nil
}

func (ghost *Ghost) createWebSocketConnection(addr string) (net.Conn, error) {
	proto := "ws"
	if ghost.tls.Enabled {
		proto = "wss"
	}
	uri := fmt.Sprintf("%s://%s/connect", proto, addr)

	dialer := websocket.DefaultDialer
	if ghost.tls.Config != nil {
		dialer = &websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 45 * time.Second,
			TLSClientConfig:  ghost.tls.Config,
		}
	}

	wsConn, _, err := dialer.Dial(uri, http.Header{})
	if err != nil {
		return nil, err
	}

	return wsConn.UnderlyingConn(), nil
}

func (ghost *Ghost) createRegistrationHandler(addr string) ResponseHandler {
	return func(res *Response) error {
		if res == nil {
			ghost.reset = true
			return errors.New("Register request timeout")
		} else if res.Status != Success {
			log.Println("Register:", res.Status)
		} else {
			log.Printf("Registered with Overlord at %s", addr)
			ghost.connectedAddr = addr
			if err := ghost.Upgrade(); err != nil {
				log.Println(err)
			}
			ghost.pauseLanDisc = true
		}
		ghost.RegisterStatus = res.Status
		return nil
	}
}

func (ghost *Ghost) getResponseHandler(addr string) ResponseHandler {
	switch ghost.mode {
	case ModeControl:
		return ghost.createRegistrationHandler(addr)
	case ModeTerminal:
		return ghost.SpawnTTYServer
	case ModeShell:
		return ghost.SpawnShellServer
	case ModeFile:
		return ghost.InitiatefileOperation
	case ModeForward:
		return ghost.SpawnPortForwardServer
	default:
		return nil
	}
}

func (ghost *Ghost) Register() error {
	for _, addr := range ghost.addrs {
		log.Printf("Trying %s ...\n", addr)
		ghost.Reset()

		if err := ghost.determineTLSSettings(addr); err != nil {
			continue
		}

		conn, err := ghost.createWebSocketConnection(addr)
		if err != nil {
			log.Printf("error: %s\n", err)
			continue
		}

		log.Println("Connection established, registering...")

		ghost.Conn = conn
		req := NewRequest("register", map[string]interface{}{
			"mid":        ghost.mid,
			"sid":        ghost.sid,
			"mode":       ghost.mode,
			"properties": ghost.properties,
		})

		handler := ghost.getResponseHandler(addr)
		return ghost.SendRequest(req, handler)
	}

	return errors.New("cannot connect to any server")
}

// InitiateDownload initiates a client-initiated download request.
func (ghost *Ghost) InitiateDownload(info downloadInfo) {
	go func() {
		addrs := []string{ghost.connectedAddr}

		val, ok := ghost.ttyName2Sid.Load(info.Ttyname)
		if !ok {
			log.Printf("Failed to get SID")
			return
		}

		g := NewGhost(addrs, ghost.tls, ModeFile, RandomMID).
			SetAllowlist(ghost.allowlist).
			SetTerminalSid(val.(string)).
			SetFileOp("download", info.Filename, 0)
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
// Helper functions for Listen
func (ghost *Ghost) handleIncomingBuffer(buffer []byte) error {
	if ghost.uploadContext.Ready {
		if len(ghost.ReadBuffer) > 0 {
			// Write the leftover from previous ReadBuffer
			ghost.uploadContext.Data <- ghost.ReadBuffer
			ghost.ReadBuffer = nil
		}
		ghost.uploadContext.Data <- buffer
		return nil
	}
	reqs := ghost.ParseRequests(buffer, ghost.RegisterStatus != Success)
	if ghost.quit {
		return io.EOF // Signal to exit
	}
	if err := ghost.processRequests(reqs); err != nil {
		log.Println(err)
	}
	return nil
}

func (ghost *Ghost) handleReadError(err error) error {
	if err == io.EOF {
		if ghost.uploadContext.Ready {
			ghost.uploadContext.Data <- nil
			ghost.quit = true
			return nil
		}
		return errors.New("connection dropped")
	}
	return err
}

func (ghost *Ghost) handlePingTick() {
	if ghost.mode == ModeControl {
		if err := ghost.Ping(); err != nil {
			log.Printf("Ping failed: %v", err)
		}
	}
}

func (ghost *Ghost) handleTimeoutTick() error {
	err := ghost.ScanForTimeoutRequests()
	if ghost.reset {
		if err == nil {
			err = errors.New("reset request")
		}
		return err
	}
	return nil
}

func (ghost *Ghost) Listen() error {
	readChan, readErrChan := ghost.SpawnReaderRoutine()
	pingTicker := time.NewTicker(pingInterval)
	reqTicker := time.NewTicker(timeoutCheckInterval)

	ghost.readChan = readChan
	ghost.readErrChan = readErrChan

	defer func() {
		ghost.StopConn()
		ghost.pauseLanDisc = false
	}()

	for {
		select {
		case buffer := <-readChan:
			if err := ghost.handleIncomingBuffer(buffer); err != nil {
				if err == io.EOF {
					return nil // Clean exit
				}
				return err
			}
		case err := <-readErrChan:
			return ghost.handleReadError(err)
		case info := <-ghost.downloadQueue:
			ghost.InitiateDownload(info)
		case <-pingTicker.C:
			ghost.handlePingTick()
		case <-reqTicker.C:
			if err := ghost.handleTimeoutTick(); err != nil {
				return err
			}
		}
	}
}

// RegisterTTY register the TTY to a session.
func (ghost *Ghost) RegisterTTY(sesssionID, ttyName string) {
	ghost.ttyName2Sid.Store(ttyName, sesssionID)
}

// RegisterSession register the PID to a session.
func (ghost *Ghost) RegisterSession(sesssionID, pidStr string) {
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		panic(err)
	}
	ghost.terminalSid2Pid.Store(sesssionID, pid)
}

// AddToDownloadQueue adds a downloadInfo to the download queue
func (ghost *Ghost) AddToDownloadQueue(ttyName, filename string) {
	ghost.downloadQueue <- downloadInfo{ttyName, filename}
}

// ListTree returns a recursive list of all files and directories under the given path
func (ghost *Ghost) ListTree(path string) ([]map[string]interface{}, error) {
	entries := []map[string]interface{}{}
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		isSymlink := (info.Mode() & os.ModeSymlink) != 0
		entry := map[string]interface{}{
			"path":       filePath,
			"size":       info.Size(),
			"perm":       info.Mode().Perm(),
			"mtime":      info.ModTime().Unix(),
			"is_dir":     info.IsDir(),
			"is_symlink": isSymlink,
		}

		if isSymlink {
			target, err := os.Readlink(filePath)
			if err == nil {
				entry["link_target"] = target
			}
		}
		entries = append(entries, entry)
		return nil
	})
	return entries, err
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
		if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			log.Printf("Failed to set read deadline: %v", err)
		}
		n, remote, err := conn.ReadFrom(buf)

		if ghost.pauseLanDisc {
			log.Println("LAN discovery: paused")
			ticker := time.NewTicker(readTimeout)
		waitLoop:
			for range ticker.C {
				if !ghost.pauseLanDisc {
					break waitLoop
				}
			}
			log.Println("LAN discovery: resumed")
			continue
		}

		if err != nil {
			continue
		}

		// LAN discovery packet format: "OVERLORD [host]:port"
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
	if _, err := io.WriteString(conn, "HTTP/1.1 200\n"); err != nil {
		log.Printf("Failed to write HTTP response: %v", err)
		return
	}
	if _, err := io.WriteString(conn, "Content-Type: application/json-rpc\n\n"); err != nil {
		log.Printf("Failed to write HTTP headers: %v", err)
		return
	}
	ghost.server.ServeCodec(jsonrpc.NewServerCodec(conn))
}

// StartRPCServer starts a local RPC server used for communication between
// ghost instances.
func (ghost *Ghost) StartRPCServer() {
	log.Println("RPC Server: started")

	ghost.server = rpc.NewServer()
	if err := ghost.server.RegisterName("rpc", &ghostRPCStub{ghost}); err != nil {
		log.Printf("Failed to register RPC service: %v", err)
	}

	http.Handle("/", ghost)
	err := http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", ghostRPCStubPort), nil)
	if err != nil {
		log.Fatalf("Unable to listen at port %d: %s\n", ghostRPCStubPort, err)
	}
}

// ScanGateway scans current network gateway and add it into addrs if not
// already exist.
func (ghost *Ghost) ScanGateway() {
	if gateways, err := GetGateWayIP(); err == nil {
		for _, gw := range gateways {
			addr := fmt.Sprintf("%s:%d", gw, DefaultHTTPSPort)
			if !ghost.existsInAddr(addr) {
				ghost.addrs = append(ghost.addrs, addr)
			}
			addr = fmt.Sprintf("%s:%d", gw, DefaultHTTPPort)
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

// Returns a ghostRPCStub client object which can be used to call ghostRPCStub
// methods.
func ghostRPCStubServer() (*rpc.Client, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", ghostRPCStubPort))
	if err != nil {
		return nil, err
	}

	if _, err := io.WriteString(conn, "GET / HTTP/1.1\nHost: 127.0.0.1\n\n"); err != nil {
		return nil, err
	}
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
	tlsCertFile string, verify bool, allowlist string, propFile string,
	download string, reset bool, status bool, tlsMode int) {
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
		if !strings.Contains(args[0], ":") {
			addrs = append(addrs,
				fmt.Sprintf("%s:%d", args[0], DefaultHTTPSPort),
				fmt.Sprintf("%s:%d", args[0], DefaultHTTPPort))
		} else {
			addrs = append(addrs, args[0])
		}
	}
	addrs = append(addrs,
		fmt.Sprintf("127.0.0.1:%d", DefaultHTTPSPort),
		fmt.Sprintf("127.0.0.1:%d", DefaultHTTPPort))

	tlsSettings := newTLSSettings(tlsCertFile, verify)

	if propFile != "" {
		var err error
		propFile, err = filepath.Abs(propFile)
		if err != nil {
			log.Println("propFile:", err)
			os.Exit(1)
		}
	}

	g := NewGhost(addrs, tlsSettings, ModeControl, mid).
		SetAllowlist(allowlist).
		SetPropFile(propFile).
		SetTLSMode(tlsMode)
	go g.Start(!noLanDisc, !noRPCServer)

	ticker := time.NewTicker(60 * time.Second)

	for range ticker.C {
		log.Printf("Num of Goroutines: %d\n", runtime.NumGoroutine())
	}
}
