// Copyright 2023 The Overlord Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

func setupTestDatabaseManager(t *testing.T) (*DatabaseManager, func()) {
	// Create a temporary test database file
	tmpFile, err := os.CreateTemp("", "overlord-dbtest-*.db")
	if err != nil {
		t.Fatalf("Failed to create temporary database file: %v", err)
	}
	tmpFileName := tmpFile.Name()
	tmpFile.Close()

	// Initialize the database manager
	dbManager := NewDatabaseManager(tmpFileName)
	err = dbManager.Connect()
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	// Cleanup function
	cleanup := func() {
		os.Remove(tmpFileName)
	}

	return dbManager, cleanup
}

// TestDatabaseInitialization tests database initialization
func TestDatabaseInitialization(t *testing.T) {
	dbManager, cleanup := setupTestDatabaseManager(t)
	defer cleanup()

	// Regenerate JWT Secret
	if err := dbManager.RegenerateJWTSecret(); err != nil {
		t.Fatalf("Failed to regenerate JWT secret: %v", err)
	}

	if dbManager.GetJWTSecret() == "" {
		t.Error("JWT secret should not be empty after regeneration")
	}

	// Test initialization with admin user
	if err := dbManager.Initialize("admin", "adminpass"); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Verify admin user was created
	user, err := dbManager.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("Failed to get admin user: %v", err)
	}
	if user.Username != "admin" {
		t.Errorf("Expected username 'admin', got '%s'", user.Username)
	}

	// Verify admin group was created
	group, err := dbManager.GetGroupByName(AdminGroupName)
	if err != nil {
		t.Fatalf("Failed to get admin group: %v", err)
	}
	if group.Name != AdminGroupName {
		t.Errorf("Expected group name '%s', got '%s'", AdminGroupName, group.Name)
	}

	// Verify admin user is part of admin group
	isAdmin, err := dbManager.IsUserAdmin("admin")
	if err != nil {
		t.Fatalf("Failed to check if user is admin: %v", err)
	}
	if !isAdmin {
		t.Error("Admin user should be part of admin group")
	}
}

// TestUserManagement tests user management functions
// Helper functions for TestUserManagement
func testUserCreation(t *testing.T, dbManager *DatabaseManager) {
	// Test creating a new user
	if err := dbManager.CreateUser("testuser", "testpass", false); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Test creating a duplicate user (should fail)
	if err := dbManager.CreateUser("testuser", "testpass", false); err == nil {
		t.Error("Creating duplicate user should fail")
	}
}

func testUserAuthentication(t *testing.T, dbManager *DatabaseManager) {
	// Test user authentication
	authenticated, err := dbManager.AuthenticateUser("testuser", "testpass")
	if err != nil {
		t.Fatalf("Failed to authenticate user: %v", err)
	}
	if !authenticated {
		t.Error("User should be authenticated with correct password")
	}

	// Test wrong password
	authenticated, err = dbManager.AuthenticateUser("testuser", "wrongpass")
	if err != nil {
		t.Fatalf("Failed to authenticate user: %v", err)
	}
	if authenticated {
		t.Error("User should not be authenticated with incorrect password")
	}
}

func testPasswordUpdate(t *testing.T, dbManager *DatabaseManager) {
	// Test updating password
	if err := dbManager.UpdateUserPassword("testuser", "newpass"); err != nil {
		t.Fatalf("Failed to update password: %v", err)
	}

	// Test authentication with new password
	authenticated, err := dbManager.AuthenticateUser("testuser", "newpass")
	if err != nil {
		t.Fatalf("Failed to authenticate user: %v", err)
	}
	if !authenticated {
		t.Error("User should be authenticated with new password")
	}
}

func testAdminUser(t *testing.T, dbManager *DatabaseManager) {
	// Test creating an admin user
	if err := dbManager.CreateUser("adminuser", "adminpass", true); err != nil {
		t.Fatalf("Failed to create admin user: %v", err)
	}

	// Test if user is admin
	isAdmin, err := dbManager.IsUserAdmin("adminuser")
	if err != nil {
		t.Fatalf("Failed to check if user is admin: %v", err)
	}
	if !isAdmin {
		t.Error("User should be admin")
	}
}

