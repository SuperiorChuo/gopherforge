package system

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/file/internal/pkg/logger"
	"github.com/go-admin-kit/services/file/internal/pkg/response"
	"github.com/go-admin-kit/services/file/internal/pkg/upload"
	systemsvc "github.com/go-admin-kit/services/file/internal/service/system"
)

func internalServerError(c *gin.Context, message string, err error) {
	if logger.Logger != nil && err != nil {
		logger.Error(message, logger.Err(err))
	}
	response.InternalServerError(c, message)
}

func writeSystemFileServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrFileNotFoundOrPermissionDenied):
		response.NotFoundWithCode(c, response.ErrorCodeFileNotFoundOrPermissionDenied, systemsvc.ErrFileNotFoundOrPermissionDenied.Error())
	case errors.Is(err, upload.ErrFileEmpty):
		response.BadRequestWithCode(c, response.ErrorCodeFileEmpty, upload.ErrFileEmpty.Error())
	case errors.Is(err, upload.ErrFileTooLarge):
		response.BadRequestWithCode(c, response.ErrorCodeFileTooLarge, upload.ErrFileTooLarge.Error())
	case errors.Is(err, upload.ErrFileTypeNotAllowed):
		response.BadRequestWithCode(c, response.ErrorCodeFileTypeNotAllowed, upload.ErrFileTypeNotAllowed.Error())
	case errors.Is(err, upload.ErrStoredObjectNotFound):
		response.NotFoundWithCode(c, response.ErrorCodeFileNotFoundOrPermissionDenied, systemsvc.ErrFileNotFoundOrPermissionDenied.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

func systemFileServiceErrorMessage(err error) string {
	switch {
	case errors.Is(err, systemsvc.ErrFileNotFoundOrPermissionDenied):
		return systemsvc.ErrFileNotFoundOrPermissionDenied.Error()
	case errors.Is(err, upload.ErrFileEmpty):
		return upload.ErrFileEmpty.Error()
	case errors.Is(err, upload.ErrFileTooLarge):
		return upload.ErrFileTooLarge.Error()
	case errors.Is(err, upload.ErrFileTypeNotAllowed):
		return upload.ErrFileTypeNotAllowed.Error()
	default:
		return "internal server error"
	}
}
