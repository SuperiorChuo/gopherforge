package auth

import (
	"time"

	"github.com/go-admin-kit/server/internal/model"
)

// UserInfoResponse is the user profile response DTO.
type UserInfoResponse struct {
	ID                 uint      `json:"id"`
	Username           string    `json:"username"`
	Email              string    `json:"email" mask:"email"`
	Phone              string    `json:"phone" mask:"phone"`
	Nickname           string    `json:"nickname"`
	Avatar             string    `json:"avatar"`
	Status             int8      `json:"status"`
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Roles              []RoleDTO `json:"roles,omitempty"`
	Permissions        []string  `json:"permissions"`
}

// LoginResponseData is the login response payload DTO.
type LoginResponseData struct {
	User         *UserInfoResponse `json:"user"`
	AccessToken  string            `json:"access_token"`
	RefreshToken string            `json:"refresh_token"`
}

// RoleDTO is the role response DTO.
type RoleDTO struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description,omitempty"`
}

// ConvertUserToResponse converts a user model to the response DTO.
func ConvertUserToResponse(user *model.User, permissions []string) *UserInfoResponse {
	return &UserInfoResponse{
		ID:                 user.ID,
		Username:           user.Username,
		Email:              user.Email,
		Phone:              user.Phone,
		Nickname:           user.Nickname,
		Avatar:             user.Avatar,
		Status:             user.Status,
		MustChangePassword: user.MustChangePassword,
		CreatedAt:          user.CreatedAt,
		UpdatedAt:          user.UpdatedAt,
		Roles:              ConvertRolesToDTO(user.Roles),
		Permissions:        permissions,
	}
}

// ConvertRolesToDTO converts role models to response DTOs.
func ConvertRolesToDTO(roles []model.Role) []RoleDTO {
	if len(roles) == 0 {
		return []RoleDTO{}
	}

	roleDTOs := make([]RoleDTO, 0, len(roles))
	for _, role := range roles {
		roleDTOs = append(roleDTOs, RoleDTO{
			ID:          role.ID,
			Name:        role.Name,
			Code:        role.Code,
			Description: role.Description,
		})
	}
	return roleDTOs
}
