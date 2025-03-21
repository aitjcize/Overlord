// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/aitjcize/Overlord/overlord"
	"golang.org/x/term"
)

var (
	bindAddr = flag.String("bind",
		"0.0.0.0", "specify alternate bind address")
	port = flag.Int("port",
		0, "alternate port listen instead of standard ports (http:80, https:443)")
	lanDiscInterface = flag.String("lan-disc-iface",
		"", "the network interface used for broadcasting LAN discovery packets")
	noLanDisc = flag.Bool("no-lan-disc",
		false, "disable LAN discovery broadcasting")
	tlsCerts = flag.String("tls",
		"", "TLS certificates in the form of 'cert.pem,key.pem'. Empty to disable.")
	noLinkTLS = flag.Bool("no-link-tls",
		false, "disable TLS between ghost and overlord. Only valid when TLS is enabled.")
	dbPath = flag.String("db-path",
		"overlord.db", "the path to the SQLite database file for user, group, and authentication data")
	initializeDB = flag.Bool("init",
		false, "Initialize the database with a custom admin user and password and generate a JWT secret")
	adminUser = flag.String("admin-user",
		"", "Admin username for database initialization (only used with -init)")
	adminPass = flag.String("admin-pass",
		"", "Admin password for database initialization (only used with -init)")
)

func usage() {
	prog := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", prog)
	fmt.Fprintln(os.Stderr, "Options:")
	flag.PrintDefaults()
	os.Exit(1)
}

func promptForInput(prompt string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func promptForPassword(prompt string) string {
	fmt.Print(prompt)
	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println() // Add a newline after the password input

	if err != nil {
		panic(err)
	}
	return string(password)
}

func initializeDatabase(dbPath string) error {
	adminUsername := *adminUser
	adminPassword := *adminPass

	// If admin username is not provided via command line, prompt for it
	if adminUsername == "" {
		adminUsername = promptForInput("Enter admin username [admin]: ")
		if adminUsername == "" {
			adminUsername = "admin"
		}
	}

	// If admin password is not provided via command line, prompt for it
	if adminPassword == "" {
		for {
			adminPassword = promptForPassword("Enter admin password: ")
			if adminPassword == "" {
				fmt.Println("Password cannot be empty, please try again.")
				continue
			}
			break
		}
	} else if adminPassword == "" {
		return fmt.Errorf("password cannot be empty")
	}

	dbManager := overlord.NewDatabaseManager(dbPath)

	err := dbManager.Initialize(adminUsername, adminPassword)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %v", err)
	}

	fmt.Println("Database initialization complete.")
	fmt.Println("You can now start the server without the -init flag.")
	return nil
}

func checkDatabaseInitialized(dbPath string) bool {
	// Simple check - if the file exists and has data, consider it initialized
	info, err := os.Stat(dbPath)
	if err != nil || info.Size() == 0 {
		return false
	}
	return true
}

func main() {
	flag.Parse()

	if len(flag.Args()) > 0 {
		fmt.Fprintf(os.Stderr, "Error: unknown argument: %s\n", flag.Args()[0])
		usage()
	}

	// Validate required flags
	if *dbPath == "" {
		fmt.Fprintf(os.Stderr, "Error: -db-path is required\n")
		usage()
	}

	// Initialize the database if requested
	if *initializeDB {
		if checkDatabaseInitialized(*dbPath) {
			fmt.Fprintf(os.Stderr, "Error: Database already initialized\n")
			os.Exit(1)
		}
		if err := initializeDatabase(*dbPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing database: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Check if the database is initialized
	if !checkDatabaseInitialized(*dbPath) {
		fmt.Fprintf(os.Stderr, "Error: Database not initialized. Run with -init to initialize\n")
		os.Exit(1)
	}

	overlord.StartOverlord(*bindAddr, *port, *lanDiscInterface, !*noLanDisc,
		*tlsCerts, !*noLinkTLS, *dbPath)
}
