// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"bufio"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	apacheMd5Magic = "$apr1$"
	maxFailCount   = 5
	blockDuration  = 24 * time.Hour
)

func getRequestIP(r *http.Request) string {
	parts := strings.Split(r.RemoteAddr, ":")
	return parts[0]
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
		auth.failedCount[ip]++

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

	authString := r.Header.Get("Authorization")
	if authString == "" {
		auth.Unauthorized(w, r, "no authorization request", false)
		return
	}

	credential, err := base64.StdEncoding.DecodeString(authString[len("Basic "):])
	if err != nil {
		auth.Unauthorized(w, r, "invaid base64 encoding", true)
		return
	}

	parts := strings.Split(string(credential), ":")
	pass, err := auth.auth.Authenticate(parts[0], parts[1])
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
	passwdHash, ok := auth.secrets[user]
	if !ok {
		return false, errors.New("no such user")
	}

	// We only support Apache MD5 crypt since it's more secure.
	if passwdHash[:len(apacheMd5Magic)] != apacheMd5Magic {
		return false, errors.New("password encryption scheme not supported")
	}

	saltHash := passwdHash[len(apacheMd5Magic):]
	parts := strings.Split(saltHash, "$")
	if apacheMD5Crypt(passwd, parts[0]) != parts[1] {
		return false, errors.New("invalid password")
	}

	return true, nil
}

// Algorithm taken from: http://code.activestate.com/recipes/325204/
func apacheMD5Crypt(passwd, salt string) string {
	const (
		itoa64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	)

	m := md5.New()
	m.Write([]byte(passwd + apacheMd5Magic + salt))

	m2 := md5.New()
	m2.Write([]byte(passwd + salt + passwd))
	mixin := m2.Sum(nil)

	for i := range passwd {
		m.Write([]byte{mixin[i%16]})
	}

	l := len(passwd)

	for l != 0 {
		if l&1 != 0 {
			m.Write([]byte("\x00"))
		} else {
			m.Write([]byte{passwd[0]})
		}
		l >>= 1
	}

	final := m.Sum(nil)

	for i := 0; i < 1000; i++ {
		m3 := md5.New()
		if i&1 != 0 {
			m3.Write([]byte(passwd))
		} else {
			m3.Write([]byte(final))
		}

		if i%3 != 0 {
			m3.Write([]byte(salt))
		}

		if i%7 != 0 {
			m3.Write([]byte(passwd))
		}

		if i&1 != 0 {
			m3.Write([]byte(final))
		} else {
			m3.Write([]byte(passwd))
		}

		final = m3.Sum(nil)
	}

	var rearranged string
	seq := [][3]int{{0, 6, 12}, {1, 7, 13}, {2, 8, 14}, {3, 9, 15}, {4, 10, 5}}

	for _, p := range seq {
		a, b, c := p[0], p[1], p[2]

		v := int(final[a])<<16 | int(final[b])<<8 | int(final[c])
		for i := 0; i < 4; i++ {
			rearranged += string(itoa64[v&0x3f])
			v >>= 6
		}
	}

	v := int(final[11])
	for i := 0; i < 2; i++ {
		rearranged += string(itoa64[v&0x3f])
		v >>= 6
	}

	return rearranged
}
