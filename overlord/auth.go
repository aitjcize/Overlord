// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)

const (
	maxFailCount  = 10
	blockDuration = 30 * time.Minute
	// JWT token expiration time
	tokenExpirationTime = 24 * time.Hour
	// JWT issuer claim
	jwtIssuer = "overlord"
)

// JWTConfig holds the JWT configuration
type JWTConfig struct {
	// SecretPath is the path to the file containing the JWT secret
	SecretPath string
	// Path to the htpasswd file for username/password validation
	HtpasswdPath string
	// Secret is the loaded JWT secret
	secret string
}

// JWTClaims represents the claims in the JWT token
type JWTClaims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// JWTAuth handles JWT authentication
type JWTAuth struct {
	config      *JWTConfig
	secrets     map[string]string
	mutex       sync.RWMutex
	blockedIps  map[string]time.Time
	failedCount map[string]int
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token  string `json:"token"`
	Expire int64  `json:"expire"`
}

// Key type for context values
type contextKey string

// Define a key for JWT claims in context
const jwtClaimsContextKey contextKey = "jwtClaims"

// LoadJWTSecret loads the JWT secret from the specified file
func (config *JWTConfig) LoadJWTSecret() error {
	if config.SecretPath == "" {
		return errors.New("JWT secret file path not provided")
	}

	data, err := ioutil.ReadFile(config.SecretPath)
	if err != nil {
		return fmt.Errorf("failed to read JWT secret file: %v", err)
	}

	// Trim any whitespace or newlines
	config.secret = strings.TrimSpace(string(data))
	if config.secret == "" {
		return errors.New("JWT secret file is empty")
	}

	return nil
}

// GetSecret returns the loaded JWT secret
func (config *JWTConfig) GetSecret() string {
	return config.secret
}

// NewJWTAuth creates a new JWTAuth instance
func NewJWTAuth(config *JWTConfig) (*JWTAuth, error) {
	secrets := make(map[string]string)

	auth := &JWTAuth{
		config:      config,
		secrets:     secrets,
		blockedIps:  make(map[string]time.Time),
		failedCount: make(map[string]int),
	}

	log.Printf("JWTAuth: initialized with htpasswd path: %s",
		config.HtpasswdPath)

	// Load JWT secret from file
	if err := config.LoadJWTSecret(); err != nil {
		log.Printf("JWTAuth Error: %s", err.Error())
		return nil, fmt.Errorf("failed to load JWT secret: %v", err)
	}
	log.Printf("JWTAuth: successfully loaded JWT secret from %s",
		config.SecretPath)

	// Load users from htpasswd file
	if err := auth.loadHtpasswd(config.HtpasswdPath); err != nil {
		log.Printf("JWTAuth Error: %s", err.Error())
		return nil, fmt.Errorf("failed to load htpasswd file: %v", err)
	}

	return auth, nil
}

// loadHtpasswd loads user credentials from the htpasswd file
func (auth *JWTAuth) loadHtpasswd(htpasswdPath string) error {
	if htpasswdPath == "" {
		return errors.New("htpasswd file path not provided")
	}

	f, err := os.Open(htpasswdPath)
	if err != nil {
		return err
	}
	defer f.Close()

	b := bufio.NewReader(f)
	userCount := 0
	for {
		line, _, err := b.ReadLine()
		if err == io.EOF {
			break
		}
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		parts := strings.Split(string(line), ":")
		if len(parts) != 2 {
			log.Printf("JWTAuth: invalid line format in htpasswd file: %s",
				string(line))
			continue
		}

		matched, err := regexp.Match("^\\$2[ay]\\$.*$", []byte(parts[1]))
		if err != nil {
			log.Printf("JWTAuth: regex error: %v", err)
			panic(err)
		}
		if !matched {
			log.Printf("JWTAuth: user %s: password encryption scheme not supported (hash: %s), ignored.",
				parts[0], parts[1])
			continue
		}

		auth.secrets[parts[0]] = parts[1]
		userCount++
	}

	log.Printf("JWTAuth: loaded %d users from htpasswd file", userCount)

	if userCount == 0 {
		return fmt.Errorf("no valid users found in htpasswd file")
	}

	return nil
}

// Authenticate authenticates a user with the provided username and password
func (auth *JWTAuth) Authenticate(user, passwd string) (bool, error) {
	deniedError := errors.New("permission denied")

	passwdHash, ok := auth.secrets[user]
	if !ok {
		log.Printf("JWTAuth: user %s not found in secrets", user)
		return false, deniedError
	}

	err := bcrypt.CompareHashAndPassword([]byte(passwdHash), []byte(passwd))
	if err != nil {
		log.Printf("JWTAuth: password comparison failed for user %s: %v", user, err)
		return false, deniedError
	}
	return true, nil
}

