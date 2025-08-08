// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

const (
	maxFailCount        = 10
	blockDuration       = 30 * time.Minute
	tokenExpirationTime = 7 * 24 * time.Hour
	jwtIssuer           = "overlord"
)

// JWTConfig holds the JWT configuration
type JWTConfig struct {
	DBPath string
}

// JWTClaims represents the claims in the JWT token
type JWTClaims struct {
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

// JWTAuth handles JWT authentication
type JWTAuth struct {
	config      *JWTConfig
	dbManager   *DatabaseManager
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

// Context keys
type contextKey string

const (
	userContextKey        contextKey = "user"
	adminStatusContextKey contextKey = "isAdmin"
	jwtClaimsContextKey   contextKey = "jwtClaims"
)

// NewJWTAuth creates a new JWTAuth instance
func NewJWTAuth(config *JWTConfig) (*JWTAuth, error) {
	dbManager := NewDatabaseManager(config.DBPath)

	auth := &JWTAuth{
		config:      config,
		dbManager:   dbManager,
		blockedIps:  make(map[string]time.Time),
		failedCount: make(map[string]int),
	}

	// Initialize database
	if err := dbManager.Connect(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %v", err)
	}

	log.Printf("JWTAuth: initialized with database path: %s", config.DBPath)
	return auth, nil
}

// Authenticate authenticates a user with the provided username and password
func (auth *JWTAuth) Authenticate(user, passwd string) bool {
	// Use database manager to authenticate
	authenticated, err := auth.dbManager.AuthenticateUser(user, passwd)
	if err != nil {
		log.Printf("JWTAuth: authentication error for user %s: %v", user, err)
		return false
	}
	return authenticated
}

// Login handles login requests and returns a JWT token
func (auth *JWTAuth) Login(w http.ResponseWriter, r *http.Request) {
	if auth.IsBlocked(r) {
		auth.Unauthorized(w, r, "Too many failed attempts", true)
		return
	}

	var loginReq LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
		auth.Unauthorized(w, r, "Invalid request", true)
		return
	}

	if !auth.Authenticate(loginReq.Username, loginReq.Password) {
		auth.Unauthorized(w, r, "Authentication error", true)
		return
	}

	auth.ResetFailCount(r)

	isAdmin, err := auth.dbManager.IsUserAdmin(loginReq.Username)
	if err != nil {
		log.Printf("JWTAuth: error checking if user %s is admin: %v", loginReq.Username, err)
		auth.Unauthorized(w, r, "Error checking admin status", true)
		return
	}

	expirationTime := time.Now().Add(tokenExpirationTime)
	claims := &JWTClaims{
		Username: loginReq.Username,
		IsAdmin:  isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    jwtIssuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(auth.dbManager.GetJWTSecret()))
	if err != nil {
		log.Printf("JWTAuth: error generating token: %v", err)
		ResponseError(w, "Error generating token", http.StatusInternalServerError)
		return
	}

	log.Printf("JWTAuth: token generated successfully for user: %s",
		loginReq.Username)

	// Return the token
	ResponseSuccess(w, LoginResponse{
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
		return []byte(auth.dbManager.GetJWTSecret()), nil
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
		var tokenString string

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

		if tokenString == "" {
			tokenString = r.URL.Query().Get("token")
		}

		if tokenString == "" {
			log.Printf("JWTAuth: no token found in header or query parameter")
			auth.Unauthorized(w, r, "JWT token required", false)
			return
		}

		claims, err := auth.VerifyToken(tokenString)
		if err != nil {
			log.Printf("JWTAuth: token verification failed: %v", err)
			auth.Unauthorized(w, r, "invalid token", false)
			return
		}

		// If a user is admin, we need to double check if the user is still an admin
		if claims.IsAdmin {
			isAdmin, err := auth.dbManager.IsUserAdmin(claims.Username)
			if err != nil {
				log.Printf("JWTAuth: error checking if user %s is admin: %v",
					claims.Username, err)
				auth.Unauthorized(w, r, "Error checking admin status", false)
				return
			}
			if !isAdmin {
				auth.Unauthorized(w, r, "User is not an admin", false)
				return
			}
		}
		r = r.WithContext(WithAuthClaims(r.Context(), claims))
		next.ServeHTTP(w, r)
	})
}

// WithAuthClaims adds JWT claims to the context
func WithAuthClaims(ctx context.Context, claims *JWTClaims) context.Context {
	ctx = context.WithValue(ctx, userContextKey, claims.Username)
	ctx = context.WithValue(ctx, adminStatusContextKey, claims.IsAdmin)
	return ctx
}

// GetUserFromContext gets the username from the request context
// and returns the username and a boolean indicating if it was found.
func GetUserFromContext(ctx context.Context) (string, bool) {
	username, ok := ctx.Value(userContextKey).(string)
	return username, ok
}

// GetAdminStatusFromContext gets the admin status from the request context
func GetAdminStatusFromContext(ctx context.Context) (bool, bool) {
	isAdmin, ok := ctx.Value(adminStatusContextKey).(bool)
	return isAdmin, ok
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

	if time.Since(t) < blockDuration {
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
	ResponseError(w, msg, http.StatusUnauthorized)
}

func getRequestIP(r *http.Request) string {
	if ips, ok := r.Header["X-Forwarded-For"]; ok {
		return ips[len(ips)-1]
	}
	idx := strings.LastIndex(r.RemoteAddr, ":")
	return r.RemoteAddr[:idx]
}

// JWTAuthConfig contains configuration for JWT authentication
type JWTAuthConfig struct {
	ExpiresIn int // The expiration time of the token in seconds
}