func testUserDeletion(t *testing.T, dbManager *DatabaseManager) {
	// Test deleting user
	if err := dbManager.DeleteUser("testuser"); err != nil {
		t.Fatalf("Failed to delete user: %v", err)
	}

	// Verify user was deleted
	_, err := dbManager.GetUserByUsername("testuser")
	if err == nil {
		t.Error("User should have been deleted")
	}
}

func TestUserManagement(t *testing.T) {
	dbManager, cleanup := setupTestDatabaseManager(t)
	defer cleanup()

	// Initialize database
	if err := dbManager.Initialize("admin", "adminpass"); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	testUserCreation(t, dbManager)
	testUserAuthentication(t, dbManager)
	testPasswordUpdate(t, dbManager)
	testAdminUser(t, dbManager)
	testUserDeletion(t, dbManager)
}

// Helper functions for TestGroupManagement
func testGroupCreation(t *testing.T, dbManager *DatabaseManager) {
	// Test creating a new group
	if err := dbManager.CreateGroup("testgroup"); err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	// Test creating a duplicate group (should fail)
	if err := dbManager.CreateGroup("testgroup"); err == nil {
		t.Error("Creating duplicate group should fail")
	}
}

func testUserGroupMembership(t *testing.T, dbManager *DatabaseManager) {
	// Test adding user to group
	if err := dbManager.AddUserToGroup("testuser", "testgroup"); err != nil {
		t.Fatalf("Failed to add user to group: %v", err)
	}

	// Test if user is in group
	isInGroup, err := dbManager.IsUserInGroup("testuser", "testgroup")
	if err != nil {
		t.Fatalf("Failed to check if user is in group: %v", err)
	}
	if !isInGroup {
		t.Error("User should be in group")
	}
}

func testGroupQueries(t *testing.T, dbManager *DatabaseManager) {
	// Test getting user groups
	groups, err := dbManager.GetUserGroups("testuser")
	if err != nil {
		t.Fatalf("Failed to get user groups: %v", err)
	}
	if len(groups) != 1 || groups[0] != "testgroup" {
		t.Errorf("Expected user to be in 1 group 'testgroup', got %v", groups)
	}

	// Test getting group users
	users, err := dbManager.GetGroupUsers("testgroup")
	if err != nil {
		t.Fatalf("Failed to get group users: %v", err)
	}
	if len(users) != 1 || users[0].Username != "testuser" {
		t.Errorf("Expected group to have 1 user 'testuser', got %d users", len(users))
	}
}

func testUserGroupRemoval(t *testing.T, dbManager *DatabaseManager) {
	// Test removing user from group
	if err := dbManager.RemoveUserFromGroup("testuser", "testgroup"); err != nil {
		t.Fatalf("Failed to remove user from group: %v", err)
	}

	// Verify user was removed from group
	isInGroup, err := dbManager.IsUserInGroup("testuser", "testgroup")
	if err != nil {
		t.Fatalf("Failed to check if user is in group: %v", err)
	}
	if isInGroup {
		t.Error("User should have been removed from group")
	}
}

func testGroupDeletion(t *testing.T, dbManager *DatabaseManager) {
	// Test deleting group
	if err := dbManager.DeleteGroup("testgroup"); err != nil {
		t.Fatalf("Failed to delete group: %v", err)
	}

	// Verify group was deleted
	_, err := dbManager.GetGroupByName("testgroup")
	if err == nil {
		t.Error("Group should have been deleted")
	}

	// Test preventing deletion of admin group
	if err := dbManager.DeleteGroup(AdminGroupName); err == nil {
		t.Error("Deleting admin group should fail")
	}
}

