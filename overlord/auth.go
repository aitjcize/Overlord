// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	maxFailCount  = 10
	blockDuration = 30 * time.Minute
)

func getRequestIP(r *http.Request) string {
	if ips, ok := r.Header["X-Forwarded-For"]; ok {
		return ips[len(ips)-1]
	}
	idx := strings.LastIndex(r.RemoteAddr, ":")
	return r.RemoteAddr[:idx]
}

type basicAuthHTTPHandlerDecorator struct {
	auth        *BasicAuth
	handler     http.Handler
	handlerFunc http.HandlerFunc
}

// ServeHTTP implements the http.Handler interface.
func (d *basicAuthHTTPHandlerDecorator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if d.auth.IsBlocked(r) {
		http.Error(w, fmt.Sprintf("%s: %s", http.StatusText(http.StatusUnauthorized),
			"too many retries"), http.StatusUnauthorized)
		return
	}

	username, password, ok := r.BasicAuth()
	if !ok {
		d.auth.Unauthorized(w, r, "authorization failed", false)
		return
	}

	pass, err := d.auth.Authenticate(username, password)
	if !pass {
		d.auth.Unauthorized(w, r, err.Error(), true)
		return
	}
	d.auth.ResetFailCount(r)

	if d.handler != nil {
		d.handler.ServeHTTP(w, r)
	} else {
		d.handlerFunc(w, r)
	}
}

// BasicAuth is a class that provide  WrapHandler and WrapHandlerFunc, which
// turns a http.Handler to a HTTP basic-auth enabled http handler.
type BasicAuth struct {
	Realm   string
	secrets map[string]string
	Disable bool // Disable basic auth function, pass through

	blockedIps  map[string]time.Time
	failedCount map[string]int
	mutex       sync.RWMutex
}

// NewBasicAuth creates a BasicAuth object
func NewBasicAuth(realm, htpasswd string, disable bool) *BasicAuth {
	secrets := make(map[string]string)

	auth := &BasicAuth{
		Realm:       realm,
		secrets:     secrets,
		Disable:     disable,
		blockedIps:  make(map[string]time.Time),
		failedCount: make(map[string]int),
	}

	f, err := os.Open(htpasswd)
	if err != nil {
		log.Printf("Warning: %s", err.Error())
		auth.Disable = true
		return auth
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

	return auth
}

// WrapHandler wraps an http.Hanlder and provide HTTP basic-auth.
func (auth *BasicAuth) WrapHandler(h http.Handler) http.Handler {
	if auth.Disable {
		return h
	}
	return &basicAuthHTTPHandlerDecorator{auth, h, nil}
}

// WrapHandlerFunc wraps an http.HanlderFunc and provide HTTP basic-auth.
func (auth *BasicAuth) WrapHandlerFunc(h http.HandlerFunc) http.Handler {
	if auth.Disable {
		return h
	}
	return &basicAuthHTTPHandlerDecorator{auth, nil, h}
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

// IsBlocked returns true if the given IP is blocked.
func (auth *BasicAuth) IsBlocked(r *http.Request) bool {
	ip := getRequestIP(r)

	auth.mutex.RLock()
	t, ok := auth.blockedIps[ip]
	auth.mutex.RUnlock()
	if !ok {
		return false
	}

	if time.Now().Sub(t) < blockDuration {
		log.Printf("BasicAuth: IP %s attempted to login, blocked\n", ip)
		return true
	}

	// Unblock the user because of timeout
	auth.mutex.Lock()
	defer auth.mutex.Unlock()

	delete(auth.failedCount, ip)
	delete(auth.blockedIps, ip)

	return false
}

// ResetFailCount resets the fail count for the given IP.
func (auth *BasicAuth) ResetFailCount(r *http.Request) {
	auth.mutex.Lock()
	defer auth.mutex.Unlock()

	ip := getRequestIP(r)
	delete(auth.failedCount, ip)
}

// Unauthorized returns a 401 Unauthorized response.
func (auth *BasicAuth) Unauthorized(w http.ResponseWriter, r *http.Request,
	msg string, record bool) {

	auth.mutex.Lock()
	defer auth.mutex.Unlock()

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

	w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=%s", auth.Realm))
	http.Error(w, fmt.Sprintf("%s: %s", http.StatusText(http.StatusUnauthorized),
		msg), http.StatusUnauthorized)
}