// Login handles login requests and returns a JWT token
func (auth *JWTAuth) Login(w http.ResponseWriter, r *http.Request) {
	// Check if IP is blocked
	if auth.IsBlocked(r) {
		log.Printf("JWTAuth: login attempt from blocked IP: %s", getRequestIP(r))
		http.Error(w, fmt.Sprintf("%s: %s", http.StatusText(http.StatusUnauthorized),
			"too many retries"), http.StatusUnauthorized)
		return
	}

	// Parse the login request
	var loginReq LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
		log.Printf("JWTAuth: failed to parse login request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("JWTAuth: login attempt for user: %s", loginReq.Username)

	// Authenticate the user
	pass, err := auth.Authenticate(loginReq.Username, loginReq.Password)
	if !pass {
		log.Printf("JWTAuth: authentication failed for user %s: %v",
			loginReq.Username, err)
		auth.Unauthorized(w, r, "Invalid username or password", true)
		return
	}

	// Reset fail count
	auth.ResetFailCount(r)

	// Generate JWT token
	expirationTime := time.Now().Add(tokenExpirationTime)
	claims := &JWTClaims{
		Username: loginReq.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    jwtIssuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(auth.config.GetSecret()))
	if err != nil {
		log.Printf("JWTAuth: error generating token: %v", err)
		http.Error(w, "Error generating token", http.StatusInternalServerError)
		return
	}

	log.Printf("JWTAuth: token generated successfully for user: %s",
		loginReq.Username)

	// Return the token
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{
		Token:  tokenString,
		Expire: expirationTime.Unix(),
	})
}

// VerifyToken verifies a JWT token and returns the claims
func (auth *JWTAuth) VerifyToken(tokenString string) (*JWTClaims, error) {
	claims := &JWTClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			log.Printf("JWTAuth: unexpected signing method: %v", token.Header["alg"])
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(auth.config.GetSecret()), nil
	})

	if err != nil {
		log.Printf("JWTAuth: token parsing error: %v", err)
		return nil, err
	}

	if !token.Valid {
		log.Printf("JWTAuth: invalid token")
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// Middleware creates a JWT middleware for HTTP handlers
func (auth *JWTAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get token from Authorization header or query parameter
		var tokenString string

		// First try to get token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			// Check if it's a Bearer token
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
			} else {
				log.Printf("JWTAuth: invalid authorization header format: %s", authHeader)
			}
		}

		// If no token in header, try query parameter
		if tokenString == "" {
			tokenString = r.URL.Query().Get("token")
		}

		// If still no token, return unauthorized
		if tokenString == "" {
			log.Printf("JWTAuth: no token found in header or query parameter")
			auth.Unauthorized(w, r, "JWT token required", false)
			return
		}

		// Verify token
		claims, err := auth.VerifyToken(tokenString)
		if err != nil {
			log.Printf("JWTAuth: token verification failed: %v", err)
			auth.Unauthorized(w, r, "invalid token", false)
			return
		}

		// Add claims to request context
		r = r.WithContext(WithJWTClaims(r.Context(), claims))

		// Continue to the next handler
		next.ServeHTTP(w, r)
	})
}

// WithJWTClaims adds JWT claims to the context
func WithJWTClaims(ctx context.Context, claims *JWTClaims) context.Context {
	return context.WithValue(ctx, jwtClaimsContextKey, claims)
}

// GetJWTClaimsFromContext retrieves JWT claims from context
func GetJWTClaimsFromContext(ctx context.Context) *JWTClaims {
	claims, _ := ctx.Value(jwtClaimsContextKey).(*JWTClaims)
	return claims
}

// IsBlocked returns true if the given IP is blocked.
func (auth *JWTAuth) IsBlocked(r *http.Request) bool {
	ip := getRequestIP(r)

	auth.mutex.RLock()
	t, ok := auth.blockedIps[ip]
	auth.mutex.RUnlock()
	if !ok {
		return false
	}

	if time.Now().Sub(t) < blockDuration {
		log.Printf("JWTAuth: IP %s attempted to login, blocked\n", ip)
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
func (auth *JWTAuth) ResetFailCount(r *http.Request) {
	auth.mutex.Lock()
	defer auth.mutex.Unlock()

	ip := getRequestIP(r)
	delete(auth.failedCount, ip)
}

// Unauthorized returns a 401 Unauthorized response.
func (auth *JWTAuth) Unauthorized(w http.ResponseWriter, r *http.Request,
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

		log.Printf("JWTAuth: IP %s failed to login, count: %d\n", ip,
			auth.failedCount[ip])

		if auth.failedCount[ip] >= maxFailCount {
			auth.blockedIps[ip] = time.Now()
			log.Printf("JWTAuth: IP %s (%s) is blocked\n", ip, r.UserAgent())
		}
	}

	// Return 401 Unauthorized response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{
		"error": fmt.Sprintf("%s: %s", http.StatusText(http.StatusUnauthorized), msg),
	})
}

func getRequestIP(r *http.Request) string {
	if ips, ok := r.Header["X-Forwarded-For"]; ok {
		return ips[len(ips)-1]
	}
	idx := strings.LastIndex(r.RemoteAddr, ":")
	return r.RemoteAddr[:idx]
}