// TestGroupManagement tests group management functions
func TestGroupManagement(t *testing.T) {
	dbManager, cleanup := setupTestDatabaseManager(t)
	defer cleanup()

	// Initialize database
	if err := dbManager.Initialize("admin", "adminpass"); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create a test user
	if err := dbManager.CreateUser("testuser", "testpass", false); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	testGroupCreation(t, dbManager)
	testUserGroupMembership(t, dbManager)
	testGroupQueries(t, dbManager)
	testUserGroupRemoval(t, dbManager)
	testGroupDeletion(t, dbManager)
}

// Helper functions for TestAdminProtection
func testLastAdminProtection(t *testing.T, dbManager *DatabaseManager) {
	// Test removing the admin user from admin group (should fail)
	if err := dbManager.RemoveUserFromGroup("admin", AdminGroupName); err == nil {
		t.Error("Removing the last admin user from admin group should fail")
	}
}

func testMultipleAdminRemoval(t *testing.T, dbManager *DatabaseManager) {
	// Create a second admin user
	if err := dbManager.CreateUser("admin2", "adminpass", true); err != nil {
		t.Fatalf("Failed to create second admin user: %v", err)
	}

	// Now removing one admin user should work since there are two
	if err := dbManager.RemoveUserFromGroup("admin", AdminGroupName); err != nil {
		t.Fatalf("Failed to remove admin user from admin group: %v", err)
	}

	// Verify admin user was removed from admin group
	isAdmin, err := dbManager.IsUserAdmin("admin")
	if err != nil {
		t.Fatalf("Failed to check if user is admin: %v", err)
	}
	if isAdmin {
		t.Error("User should have been removed from admin group")
	}

	// Remove the second admin user (should fail)
	if err := dbManager.RemoveUserFromGroup("admin2", AdminGroupName); err == nil {
		t.Error("Removing the last admin user from admin group should fail")
	}
}

// TestAdminProtection tests protections around admin users and groups
func TestAdminProtection(t *testing.T) {
	dbManager, cleanup := setupTestDatabaseManager(t)
	defer cleanup()

	// Initialize database with admin user
	if err := dbManager.Initialize("admin", "adminpass"); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	testLastAdminProtection(t, dbManager)
	testMultipleAdminRemoval(t, dbManager)
}

// Helper functions for TestAllowlistChecking
func setupAllowlistTestData(t *testing.T, dbManager *DatabaseManager) {
	// Create test users
	if err := dbManager.CreateUser("user1", "pass", false); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	if err := dbManager.CreateUser("user2", "pass", false); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create test group
	if err := dbManager.CreateGroup("testgroup"); err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	// Add user1 to testgroup
	if err := dbManager.AddUserToGroup("user1", "testgroup"); err != nil {
		t.Fatalf("Failed to add user to group: %v", err)
	}
}

func testEmptyAndSpecialAllowlists(t *testing.T, dbManager *DatabaseManager) {
	// Test empty allowlist
	hasAccess, err := dbManager.CheckAllowlist("user1", []string{})
	if err != nil {
		t.Fatalf("Failed to check allowlist: %v", err)
	}
	if hasAccess {
		t.Error("User should not have access with empty allowlist")
	}

	// Test "anyone" special case
	hasAccess, err = dbManager.CheckAllowlist("user1", []string{"anyone"})
	if err != nil {
		t.Fatalf("Failed to check allowlist: %v", err)
	}
	if !hasAccess {
		t.Error("User should have access with 'anyone' in allowlist")
	}
}

func testDirectUserAndGroupAccess(t *testing.T, dbManager *DatabaseManager) {
	// Test direct user specification
	hasAccess, err := dbManager.CheckAllowlist("user1", []string{"u/user1"})
	if err != nil {
		t.Fatalf("Failed to check allowlist: %v", err)
	}
	if !hasAccess {
		t.Error("User should have access when directly specified in allowlist")
	}

	// Test group specification
	hasAccess, err = dbManager.CheckAllowlist("user1", []string{"g/testgroup"})
	if err != nil {
		t.Fatalf("Failed to check allowlist: %v", err)
	}
	if !hasAccess {
		t.Error("User should have access when their group is in allowlist")
	}
}

