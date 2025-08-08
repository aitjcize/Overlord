// Copyright 2023 The Overlord Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gorilla/mux"
)

const (
	adminUsername = "admin"
	adminPassword = "adminpass"
	testUsername  = "testuser"
	testPassword  = "testpass"
)

func setupTestDB(t *testing.T) (*DatabaseManager, func()) {
	// Create a temporary test database file
	tmpFile, err := os.CreateTemp("", "overlord-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temporary database file: %v", err)
	}
	tmpFileName := tmpFile.Name()
	tmpFile.Close()

	// Initialize the database
	dbManager := NewDatabaseManager(tmpFileName)

	// Initialize with admin user
	err = dbManager.Initialize(adminUsername, adminPassword)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create test user
	err = dbManager.CreateUser(testUsername, testPassword, false)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create test group
	err = dbManager.CreateGroup("testgroup")
	if err != nil {
		t.Fatalf("Failed to create test group: %v", err)
	}

	// Verify admin user is admin
	isAdmin, err := dbManager.IsUserAdmin(adminUsername)
	if err != nil {
		t.Fatalf("Failed to check if admin user is admin: %v", err)
	}
	if !isAdmin {
		t.Fatalf("Admin user is not marked as admin")
	}

	// Verify the number of users in the database
	users, err := dbManager.GetAllUsers()
	if err != nil {
		t.Fatalf("Failed to get all users: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("Expected 2 users, got %d", len(users))
	}

	// Cleanup function
	cleanup := func() {
		// We don't have a Close() method, so just delete the database file
		os.Remove(tmpFileName)
	}
	return dbManager, cleanup
}

func setupTestOverlord(t *testing.T) (*Overlord, func()) {
	dbManager, cleanup := setupTestDB(t)

	ovl := &Overlord{
		dbPath:    dbManager.dbPath,
		dbManager: dbManager,
	}

	return ovl, cleanup
}

