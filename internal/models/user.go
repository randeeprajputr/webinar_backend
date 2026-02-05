package models

import (
	"time"

	"github.com/google/uuid"
)

// Role represents user role in the platform.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleSpeaker  Role = "speaker"
	RoleAudience Role = "audience"
)

// User represents a platform user.
type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	FullName  string    `json:"full_name"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserPublic is User without sensitive fields for API responses.
type UserPublic struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// ToPublic converts User to UserPublic.
func (u *User) ToPublic() UserPublic {
	return UserPublic{
		ID:        u.ID,
		Email:     u.Email,
		FullName:  u.FullName,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
	}
}
