// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"bufio"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	maxFailCount  = 5
	blockDuration = 24 * time.Hour
)

func getRequestIP(r *http.Request) string {
	idx := strings.LastIndex(r.RemoteAddr, ":")
	return r.RemoteAddr[:idx]
}

type basicAuthHTTPHandlerDecorator struct {
	auth        *BasicAuth
	handler     http.Handler
	handlerFunc http.HandlerFunc
	blockedIps  map[string]time.Time
	failedCount map[string]int
}

func (auth *basicAuthHTTPHandlerDecorator) Unauthorized(w http.ResponseWriter, r *http.Request,
	msg string, record bool) {

	// Record failure
	if record {
		ip := getRequestIP(r)
		if _, ok := auth.failedCount[ip]; !ok {
			auth.failedCount[ip] = 0
		}
		if ip != "127.0.0.1" {
			// Only count for non-trusted IP.
			auth.failedCount[ip]++
		}

		log.Printf("BasicAuth: IP %s failed to login, count: %d\n", ip,
			auth.failedCount[ip])

		if auth.failedCount[ip] >= maxFailCount {
			auth.blockedIps[ip] = time.Now()
			log.Printf("BasicAuth: IP %s is blocked\n", ip)
		}
	}

	w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=%s", auth.auth.Realm))
	http.Error(w, fmt.Sprintf("%s: %s", http.StatusText(http.StatusUnauthorized),
		msg), http.StatusUnauthorized)
}

func (auth *basicAuthHTTPHandlerDecorator) IsBlocked(r *http.Request) bool {
	ip := getRequestIP(r)

	if t, ok := auth.blockedIps[ip]; ok {
		if time.Now().Sub(t) < blockDuration {
			log.Printf("BasicAuth: IP %s attempted to login, blocked\n", ip)
			return true
		}
		// Unblock the user because of timeout
		delete(auth.failedCount, ip)
		delete(auth.blockedIps, ip)
	}
	return false
}

func (auth *basicAuthHTTPHandlerDecorator) ResetFailCount(r *http.Request) {
	ip := getRequestIP(r)
	delete(auth.failedCount, ip)
}

func (auth *basicAuthHTTPHandlerDecorator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if auth.IsBlocked(r) {
		http.Error(w, fmt.Sprintf("%s: %s", http.StatusText(http.StatusUnauthorized),
			"too many retries"), http.StatusUnauthorized)
		return
	}

	username, password, ok := r.BasicAuth()
	if !ok {
		auth.Unauthorized(w, r, "authorization failed", false)
		return
	}

	pass, err := auth.auth.Authenticate(username, password)
	if !pass {
		auth.Unauthorized(w, r, err.Error(), true)
		return
	}
	auth.ResetFailCount(r)

	if auth.handler != nil {
		auth.handler.ServeHTTP(w, r)
	} else {
		auth.handlerFunc(w, r)
	}
}

// BasicAuth is a class that provide  WrapHandler and WrapHandlerFunc, which
// turns a http.Handler to a HTTP basic-auth enabled http handler.
type BasicAuth struct {
	Realm   string
	secrets map[string]string
	Disable bool // Disable basic auth function, pass through
}

// NewBasicAuth creates a BasicAuth object
func NewBasicAuth(realm, htpasswd string, Disable bool) *BasicAuth {
	secrets := make(map[string]string)

	f, err := os.Open(htpasswd)
	if err != nil {
		return &BasicAuth{realm, secrets, true}
	}

	b := bufio.NewReader(f)
	for {
		line, _, err := b.ReadLine()
		if err == io.EOF {
			break
		}
		if line[0] == '#' {
			continue
		}
		parts := strings.Split(string(line), ":")
		if len(parts) != 2 {
			continue
		}
		matched, err := regexp.Match("^\\$2[ay]\\$.*$", []byte(parts[1]))
		if err != nil {
			panic(err)
		}
		if !matched {
			log.Printf("BasicAuth: user %s: password encryption scheme not supported, ignored.\n", parts[0])
			continue
		}
		secrets[parts[0]] = parts[1]
	}

	return &BasicAuth{realm, secrets, Disable}
}

// WrapHandler wraps an http.Hanlder and provide HTTP basic-auth.
func (auth *BasicAuth) WrapHandler(h http.Handler) http.Handler {
	if auth.Disable {
		return h
	}
	return &basicAuthHTTPHandlerDecorator{auth, h, nil,
		make(map[string]time.Time), make(map[string]int)}
}

// WrapHandlerFunc wraps an http.HanlderFunc and provide HTTP basic-auth.
func (auth *BasicAuth) WrapHandlerFunc(h http.HandlerFunc) http.Handler {
	if auth.Disable {
		return h
	}
	return &basicAuthHTTPHandlerDecorator{auth, nil, h,
		make(map[string]time.Time), make(map[string]int)}
}

// Authenticate authenticate an user with the provided user and passwd.
func (auth *BasicAuth) Authenticate(user, passwd string) (bool, error) {
	deniedError := errors.New("permission denied")

	passwdHash, ok := auth.secrets[user]
	if !ok {
		return false, deniedError
	}

	if bcrypt.CompareHashAndPassword([]byte(passwdHash), []byte(passwd)) != nil {
		return false, deniedError
	}

	return true, nil
}
