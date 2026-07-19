package system

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/response"
	systemsvc "github.com/go-admin-kit/services/system/internal/service/system"
)

func internalServerError(c *gin.Context, message string, err error) {
	if logger.Logger != nil && err != nil {
		logger.Error(message, logger.Err(err))
	}
	response.InternalServerError(c, message)
}

func writeSystemMenuServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrParentMenuNotFound):
		response.BadRequestWithCode(c, response.ErrorCodeMenuParentNotFound, systemsvc.ErrParentMenuNotFound.Error())
	case errors.Is(err, systemsvc.ErrMenuParentIsDescendant):
		response.BadRequestWithCode(c, response.ErrorCodeMenuParentIsDescendant, systemsvc.ErrMenuParentIsDescendant.Error())
	case errors.Is(err, systemsvc.ErrMenuHasChildren):
		response.BadRequestWithCode(c, response.ErrorCodeMenuHasChildren, systemsvc.ErrMenuHasChildren.Error())
	case errors.Is(err, systemsvc.ErrMenuNotFound):
		response.NotFoundWithCode(c, response.ErrorCodeMenuNotFound, systemsvc.ErrMenuNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

func writeSystemDictServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrDictTypeCodeAlreadyExists):
		response.BadRequestWithCode(c, response.ErrorCodeDictTypeCodeAlreadyExists, systemsvc.ErrDictTypeCodeAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrDictTypeNotFound):
		response.NotFoundWithCode(c, response.ErrorCodeDictTypeNotFound, systemsvc.ErrDictTypeNotFound.Error())
	case errors.Is(err, systemsvc.ErrDictItemNotFound):
		response.NotFoundWithCode(c, response.ErrorCodeDictItemNotFound, systemsvc.ErrDictItemNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

func writeSystemNoticeServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrNoticeNotFound):
		response.NotFoundWithCode(c, response.ErrorCodeNoticeNotFound, systemsvc.ErrNoticeNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}