func testAllowlistDenialAndErrors(t *testing.T, dbManager *DatabaseManager) {
	// Test user not in allowlist
	hasAccess, err := dbManager.CheckAllowlist("user2",
		[]string{"u/user1", "g/testgroup"})
	if err != nil {
		t.Fatalf("Failed to check allowlist: %v", err)
	}
	if hasAccess {
		t.Error("User should not have access when neither they nor their groups are in allowlist")
	}

	// Test invalid entity format
	hasAccess, err = dbManager.CheckAllowlist("user1", []string{"invalid"})
	if err == nil {
		t.Error("Checking allowlist with invalid entity format should return error")
	}
	if hasAccess {
		t.Error("User should not have access with invalid entity format")
	}
}

// TestAllowlistChecking tests the allowlist checking functionality
func TestAllowlistChecking(t *testing.T) {
	dbManager, cleanup := setupTestDatabaseManager(t)
	defer cleanup()

	// Initialize database
	if err := dbManager.Initialize("admin", "adminpass"); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	setupAllowlistTestData(t, dbManager)
	testEmptyAndSpecialAllowlists(t, dbManager)
	testDirectUserAndGroupAccess(t, dbManager)
	testAllowlistDenialAndErrors(t, dbManager)
}

// TestParseEntityName tests the entity name parsing function
func TestParseEntityName(t *testing.T) {
	// Test valid user entity
	entityType, entityName, err := ParseEntityName("u/username")
	if err != nil {
		t.Fatalf("Failed to parse entity name: %v", err)
	}
	if entityType != "u" || entityName != "username" {
		t.Errorf("Expected entity type 'u' and name 'username', got '%s' and '%s'",
			entityType, entityName)
	}

	// Test valid group entity
	entityType, entityName, err = ParseEntityName("g/groupname")
	if err != nil {
		t.Fatalf("Failed to parse entity name: %v", err)
	}
	if entityType != "g" || entityName != "groupname" {
		t.Errorf("Expected entity type 'g' and name 'groupname', got '%s' and '%s'",
			entityType, entityName)
	}

	// Test invalid entity format
	_, _, err = ParseEntityName("invalid")
	if err == nil {
		t.Error("Parsing invalid entity format should return error")
	}

	// Test invalid entity type
	_, _, err = ParseEntityName("x/something")
	if err == nil {
		t.Error("Parsing invalid entity type should return error")
	}
}

// TestGetAllUsers tests the GetAllUsers function
func TestGetAllUsers(t *testing.T) {
	dbManager, cleanup := setupTestDatabaseManager(t)
	defer cleanup()

	// Initialize database
	if err := dbManager.Initialize("admin", "adminpass"); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create test users
	if err := dbManager.CreateUser("user1", "pass", false); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	if err := dbManager.CreateUser("user2", "pass", false); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Test GetAllUsers
	users, err := dbManager.GetAllUsers()
	if err != nil {
		t.Fatalf("Failed to get all users: %v", err)
	}
	if len(users) != 3 { // admin + user1 + user2
		t.Errorf("Expected 3 users, got %d", len(users))
	}

	// Verify usernames
	usernames := make(map[string]bool)
	for _, user := range users {
		usernames[user.Username] = true
	}
	if !usernames["admin"] || !usernames["user1"] || !usernames["user2"] {
		t.Errorf("Missing expected users in result: %v", usernames)
	}
}

// TestGetAllGroups tests the GetAllGroups function
func TestGetAllGroups(t *testing.T) {
	dbManager, cleanup := setupTestDatabaseManager(t)
	defer cleanup()

	// Initialize database
	if err := dbManager.Initialize("admin", "adminpass"); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create test groups
	if err := dbManager.CreateGroup("group1"); err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}
	if err := dbManager.CreateGroup("group2"); err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	// Test GetAllGroups
	groups, err := dbManager.GetAllGroups()
	if err != nil {
		t.Fatalf("Failed to get all groups: %v", err)
	}
	if len(groups) != 3 { // admin + group1 + group2
		t.Errorf("Expected 3 groups, got %d", len(groups))
	}

	// Verify group names
	groupNames := make(map[string]bool)
	for _, group := range groups {
		groupNames[group.Name] = true
	}
	if !groupNames[AdminGroupName] || !groupNames["group1"] || !groupNames["group2"] {
		t.Errorf("Missing expected groups in result: %v", groupNames)
	}
}

