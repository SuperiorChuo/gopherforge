package system

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/go-admin-kit/server/internal/pkg/response"
	systemsvc "github.com/go-admin-kit/server/internal/service/system"
)

func internalServerError(c *gin.Context, message string, err error) {
	if logger.Logger != nil && err != nil {
		logger.Error(message, logger.Err(err))
	}
	response.InternalServerError(c, message)
}

func writeSystemUserServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrUsernameAlreadyExists):
		response.BadRequest(c, systemsvc.ErrUsernameAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrEmailAlreadyExists):
		response.BadRequest(c, systemsvc.ErrEmailAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrUserNotFound):
		response.NotFound(c, systemsvc.ErrUserNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}