func createAuthRequest(jwtToken, method, url string, body []byte) *http.Request {
	req := httptest.NewRequest(method, url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	return req
}

// Helper function to check if a user exists
func userExists(dbManager *DatabaseManager, username string) (bool, error) {
	user, err := dbManager.GetUserByUsername(username)
	if err != nil {
		if err.Error() == "record not found" {
			return false, nil
		}
		return false, err
	}
	return user != nil, nil
}

func loginAs(router *mux.Router, username string, pwd ...string) (string, error) {
	password := ""
	if len(pwd) == 0 {
		switch username {
		case adminUsername:
			password = adminPassword
		case testUsername:
			password = testPassword
		default:
			return "", fmt.Errorf("invalid username: %s", username)
		}
	} else {
		password = pwd[0]
	}

	loginReq := LoginRequest{
		Username: username,
		Password: password,
	}
	body, err := json.Marshal(loginReq)
	if err != nil {
		return "", err
	}

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	var response struct {
		Status string `json:"status"`
		Data   struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %v", err)
	}
	return response.Data.Token, nil
}

// TestListUsersHandler tests the listUsersHandler function
func TestListUsersHandler(t *testing.T) {
	ovl, cleanup := setupTestOverlord(t)
	defer cleanup()

	// Get the router with proper configuration
	router := ovl.registerRoutes()

	// Login to get a JWT token
	adminJWTToken, err := loginAs(router, adminUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Test with admin user
	req := createAuthRequest(adminJWTToken, "GET", "/api/users", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", rr.Code)
	}

	// Verify the response body contains the correct users
	var response struct {
		Status string `json:"status"`
		Data   []struct {
			Username string   `json:"username"`
			IsAdmin  bool     `json:"is_admin"`
			Groups   []string `json:"groups"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Check that we got 2 users (admin and testuser)
	if len(response.Data) != 2 {
		t.Errorf("Expected 2 users, got %d", len(response.Data))
	}

	// Check admin status
	found := false
	for _, user := range response.Data {
		if user.Username == adminUsername && user.IsAdmin {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Admin user not found or not marked as admin")
	}

	// Test with non-admin user (should be forbidden by middleware)
	userJWTToken, err := loginAs(router, testUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	req = createAuthRequest(userJWTToken, "GET", "/api/users", nil)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response - should be forbidden by middleware
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status code 403, got %d", rr.Code)
	}
}

// TestCreateUserHandler tests the createUserHandler function
func TestCreateUserHandler(t *testing.T) {
	ovl, cleanup := setupTestOverlord(t)
	defer cleanup()

	// Get the router with proper configuration
	router := ovl.registerRoutes()

	// Login to get a JWT token
	adminJWTToken, err := loginAs(router, adminUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Prepare request body
	newUser := struct {
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
	}{
		Username: "newuser",
		Password: "newpass",
		IsAdmin:  false,
	}

	body, err := json.Marshal(newUser)
	if err != nil {
		t.Fatalf("Failed to marshal new user: %v", err)
	}

	// Test with admin user
	req := createAuthRequest(adminJWTToken, "POST", "/api/users", body)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", rr.Code)
	}

	// Verify the user was created in the database
	exists, err := userExists(ovl.dbManager, "newuser")
	if err != nil {
		t.Fatalf("Error checking if user exists: %v", err)
	}
	if !exists {
		t.Errorf("User 'newuser' should exist in the database")
	}

	// Test with non-admin user (should be forbidden by middleware)
	userJWTToken, err := loginAs(router, testUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	newUser.Username = "anotheruser"
	body, _ = json.Marshal(newUser)
	req = createAuthRequest(userJWTToken, "POST", "/api/users", body)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response - should be forbidden by middleware
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status code 403, got %d", rr.Code)
	}

	// Verify the user was not created
	exists, _ = userExists(ovl.dbManager, "anotheruser")
	if exists {
		t.Errorf("User 'anotheruser' should not exist in the database")
	}
}

// TestDeleteUserHandler tests the deleteUserHandler function
func TestDeleteUserHandler(t *testing.T) {
	ovl, cleanup := setupTestOverlord(t)
	defer cleanup()

	// Get the router with proper configuration
	router := ovl.registerRoutes()

	// Login to get a JWT token
	adminJWTToken, err := loginAs(router, adminUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Create a new test user to be deleted.
	err = ovl.dbManager.CreateUser("usertobedeleted", "userpass", false)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Test with admin user
	req := createAuthRequest(adminJWTToken, "DELETE", "/api/users/usertobedeleted", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", rr.Code)
	}

	// Verify the user was deleted
	exists, _ := userExists(ovl.dbManager, "usertobedeleted")
	if exists {
		t.Errorf("User 'usertobedeleted' should have been deleted from the database")
	}

	// Test deleting the admin user (should be prevented)
	req = createAuthRequest(adminJWTToken, "DELETE", "/api/users/admin", nil)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response (should be bad request - cannot delete admin)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code 400, got %d", rr.Code)
	}

	// Verify admin still exists
	exists, _ = userExists(ovl.dbManager, "admin")
	if !exists {
		t.Errorf("User 'admin' should still exist in the database")
	}

	// Test with non-admin user (should be forbidden by middleware)
	userJWTToken, err := loginAs(router, testUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Create a new test user to be deleted.
	err = ovl.dbManager.CreateUser("usertobedeleted", "userpass", false)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	req = createAuthRequest(userJWTToken, "DELETE", "/api/users/usertobedeleted", nil)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response - should be forbidden by middleware
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status code 403, got %d", rr.Code)
	}
}

// TestUpdateUserPasswordHandler tests the updateUserPasswordHandler function
func TestUpdateUserPasswordHandler(t *testing.T) {
	ovl, cleanup := setupTestOverlord(t)
	defer cleanup()

	// Get the router with proper configuration
	router := ovl.registerRoutes()

	// Login to get a JWT token
	adminJWTToken, err := loginAs(router, adminUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Prepare request body
	passwordUpdate := struct {
		Password string `json:"password"`
	}{
		Password: "newpassword",
	}

	body, _ := json.Marshal(passwordUpdate)

	// Test admin updating another user's password
	req := createAuthRequest(adminJWTToken, "PUT", "/api/users/testuser/password", body)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", rr.Code)
	}

	// Verify the password was updated
	authenticated, _ := ovl.dbManager.AuthenticateUser("testuser", "newpassword")
	if !authenticated {
		t.Errorf("Password should have been updated")
	}

	// Test with non-admin user (should be forbidden by middleware)
	userJWTToken, err := loginAs(router, testUsername, "newpassword")
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	passwordUpdate.Password = "hackedpassword"
	body, _ = json.Marshal(passwordUpdate)

	req = createAuthRequest(userJWTToken, "PUT", "/api/users/testuser/password", body)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response - should be forbidden by middleware when using the admin only route
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status code 403, got %d", rr.Code)
	}
}

// TestCreateGroupHandler tests the createGroupHandler function
func TestCreateGroupHandler(t *testing.T) {
	ovl, cleanup := setupTestOverlord(t)
	defer cleanup()

	// Get the router with proper configuration
	router := ovl.registerRoutes()

	// Login to get a JWT token
	adminJWTToken, err := loginAs(router, adminUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Prepare request body
	newGroup := struct {
		Name string `json:"name"`
	}{
		Name: "newgroup",
	}

	body, _ := json.Marshal(newGroup)

	// Test with admin user
	req := createAuthRequest(adminJWTToken, "POST", "/api/groups", body)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", rr.Code)
	}

	// Verify the group was created
	groups, _ := ovl.dbManager.GetAllGroups()
	found := false
	for _, group := range groups {
		if group.Name == "newgroup" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Group 'newgroup' should exist in the database")
	}

	// Test with non-admin user (should be forbidden by middleware)
	userJWTToken, err := loginAs(router, testUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	newGroup.Name = "anothergroup"
	body, _ = json.Marshal(newGroup)
	req = createAuthRequest(userJWTToken, "POST", "/api/groups", body)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response - should be forbidden by middleware
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status code 403, got %d", rr.Code)
	}
}

// TestDeleteGroupHandler tests the deleteGroupHandler function
func TestDeleteGroupHandler(t *testing.T) {
	ovl, cleanup := setupTestOverlord(t)
	defer cleanup()

	// Get the router with proper configuration
	router := ovl.registerRoutes()

	// Login to get a JWT token
	adminJWTToken, err := loginAs(router, adminUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Test with admin user
	req := createAuthRequest(adminJWTToken, "DELETE", "/api/groups/testgroup", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", rr.Code)
	}

	// Verify the group was deleted
	groups, _ := ovl.dbManager.GetAllGroups()
	for _, group := range groups {
		if group.Name == "testgroup" {
			t.Errorf("Group 'testgroup' should have been deleted")
		}
	}

	// Test with non-admin user (should be forbidden by middleware)
	userJWTToken, err := loginAs(router, testUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	req = createAuthRequest(userJWTToken, "DELETE", "/api/groups/testgroup", nil)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response - should be forbidden by middleware
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status code 403, got %d", rr.Code)
	}
}

// TestAddUserToGroupHandler tests the addUserToGroupHandler function
func TestAddUserToGroupHandler(t *testing.T) {
	ovl, cleanup := setupTestOverlord(t)
	defer cleanup()

	// Get the router with proper configuration
	router := ovl.registerRoutes()

	// Login to get a JWT token
	adminJWTToken, err := loginAs(router, adminUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Prepare request body
	addRequest := struct {
		Username string `json:"username"`
	}{
		Username: "testuser",
	}

	body, _ := json.Marshal(addRequest)

	// Test with admin user
	req := createAuthRequest(adminJWTToken, "POST", "/api/groups/testgroup/users", body)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", rr.Code)
	}

	// Verify user was added to the group
	users, _ := ovl.dbManager.GetGroupUsers("testgroup")
	found := false
	for _, user := range users {
		if user.Username == "testuser" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("User 'testuser' should be in group 'testgroup'")
	}

	// Test with non-admin user (should be forbidden by middleware)
	userJWTToken, err := loginAs(router, testUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	req = createAuthRequest(userJWTToken, "POST", "/api/groups/testgroup/users", body)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response - should be forbidden by middleware
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status code 403, got %d", rr.Code)
	}
}

// TestRemoveUserFromGroupHandler tests the removeUserFromGroupHandler function
func TestRemoveUserFromGroupHandler(t *testing.T) {
	ovl, cleanup := setupTestOverlord(t)
	defer cleanup()

	// First add a user to the group
	err := ovl.dbManager.AddUserToGroup("testuser", "testgroup")
	if err != nil {
		t.Fatalf("Failed to add user to group: %v", err)
	}

	// Get the router with proper configuration
	router := ovl.registerRoutes()

	// Login to get a JWT token
	adminJWTToken, err := loginAs(router, adminUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Test with admin user
	req := createAuthRequest(adminJWTToken, "DELETE", "/api/groups/testgroup/users/testuser", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", rr.Code)
	}

	// Verify user was removed from the group
	users, _ := ovl.dbManager.GetGroupUsers("testgroup")
	for _, user := range users {
		if user.Username == "testuser" {
			t.Errorf("User 'testuser' should have been removed from group 'testgroup'")
		}
	}

	// Test with non-admin user (should be forbidden by middleware)
	// First add the user back to the group
	userJWTToken, err := loginAs(router, testUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	if err := ovl.dbManager.AddUserToGroup("testuser", "testgroup"); err != nil {
		t.Fatalf("Failed to add user to group: %v", err)
	}

	req = createAuthRequest(userJWTToken, "DELETE", "/api/groups/testgroup/users/testuser", nil)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response - should be forbidden by middleware
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status code 403, got %d", rr.Code)
	}
}

// TestListGroupUsersHandler tests the listGroupUsersHandler function
func TestListGroupUsersHandler(t *testing.T) {
	ovl, cleanup := setupTestOverlord(t)
	defer cleanup()

	// Add a user to the group first
	err := ovl.dbManager.AddUserToGroup("testuser", "testgroup")
	if err != nil {
		t.Fatalf("Failed to add user to group: %v", err)
	}

	// Get the router with proper configuration
	router := ovl.registerRoutes()

	// Login to get a JWT token
	adminJWTToken, err := loginAs(router, adminUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Test with admin user
	req := createAuthRequest(adminJWTToken, "GET", "/api/groups/testgroup/users", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", rr.Code)
	}

	// Verify the response contains the correct users
	var response struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(response.Data) != 1 {
		t.Errorf("Expected 1 user, got %d", len(response.Data))
	}

	if len(response.Data) > 0 && response.Data[0] != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", response.Data[0])
	}

	// Test with non-admin user (should be forbidden by middleware)
	userJWTToken, err := loginAs(router, testUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	req = createAuthRequest(userJWTToken, "GET", "/api/groups/testgroup/users", nil)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response - should be forbidden by middleware
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status code 403, got %d", rr.Code)
	}
}

// TestUpdateOwnPasswordHandler tests the updateOwnPasswordHandler function
func TestUpdateOwnPasswordHandler(t *testing.T) {
	ovl, cleanup := setupTestOverlord(t)
	defer cleanup()

	// Get the router with proper configuration
	router := ovl.registerRoutes()

	// Login to get a JWT token
	userJWTToken, err := loginAs(router, testUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Prepare request body
	passwordUpdate := struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}{
		CurrentPassword: testPassword,
		NewPassword:     "newtestpass",
	}

	body, _ := json.Marshal(passwordUpdate)

	// Test user updating their own password
	req := createAuthRequest(userJWTToken, "PUT", "/api/users/self/password", body)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", rr.Code)
	}

	// Verify the password was updated
	authenticated, _ := ovl.dbManager.AuthenticateUser(testUsername, "newtestpass")
	if !authenticated {
		t.Errorf("Password should have been updated to 'newtestpass'")
	}

	// Test with incorrect current password
	passwordUpdate.CurrentPassword = "wrongpass"
	passwordUpdate.NewPassword = "shouldnotchange"
	body, _ = json.Marshal(passwordUpdate)

	req = createAuthRequest(userJWTToken, "PUT", "/api/users/self/password", body)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check unauthorized response
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code 401, got %d", rr.Code)
	}

	// Verify the password was not updated
	authenticated, _ = ovl.dbManager.AuthenticateUser(testUsername, "shouldnotchange")
	if authenticated {
		t.Errorf("Password should not have been updated")
	}
}

// TestAdminRequiredMiddleware tests the adminRequired middleware through the configured router
func TestAdminRequiredMiddleware(t *testing.T) {
	ovl, cleanup := setupTestOverlord(t)
	defer cleanup()

	// Get the router with proper configuration
	router := ovl.registerRoutes()

	// Login to get a JWT token
	adminJWTToken, err := loginAs(router, adminUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Test with admin user - should be able to access admin resources
	req := createAuthRequest(adminJWTToken, "GET", "/api/users", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response - should get 200 OK for admin
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", rr.Code)
	}

	// Test with non-admin user - should be forbidden
	userJWTToken, err := loginAs(router, testUsername)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	req = createAuthRequest(userJWTToken, "GET", "/api/users", nil)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response - should get 403 Forbidden
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status code 403, got %d", rr.Code)
	}

	// Test with no user in context - should be forbidden
	req = httptest.NewRequest("GET", "/api/users", nil)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// Check response - should get 403 Forbidden
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code 401, got %d", rr.Code)
	}
}
