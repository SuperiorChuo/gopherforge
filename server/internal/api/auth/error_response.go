package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/jwt"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/go-admin-kit/server/internal/pkg/response"
	authsvc "github.com/go-admin-kit/server/internal/service/auth"
)

func internalServerError(c *gin.Context, message string, err error) {
	if logger.Logger != nil && err != nil {
		logger.Error(message, logger.Err(err))
	}
	response.InternalServerError(c, message)
}

func writeAuthServiceError(c *gin.Context, operation string, err error) {
	var profileValidationErr authsvc.ProfileValidationError
	var passwordValidationErr authsvc.PasswordValidationError

	switch {
	case errors.As(err, &profileValidationErr):
		response.BadRequest(c, profileValidationErr.Error())
	case errors.As(err, &passwordValidationErr):
		response.BadRequest(c, passwordValidationErr.Error())
	case errors.Is(err, authsvc.ErrInvalidCaptcha):
		response.BadRequest(c, authsvc.ErrInvalidCaptcha.Error())
	case errors.Is(err, authsvc.ErrInvalidCredentials):
		response.Unauthorized(c, authsvc.ErrInvalidCredentials.Error())
	case errors.Is(err, authsvc.ErrUserDisabled):
		response.Unauthorized(c, authsvc.ErrUserDisabled.Error())
	case errors.Is(err, authsvc.ErrOldPasswordIncorrect):
		response.BadRequest(c, authsvc.ErrOldPasswordIncorrect.Error())
	case errors.Is(err, authsvc.ErrUsernameAlreadyExists):
		response.BadRequest(c, authsvc.ErrUsernameAlreadyExists.Error())
	case errors.Is(err, authsvc.ErrEmailAlreadyExists):
		response.BadRequest(c, authsvc.ErrEmailAlreadyExists.Error())
	case errors.Is(err, authsvc.ErrPhoneAlreadyExists):
		response.BadRequest(c, authsvc.ErrPhoneAlreadyExists.Error())
	case errors.Is(err, authsvc.ErrUserNotFound):
		response.NotFound(c, authsvc.ErrUserNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

func writeJWTUnauthorizedError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, jwt.ErrExpiredToken):
		response.Unauthorized(c, "Token has expired")
	case errors.Is(err, jwt.ErrInvalidToken), errors.Is(err, jwt.ErrWrongTokenType):
		response.Unauthorized(c, "Invalid token")
	case errors.Is(err, jwt.ErrRevokedToken):
		response.Unauthorized(c, "Token has been revoked")
	default:
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
	}
}
