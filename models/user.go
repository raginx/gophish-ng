package models

import (
	"errors"
	"time"

	log "github.com/raginx/gophish-ng/logger"
)

// ErrModifyingOnlyAdmin occurs when there is an attempt to modify the only
// user account with the Admin role in such a way that there will be no user
// accounts left in Gophish with that role.
var ErrModifyingOnlyAdmin = errors.New("cannot remove the only administrator")

// User represents the user model for gophish.
type User struct {
	Id       int64  `json:"id"`
	Username string `json:"username" gorm:"not null;unique"`
	Hash     string `json:"-"`
	ApiKey   string `json:"api_key" gorm:"not null;unique"`
	// gorm v2 defaults to not auto-saving/creating associations on Save,
	// which is the behavior the v1 association_autoupdate/autocreate tags
	// used to opt into explicitly.
	Role                   Role      `json:"role"`
	RoleID                 int64     `json:"-"`
	Team                   Team      `json:"team"`
	TeamID                 int64     `json:"-"`
	PasswordChangeRequired bool      `json:"password_change_required"`
	AccountLocked          bool      `json:"account_locked"`
	LastLogin              time.Time `json:"last_login"`
}

// GetUser returns the user that the given id corresponds to. If no user is found, an
// error is thrown.
func GetUser(id int64) (User, error) {
	u := User{}
	err := db.Preload("Role").Preload("Team").Where("id=?", id).First(&u).Error
	return u, err
}

// GetUsers returns the users registered in Gophish
func GetUsers() ([]User, error) {
	us := []User{}
	err := db.Preload("Role").Preload("Team").Find(&us).Error
	return us, err
}

// GetUserByAPIKey returns the user that the given API Key corresponds to. If no user is found, an
// error is thrown.
func GetUserByAPIKey(key string) (User, error) {
	u := User{}
	err := db.Preload("Role").Preload("Team").Where("api_key = ?", key).First(&u).Error
	return u, err
}

// GetUserByUsername returns the user that the given username corresponds to. If no user is found, an
// error is thrown.
func GetUserByUsername(username string) (User, error) {
	u := User{}
	err := db.Preload("Role").Preload("Team").Where("username = ?", username).First(&u).Error
	return u, err
}

// PutUser updates the given user
func PutUser(u *User) error {
	err := db.Save(u).Error
	return err
}

// EnsureEnoughAdmins ensures that there is more than one user account in
// Gophish with the Admin role. This function is meant to be called before
// modifying a user account with the Admin role in a non-revokable way.
func EnsureEnoughAdmins() error {
	role, err := GetRoleBySlug(RoleAdmin)
	if err != nil {
		return err
	}
	var adminCount int64
	err = db.Model(&User{}).Where("role_id=?", role.ID).Count(&adminCount).Error
	if err != nil {
		return err
	}
	if adminCount == 1 {
		return ErrModifyingOnlyAdmin
	}
	return nil
}

// DeleteUser deletes the given user account. To ensure that there is always
// at least one user account with the Admin role, this function will refuse
// to delete the last Admin.
//
// This only removes the account itself - campaigns, pages, templates,
// groups, and sending profiles the user created are team-owned (visible to
// and editable by every team member, see rbac.go's design note), so they're
// left in place for the rest of the team rather than cascade-deleted.
func DeleteUser(id int64) error {
	existing, err := GetUser(id)
	if err != nil {
		return err
	}
	// If the user is an admin, we need to verify that it's not the last one.
	if existing.Role.Slug == RoleAdmin {
		err = EnsureEnoughAdmins()
		if err != nil {
			return err
		}
	}
	log.Infof("Deleting user ID %d", id)
	err = db.Where("id=?", id).Delete(&User{}).Error
	return err
}