// TestGenerateRandomSecret tests the random secret generation function
func TestGenerateRandomSecret(t *testing.T) {
	// Test secret with length 32
	secret1, err := GenerateRandomSecret(32)
	if err != nil {
		t.Fatalf("Failed to generate random secret: %v", err)
	}
	if len(secret1) != 32 {
		t.Errorf("Expected secret of length 32, got %d", len(secret1))
	}

	// Test secret with length 64
	secret2, err := GenerateRandomSecret(64)
	if err != nil {
		t.Fatalf("Failed to generate random secret: %v", err)
	}
	if len(secret2) != 64 {
		t.Errorf("Expected secret of length 64, got %d", len(secret2))
	}

	// Test that two secrets are different
	if secret1 == secret2[:32] {
		t.Error("Two generated secrets should be different")
	}

	// Test that generated secrets only contain valid URL-safe base64 characters
	validChars := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	for _, char := range secret1 {
		if !strings.Contains(validChars, string(char)) {
			t.Errorf("Secret contains invalid character: %c", char)
		}
	}
}

// TestDatabaseEdgeCases tests edge cases and error handling in the database manager
func TestDatabaseEdgeCases(t *testing.T) {
	dbManager, cleanup := setupTestDatabaseManager(t)
	defer cleanup()

	// Initialize database
	if err := dbManager.Initialize("admin", "adminpass"); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Test user not found
	_, err := dbManager.GetUserByUsername("nonexistentuser")
	if err == nil {
		t.Error("GetUserByUsername should return error for non-existent user")
	}

	// Test group not found
	_, err = dbManager.GetGroupByName("nonexistentgroup")
	if err == nil {
		t.Error("GetGroupByName should return error for non-existent group")
	}

	// Test adding user to non-existent group
	err = dbManager.AddUserToGroup("admin", "nonexistentgroup")
	if err == nil {
		t.Error("AddUserToGroup should return error for non-existent group")
	}

	// Test adding non-existent user to group
	err = dbManager.AddUserToGroup("nonexistentuser", AdminGroupName)
	if err == nil {
		t.Error("AddUserToGroup should return error for non-existent user")
	}

	// Test removing user from non-existent group
	err = dbManager.RemoveUserFromGroup("admin", "nonexistentgroup")
	if err == nil {
		t.Error("RemoveUserFromGroup should return error for non-existent group")
	}

	// Test removing non-existent user from group
	err = dbManager.RemoveUserFromGroup("nonexistentuser", AdminGroupName)
	if err == nil {
		t.Error("RemoveUserFromGroup should return error for non-existent user")
	}

	// Test updating password for non-existent user
	err = dbManager.UpdateUserPassword("nonexistentuser", "newpass")
	if err == nil {
		t.Error("UpdateUserPassword should return error for non-existent user")
	}

	// Test delete non-existent user (should error)
	err = dbManager.DeleteUser("nonexistentuser")
	if err == nil {
		t.Error("DeleteUser should return error for non-existent user")
	}

	// Test delete non-existent group (GORM silently ignores this, so no error)
	err = dbManager.DeleteGroup("nonexistentgroup")
	if err != nil {
		t.Errorf("DeleteGroup should not error for non-existent group: %v", err)
	}
}

