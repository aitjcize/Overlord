package overlord

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

// listUsersHandler lists all users
func (ovl *Overlord) listUsersHandler(w http.ResponseWriter, r *http.Request) {
	users, err := ovl.dbManager.GetAllUsers()
	if err != nil {
		ResponseError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Only return safe user data (exclude passwords)
	type UserResponse struct {
		Username string   `json:"username"`
		IsAdmin  bool     `json:"is_admin"`
		Groups   []string `json:"groups"`
	}

	var userResponses []UserResponse
	for _, user := range users {
		groups, err := ovl.dbManager.GetUserGroups(user.Username)
		if err != nil {
			log.Printf("Error retrieving groups for user %s: %v", user.Username, err)
			groups = []string{}
		}

		isAdmin, err := ovl.dbManager.IsUserAdmin(user.Username)
		if err != nil {
			log.Printf("Error checking if user %s is admin: %v", user.Username, err)
			isAdmin = false
		}

		userResponses = append(userResponses, UserResponse{
			Username: user.Username,
			IsAdmin:  isAdmin,
			Groups:   groups,
		})
	}

	ResponseSuccess(w, userResponses)
}

// createUserHandler creates a new user
func (ovl *Overlord) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var createRequest struct {
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
	}

	if err := json.NewDecoder(r.Body).Decode(&createRequest); err != nil {
		ResponseError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if createRequest.Username == "" || createRequest.Password == "" {
		ResponseError(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	// Create the user
	err := ovl.dbManager.CreateUser(createRequest.Username, createRequest.Password,
		createRequest.IsAdmin)
	if err != nil {
		ResponseError(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("User created: %s", createRequest.Username)
	ResponseSuccess(w, map[string]string{"message": "User created successfully"})
}

// deleteUserHandler deletes a user
func (ovl *Overlord) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetUsername := vars["username"]

	err := ovl.dbManager.DeleteUser(targetUsername)
	if err != nil {
		ResponseError(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("User deleted: %s", targetUsername)
	ResponseSuccess(w, map[string]string{"message": "User deleted successfully"})
}

// updateUserPasswordHandler updates a user's password
func (ovl *Overlord) updateUserPasswordHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetUsername := vars["username"]

	var updateRequest struct {
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&updateRequest); err != nil {
		ResponseError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if updateRequest.Password == "" {
		ResponseError(w, "Password is required", http.StatusBadRequest)
		return
	}

	err := ovl.dbManager.UpdateUserPassword(targetUsername, updateRequest.Password)
	if err != nil {
		ResponseError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("User password updated: %s", targetUsername)
	ResponseSuccess(w, map[string]string{"message": "Password updated successfully"})
}

// Add handlers for group management

// listGroupsHandler lists all groups
func (ovl *Overlord) listGroupsHandler(w http.ResponseWriter, r *http.Request) {
	groups, err := ovl.dbManager.GetAllGroups()
	if err != nil {
		ResponseError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to response format
	type GroupResponse struct {
		Name      string `json:"name"`
		UserCount int    `json:"user_count"`
	}

	var groupResponses []GroupResponse
	for _, group := range groups {
		userCount := len(group.Users)
		groupResponses = append(groupResponses, GroupResponse{
			Name:      group.Name,
			UserCount: userCount,
		})
	}

	ResponseSuccess(w, groupResponses)
}

// createGroupHandler creates a new group
func (ovl *Overlord) createGroupHandler(w http.ResponseWriter, r *http.Request) {
	var createRequest struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&createRequest); err != nil {
		ResponseError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if createRequest.Name == "" {
		ResponseError(w, "Group name is required", http.StatusBadRequest)
		return
	}

	// Create the group
	var err error
	err = ovl.dbManager.CreateGroup(createRequest.Name)
	if err != nil {
		ResponseError(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("Group created: %s", createRequest.Name)
	ResponseSuccess(w, map[string]string{"message": "Group created successfully"})
}

// deleteGroupHandler deletes a group
func (ovl *Overlord) deleteGroupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	groupName := vars["groupname"]

	err := ovl.dbManager.DeleteGroup(groupName)
	if err != nil {
		ResponseError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Group deleted: %s", groupName)
	ResponseSuccess(w, map[string]string{"message": "Group deleted successfully"})
}

// addUserToGroupHandler adds a user to a group
func (ovl *Overlord) addUserToGroupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	groupName := vars["groupname"]

	var addRequest struct {
		Username string `json:"username"`
	}

	decodeErr := json.NewDecoder(r.Body).Decode(&addRequest)
	if decodeErr != nil {
		ResponseError(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if addRequest.Username == "" {
		ResponseError(w, "Username is required", http.StatusBadRequest)
		return
	}

	dbErr := ovl.dbManager.AddUserToGroup(addRequest.Username, groupName)
	if dbErr != nil {
		ResponseError(w, dbErr.Error(), http.StatusInternalServerError)
		return
	}

	ResponseSuccess(w, map[string]string{
		"message": "User added to group successfully"})
}

// removeUserFromGroupHandler removes a user from a group
func (ovl *Overlord) removeUserFromGroupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	groupName := vars["groupname"]
	targetUsername := vars["username"]

	err := ovl.dbManager.RemoveUserFromGroup(targetUsername, groupName)
	if err != nil {
		ResponseError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ResponseSuccess(w, map[string]string{
		"message": "User removed from group successfully"})
}

// listGroupUsersHandler lists all users in a group
func (ovl *Overlord) listGroupUsersHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	groupName := vars["groupname"]

	users, err := ovl.dbManager.GetGroupUsers(groupName)
	if err != nil {
		ResponseError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Only return usernames
	var usernames []string
	for _, user := range users {
		usernames = append(usernames, user.Username)
	}

	ResponseSuccess(w, usernames)
}

// adminRequired is a middleware that checks if the user is an admin
func (ovl *Overlord) adminRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isAdmin, ok := GetAdminStatusFromContext(r.Context())
		if !ok || !isAdmin {
			http.Error(w, "Admin privileges required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Add the new handler for changing own password
type updatePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (ovl *Overlord) updateOwnPasswordHandler(w http.ResponseWriter, r *http.Request) {
	// Get the authenticated user from the context
	username, ok := GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	log.Printf("Updating password for user: %s", username)

	var updateRequest updatePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&updateRequest); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Verify current password
	authenticated, err := ovl.dbManager.AuthenticateUser(
		username, updateRequest.CurrentPassword)
	if err != nil || !authenticated {
		http.Error(w, "Current password is incorrect", http.StatusUnauthorized)
		return
	}

	// Update password
	if err := ovl.dbManager.UpdateUserPassword(
		username, updateRequest.NewPassword); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update password: %v", err),
			http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"data":   "Password updated successfully",
	})
}
