// Copyright 2023 The Overlord Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DatabaseManager manages the SQLite database connection and operations
type DatabaseManager struct {
	db        *gorm.DB
	dbPath    string
	jwtSecret string
}

// User model for authentication
type User struct {
	ID        uint      `gorm:"primaryKey"`
	Username  string    `gorm:"uniqueIndex;size:255;not null"`
	Password  string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	Groups    []Group   `gorm:"many2many:user_groups;"`
}

// Group model for access control
type Group struct {
	ID        uint      `gorm:"primaryKey"`
	Name      string    `gorm:"uniqueIndex;size:255;not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	Users     []User    `gorm:"many2many:user_groups;"`
}

// JWTSecret model to store the JWT signing secret
type JWTSecret struct {
	ID        uint      `gorm:"primaryKey"`
	Secret    string    `gorm:"size:4096;not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// AdminGroupName is the name of the admin group
const AdminGroupName = "admin"

// NewDatabaseManager creates a new database manager
func NewDatabaseManager(dbPath string) *DatabaseManager {
	return &DatabaseManager{
		dbPath: dbPath,
	}
}

// Connect connects to the database
func (dm *DatabaseManager) Connect() error {
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	var err error
	dm.db, err = gorm.Open(sqlite.Open(dm.dbPath), gormConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	// Create tables
	err = dm.db.AutoMigrate(&User{}, &Group{}, &JWTSecret{})
	if err != nil {
		return fmt.Errorf("failed to migrate database: %v", err)
	}
	return nil
}

// GenerateRandomSecret generates a cryptographically secure random string
func GenerateRandomSecret(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

// RegenerateJWTSecret generates a new JWT secret and updates it in the database
func (dm *DatabaseManager) RegenerateJWTSecret() error {
	secret, err := GenerateRandomSecret(64)
	if err != nil {
		return fmt.Errorf("failed to generate random secret: %v", err)
	}

	var jwtSecret JWTSecret
	result := dm.db.First(&jwtSecret)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// Create new record if none exists
			result = dm.db.Create(&JWTSecret{Secret: secret})
		} else {
			return result.Error
		}
	} else {
		// Update existing record
		jwtSecret.Secret = secret
		result = dm.db.Save(&jwtSecret)
	}

	if result.Error != nil {
		return result.Error
	}

	dm.jwtSecret = secret
	log.Println("JWT secret regenerated successfully")
	return nil
}

// ensureAdminGroup ensures that an admin group exists in the database
func (dm *DatabaseManager) ensureAdminGroup() error {
	var count int64
	dm.db.Model(&Group{}).Where("name = ?", AdminGroupName).Count(&count)

	if count == 0 {
		// Create admin group
		adminGroup := Group{
			Name:      AdminGroupName,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		result := dm.db.Create(&adminGroup)
		if result.Error != nil {
			return result.Error
		}

		log.Printf("Created admin group")
	}

	return nil
}

// GetJWTSecret returns the JWT secret from the database
func (dm *DatabaseManager) GetJWTSecret() string {
	return dm.jwtSecret
}

// CreateUser creates a new user in the database
func (dm *DatabaseManager) CreateUser(username, password string, isAdmin bool) error {
	// Check if user already exists
	var count int64
	dm.db.Model(&User{}).Where("username = ?", username).Count(&count)
	if count > 0 {
		return errors.New("user already exists")
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := User{
		Username:  username,
		Password:  string(hashedPassword),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	tx := dm.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	result := tx.Create(&user)
	if result.Error != nil {
		tx.Rollback()
		return result.Error
	}

	// If isAdmin, add user to admin group
	if isAdmin {
		var adminGroup Group
		if err := tx.Where("name = ?", AdminGroupName).First(&adminGroup).Error; err != nil {
			tx.Rollback()
			return err
		}

		if err := tx.Model(&user).Association("Groups").Append(&adminGroup); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

// IsUserAdmin checks if a user is a member of the admin group
func (dm *DatabaseManager) IsUserAdmin(username string) (bool, error) {
	return dm.IsUserInGroup(username, AdminGroupName)
}

// AuthenticateUser authenticates a user with the provided credentials
func (dm *DatabaseManager) AuthenticateUser(username, password string) (bool, error) {
	var user User
	result := dm.db.Where("username = ?", username).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, result.Error
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	return err == nil, nil
}

// GetUserByUsername retrieves a user by username
func (dm *DatabaseManager) GetUserByUsername(username string) (*User, error) {
	var user User
	result := dm.db.Where("username = ?", username).First(&user)
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}

// UpdateUserPassword updates a user's password
func (dm *DatabaseManager) UpdateUserPassword(username, newPassword string) error {
	var user User
	result := dm.db.Where("username = ?", username).First(&user)
	if result.Error != nil {
		return result.Error
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.Password = string(hashedPassword)
	user.UpdatedAt = time.Now()
	dm.db.Save(&user)
	return nil
}

// DeleteUser deletes a user from the database
func (dm *DatabaseManager) DeleteUser(username string) error {
	// Remove the user from admin group
	err := dm.RemoveUserFromGroup(username, AdminGroupName)
	if err != nil {
		return err
	}

	// Remove user from all other groups
	if err := dm.db.Model(&User{}).Where("username = ?", username).Association("Groups").Clear(); err != nil {
		log.Printf("Failed to clear user groups: %v", err)
	}

	result := dm.db.Where("username = ?", username).Delete(&User{})
	return result.Error
}

// CreateGroup creates a new group in the database
func (dm *DatabaseManager) CreateGroup(name string) error {
	var count int64
	dm.db.Model(&Group{}).Where("name = ?", name).Count(&count)
	if count > 0 {
		return errors.New("group already exists")
	}

	group := Group{
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	result := dm.db.Create(&group)
	return result.Error
}

// GetGroupByName retrieves a group by name
func (dm *DatabaseManager) GetGroupByName(name string) (*Group, error) {
	var group Group
	result := dm.db.Where("name = ?", name).First(&group)
	if result.Error != nil {
		return nil, result.Error
	}
	return &group, nil
}

// DeleteGroup deletes a group from the database
func (dm *DatabaseManager) DeleteGroup(name string) error {
	// Prevent deletion of admin group
	if name == AdminGroupName {
		return errors.New("cannot delete admin group")
	}

	result := dm.db.Where("name = ?", name).Delete(&Group{})
	return result.Error
}

// AddUserToGroup adds a user to a group
func (dm *DatabaseManager) AddUserToGroup(username, groupName string) error {
	var user User
	var group Group

	if err := dm.db.Where("username = ?", username).First(&user).Error; err != nil {
		return fmt.Errorf("user not found: %v", err)
	}

	if err := dm.db.Where("name = ?", groupName).First(&group).Error; err != nil {
		return fmt.Errorf("group not found: %v", err)
	}

	// Add the user to the group
	if err := dm.db.Model(&user).Association("Groups").Append(&group); err != nil {
		return fmt.Errorf("failed to add user to group: %v", err)
	}

	return nil
}

// RemoveUserFromGroup removes a user from a group
func (dm *DatabaseManager) RemoveUserFromGroup(username, groupName string) error {
	// Prevent removing the last admin
	if groupName == AdminGroupName {
		var adminCount int64
		adminGroup := Group{}
		if err := dm.db.Where("name = ?", AdminGroupName).
			First(&adminGroup).Error; err != nil {
			return err
		}

		adminCount = dm.db.Model(&adminGroup).Association("Users").Count()

		if adminCount <= 1 {
			var user User
			if err := dm.db.Where("username = ?", username).
				First(&user).Error; err != nil {
				return fmt.Errorf("user not found: %v", err)
			}

			// Check if this user is actually in the admin group
			isAdmin, err := dm.IsUserInGroup(username, AdminGroupName)
			if err != nil {
				return err
			}

			if isAdmin && adminCount <= 1 {
				return errors.New("cannot remove the last admin user")
			}
		}
	}

	var user User
	var group Group

	if err := dm.db.Where("username = ?", username).
		First(&user).Error; err != nil {
		return fmt.Errorf("user not found: %v", err)
	}

	if err := dm.db.Where("name = ?", groupName).
		First(&group).Error; err != nil {
		return fmt.Errorf("group not found: %v", err)
	}

	// Remove the user from the group
	if err := dm.db.Model(&user).Association("Groups").
		Delete(&group); err != nil {
		return fmt.Errorf("failed to remove user from group: %v", err)
	}

	return nil
}

// GetUserGroups returns all groups that a user belongs to
func (dm *DatabaseManager) GetUserGroups(username string) ([]string, error) {
	var user User
	if err := dm.db.Where("username = ?", username).Preload("Groups").
		First(&user).Error; err != nil {
		return nil, fmt.Errorf("user not found: %v", err)
	}

	var groupNames []string
	for _, group := range user.Groups {
		groupNames = append(groupNames, group.Name)
	}

	return groupNames, nil
}

// IsUserInGroup checks if a user is a member of a specific group
func (dm *DatabaseManager) IsUserInGroup(username, groupName string) (bool, error) {
	groups, err := dm.GetUserGroups(username)
	if err != nil {
		return false, err
	}

	return slices.Contains(groups, groupName), nil
}

// ParseEntityName parses an entity string (u/username or g/groupname) and returns the type and name
func ParseEntityName(entity string) (string, string, error) {
	if !strings.Contains(entity, "/") {
		return "", "", errors.New("invalid entity format, expected u/username or g/groupname")
	}

	parts := strings.SplitN(entity, "/", 2)
	if len(parts) != 2 {
		return "", "", errors.New("invalid entity format, expected u/username or g/groupname")
	}

	entityType := parts[0]
	entityName := parts[1]

	if entityType != "u" && entityType != "g" {
		return "", "", errors.New("invalid entity type, expected 'u' for user or 'g' for group")
	}

	return entityType, entityName, nil
}

// CheckAllowlist checks if a user has access based on the allowlist
func (dm *DatabaseManager) CheckAllowlist(username string, allowlist []string) (bool, error) {
	if len(allowlist) == 0 {
		return false, nil
	}

	// Check each entity in the allowlist
	for _, entity := range allowlist {
		// Special case: "anyone" grants access to everyone
		if entity == EntityAnyone {
			return true, nil
		}

		entityType, entityName, err := ParseEntityName(entity)
		if err != nil {
			return false, err
		}

		// User entity
		if entityType == "u" && entityName == username {
			return true, nil
		}

		// Group entity
		if entityType == "g" {
			isInGroup, err := dm.IsUserInGroup(username, entityName)
			if err != nil {
				continue
			}
			if isInGroup {
				return true, nil
			}
		}
	}
	return false, nil
}

// GetAdminCount returns the number of admin users
func (dm *DatabaseManager) GetAdminCount() (int64, error) {
	var count int64
	result := dm.db.Model(&Group{}).Where("name = ?", AdminGroupName).
		Preload("Users").Count(&count)
	return count, result.Error
}

// GetAllUsers returns all users
func (dm *DatabaseManager) GetAllUsers() ([]User, error) {
	var users []User
	result := dm.db.Find(&users)
	return users, result.Error
}

// GetAllGroups returns all groups
func (dm *DatabaseManager) GetAllGroups() ([]Group, error) {
	var groups []Group
	result := dm.db.Preload("Users").Find(&groups)
	return groups, result.Error
}

// GetGroupUsers returns all users in a group
func (dm *DatabaseManager) GetGroupUsers(groupName string) ([]User, error) {
	var group Group
	result := dm.db.Where("name = ?", groupName).Preload("Users").First(&group)
	if result.Error != nil {
		return nil, result.Error
	}
	return group.Users, nil
}

// Initialize initializes the database.
func (dm *DatabaseManager) Initialize(username, password string) error {
	if err := dm.Connect(); err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	if err := dm.RegenerateJWTSecret(); err != nil {
		return fmt.Errorf("failed to regenerate JWT secret: %v", err)
	}

	err := dm.ensureAdminGroup()
	if err != nil {
		return fmt.Errorf("failed to ensure admin group: %v", err)
	}

	err = dm.CreateUser(username, password, true)
	if err != nil {
		return fmt.Errorf("failed to create admin user: %v", err)
	}

	log.Println("Database initialized successfully")
	return nil
}