// TestConcurrentDatabaseAccess tests concurrent access to the database
// Helper functions for TestConcurrentDatabaseAccess
func performConcurrentUserOperations(t *testing.T, dbManager *DatabaseManager, id int, numOps int) {
	username := fmt.Sprintf("user%d", id)
	err := dbManager.CreateUser(username, "testpass", false)
	if err != nil {
		t.Errorf("Failed to create user %s: %v", username, err)
		return
	}

	// Perform multiple read operations
	for range numOps {
		_, err := dbManager.GetUserByUsername(username)
		if err != nil {
			t.Errorf("Failed to get user %s: %v", username, err)
		}

		_, err = dbManager.IsUserAdmin(username)
		if err != nil {
			t.Errorf("Failed to check if user %s is admin: %v", username, err)
		}
	}

	// Update user password
	err = dbManager.UpdateUserPassword(username, "newpass")
	if err != nil {
		t.Errorf("Failed to update password for user %s: %v", username, err)
	}
}

func performConcurrentGroupOperations(t *testing.T, dbManager *DatabaseManager, id int) {
	username := fmt.Sprintf("user%d", id)
	groupName := fmt.Sprintf("group%d", id)

	err := dbManager.CreateGroup(groupName)
	if err != nil {
		t.Errorf("Failed to create group %s: %v", groupName, err)
		return
	}

	// Add user to group
	err = dbManager.AddUserToGroup(username, groupName)
	if err != nil {
		t.Errorf("Failed to add user %s to group %s: %v", username, groupName, err)
	}

	// Check if user is in group
	isInGroup, err := dbManager.IsUserInGroup(username, groupName)
	if err != nil {
		t.Errorf("Failed to check if user %s is in group %s: %v",
			username, groupName, err)
	}
	if !isInGroup {
		t.Errorf("User %s should be in group %s", username, groupName)
	}

	// Remove user from group
	err = dbManager.RemoveUserFromGroup(username, groupName)
	if err != nil {
		t.Errorf("Failed to remove user %s from group %s: %v",
			username, groupName, err)
	}
}

func performConcurrentCleanup(t *testing.T, dbManager *DatabaseManager, id int) {
	username := fmt.Sprintf("user%d", id)
	groupName := fmt.Sprintf("group%d", id)

	// Delete group
	err := dbManager.DeleteGroup(groupName)
	if err != nil {
		t.Errorf("Failed to delete group %s: %v", groupName, err)
	}

	// Delete user
	err = dbManager.DeleteUser(username)
	if err != nil {
		t.Errorf("Failed to delete user %s: %v", username, err)
	}
}

func verifyConcurrentTestResults(t *testing.T, dbManager *DatabaseManager) {
	// Verify only the admin user remains
	users, err := dbManager.GetAllUsers()
	if err != nil {
		t.Fatalf("Failed to get all users: %v", err)
	}
	if len(users) != 1 || users[0].Username != "admin" {
		t.Errorf("Expected only admin user to remain, got %d users", len(users))
	}

	// Verify only the admin group remains
	groups, err := dbManager.GetAllGroups()
	if err != nil {
		t.Fatalf("Failed to get all groups: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != AdminGroupName {
		t.Errorf("Expected only admin group to remain, got %d groups", len(groups))
	}
}

func TestConcurrentDatabaseAccess(t *testing.T) {
	dbManager, cleanup := setupTestDatabaseManager(t)
	defer cleanup()

	// Initialize database
	if err := dbManager.Initialize("admin", "adminpass"); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Number of concurrent goroutines
	numGoroutines := 10
	// Number of operations per goroutine
	numOps := 5

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			performConcurrentUserOperations(t, dbManager, id, numOps)
			performConcurrentGroupOperations(t, dbManager, id)
			performConcurrentCleanup(t, dbManager, id)
		}(i)
	}

	wg.Wait()
	verifyConcurrentTestResults(t, dbManager)
}

// TestDatabaseConnectionHandling tests database connection handling
func TestDatabaseConnectionHandling(t *testing.T) {
	// Test with invalid database path
	invalidDBManager := NewDatabaseManager("/invalid/path/to/db.sqlite")
	err := invalidDBManager.Connect()
	if err == nil {
		t.Error("Connect should fail with invalid database path")
	}

	// Test with valid database
	dbManager, cleanup := setupTestDatabaseManager(t)
	defer cleanup()

	// Test double initialization (should expect error on second initialization)
	err = dbManager.Initialize("admin", "adminpass")
	if err != nil {
		t.Fatalf("First initialization failed: %v", err)
	}

	// Second initialization should return an error since the admin user already exists
	err = dbManager.Initialize("admin", "adminpass")
	if err == nil {
		t.Error("Second initialization should return an error since admin user already exists")
	} else if !strings.Contains(err.Error(), "user already exists") {
		t.Errorf("Expected 'user already exists' error, got: %v", err)
	}

	// Verify JWT secret is retrievable
	secret := dbManager.GetJWTSecret()
	if secret == "" {
		t.Error("JWT secret should be set after initialization")
	}
}

