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
		response.BadRequestWithCode(c, response.ErrorCodeAuthProfileValidationFailed, profileValidationErr.Error())
	case errors.As(err, &passwordValidationErr):
		response.BadRequestWithCode(c, response.ErrorCodeAuthPasswordValidationFailed, passwordValidationErr.Error())
	case errors.Is(err, authsvc.ErrInvalidCaptcha):
		response.BadRequestWithCode(c, response.ErrorCodeAuthInvalidCaptcha, authsvc.ErrInvalidCaptcha.Error())
	case errors.Is(err, authsvc.ErrInvalidCredentials):
		response.UnauthorizedWithCode(c, response.ErrorCodeAuthInvalidCredentials, authsvc.ErrInvalidCredentials.Error())
	case errors.Is(err, authsvc.ErrUserDisabled):
		response.UnauthorizedWithCode(c, response.ErrorCodeAuthUserDisabled, authsvc.ErrUserDisabled.Error())
	case errors.Is(err, authsvc.ErrOldPasswordIncorrect):
		response.BadRequestWithCode(c, response.ErrorCodeAuthOldPasswordIncorrect, authsvc.ErrOldPasswordIncorrect.Error())
	case errors.Is(err, authsvc.ErrUsernameAlreadyExists):
		response.BadRequestWithCode(c, response.ErrorCodeAuthUsernameAlreadyExists, authsvc.ErrUsernameAlreadyExists.Error())
	case errors.Is(err, authsvc.ErrEmailAlreadyExists):
		response.BadRequestWithCode(c, response.ErrorCodeAuthEmailAlreadyExists, authsvc.ErrEmailAlreadyExists.Error())
	case errors.Is(err, authsvc.ErrPhoneAlreadyExists):
		response.BadRequestWithCode(c, response.ErrorCodeAuthPhoneAlreadyExists, authsvc.ErrPhoneAlreadyExists.Error())
	case errors.Is(err, authsvc.ErrUserNotFound):
		response.NotFoundWithCode(c, response.ErrorCodeAuthUserNotFound, authsvc.ErrUserNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

func writeJWTUnauthorizedError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, jwt.ErrExpiredToken):
		response.UnauthorizedWithCode(c, response.ErrorCodeAuthTokenExpired, "Token has expired")
	case errors.Is(err, jwt.ErrInvalidToken), errors.Is(err, jwt.ErrWrongTokenType):
		response.UnauthorizedWithCode(c, response.ErrorCodeAuthTokenInvalid, "Invalid token")
	case errors.Is(err, jwt.ErrRevokedToken):
		response.UnauthorizedWithCode(c, response.ErrorCodeAuthTokenRevoked, "Token has been revoked")
	default:
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
	}
}
