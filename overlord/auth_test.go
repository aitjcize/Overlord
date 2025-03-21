// Copyright 2023 The Overlord Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"context"
	"testing"
)

// Test getting a user from the context
func TestGetUserFromContext(t *testing.T) {
	// Test case: user in context
	ctx := context.WithValue(context.Background(), userContextKey, "testuser")
	username, ok := GetUserFromContext(ctx)
	if !ok {
		t.Error("Expected GetUserFromContext to return true, got false")
	}
	if username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", username)
	}

	// Test case: no user in context
	ctx = context.Background()
	username, ok = GetUserFromContext(ctx)
	if ok {
		t.Error("Expected GetUserFromContext to return false, got true")
	}
	if username != "" {
		t.Errorf("Expected empty username, got '%s'", username)
	}
}

// Test getting admin status from the context
func TestGetAdminStatusFromContext(t *testing.T) {
	// Test case: admin user
	ctx := context.WithValue(context.Background(), adminStatusContextKey, true)
	isAdmin, ok := GetAdminStatusFromContext(ctx)
	if !ok {
		t.Error("Expected GetAdminStatusFromContext to return true, got false")
	}
	if !isAdmin {
		t.Error("Expected admin status to be true, got false")
	}

	// Test case: non-admin user
	ctx = context.Background()
	isAdmin, ok = GetAdminStatusFromContext(ctx)
	if ok {
		t.Error("Expected GetAdminStatusFromContext to return false, got true")
	}
	if isAdmin {
		t.Error("Expected admin status to be false, got true")
	}
}