// Helper functions for TestDatabaseTransactionBehavior
func testBasicUserCreationAndAuth(t *testing.T, dbManager *DatabaseManager) {
	// Test creating a user with correct password
	err := dbManager.CreateUser("user1", "goodpassword", false)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Verify user was created
	user, err := dbManager.GetUserByUsername("user1")
	if err != nil {
		t.Fatalf("Failed to get user after creation: %v", err)
	}
	if user.Username != "user1" {
		t.Errorf("Expected username 'user1', got '%s'", user.Username)
	}

	// Verify authentication works
	authenticated, err := dbManager.AuthenticateUser("user1", "goodpassword")
	if err != nil {
		t.Fatalf("Failed to authenticate user: %v", err)
	}
	if !authenticated {
		t.Error("User should be authenticated with correct password")
	}
}

func testAdminUserCreation(t *testing.T, dbManager *DatabaseManager) {
	// Now create an admin user to verify they get added to admin group
	err := dbManager.CreateUser("adminuser", "adminpass", true)
	if err != nil {
		t.Fatalf("Failed to create admin user: %v", err)
	}

	// Verify user is in admin group
	isAdmin, err := dbManager.IsUserAdmin("adminuser")
	if err != nil {
		t.Fatalf("Failed to check if user is admin: %v", err)
	}
	if !isAdmin {
		t.Error("User should be admin")
	}
}

func testMultiGroupMembership(t *testing.T, dbManager *DatabaseManager) {
	// Test creating a user and adding to multiple groups
	err := dbManager.CreateUser("multigroup", "pass", false)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create test groups
	err = dbManager.CreateGroup("group1")
	if err != nil {
		t.Fatalf("Failed to create group1: %v", err)
	}
	err = dbManager.CreateGroup("group2")
	if err != nil {
		t.Fatalf("Failed to create group2: %v", err)
	}

	// Add user to both groups
	err = dbManager.AddUserToGroup("multigroup", "group1")
	if err != nil {
		t.Fatalf("Failed to add user to group1: %v", err)
	}
	err = dbManager.AddUserToGroup("multigroup", "group2")
	if err != nil {
		t.Fatalf("Failed to add user to group2: %v", err)
	}
}

func verifyMultiGroupMembership(t *testing.T, dbManager *DatabaseManager) {
	// Verify user is in both groups
	groups, err := dbManager.GetUserGroups("multigroup")
	if err != nil {
		t.Fatalf("Failed to get user groups: %v", err)
	}
	if len(groups) != 2 {
		t.Errorf("Expected user to be in 2 groups, got %d", len(groups))
	}
	hasGroup1 := false
	hasGroup2 := false
	for _, g := range groups {
		if g == "group1" {
			hasGroup1 = true
		}
		if g == "group2" {
			hasGroup2 = true
		}
	}
	if !hasGroup1 || !hasGroup2 {
		t.Errorf("User should be in groups 'group1' and 'group2', got %v", groups)
	}
}

// TestDatabaseTransactionBehavior tests database transaction behavior
func TestDatabaseTransactionBehavior(t *testing.T) {
	dbManager, cleanup := setupTestDatabaseManager(t)
	defer cleanup()

	// Initialize database
	if err := dbManager.Initialize("admin", "adminpass"); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	testBasicUserCreationAndAuth(t, dbManager)
	testAdminUserCreation(t, dbManager)
	testMultiGroupMembership(t, dbManager)
	verifyMultiGroupMembership(t, dbManager)
}
