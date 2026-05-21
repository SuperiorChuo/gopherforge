package system

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/pkg/upload"
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
		response.BadRequestWithCode(c, response.ErrorCodeUsernameAlreadyExists, systemsvc.ErrUsernameAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrEmailAlreadyExists):
		response.BadRequestWithCode(c, response.ErrorCodeEmailAlreadyExists, systemsvc.ErrEmailAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrUserNotFound):
		response.NotFoundWithCode(c, response.ErrorCodeUserNotFound, systemsvc.ErrUserNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

func writeSystemRoleServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrRoleCodeAlreadyExists):
		response.BadRequestWithCode(c, response.ErrorCodeRoleCodeAlreadyExists, systemsvc.ErrRoleCodeAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrInvalidRoleDataScope):
		response.BadRequestWithCode(c, response.ErrorCodeRoleInvalidDataScope, systemsvc.ErrInvalidRoleDataScope.Error())
	case errors.Is(err, systemsvc.ErrCustomDataScopeRequiresDepartments):
		response.BadRequestWithCode(c, response.ErrorCodeRoleCustomDataScopeRequiresDepartments, systemsvc.ErrCustomDataScopeRequiresDepartments.Error())
	case errors.Is(err, systemsvc.ErrRoleNotFound):
		response.NotFoundWithCode(c, response.ErrorCodeRoleNotFound, systemsvc.ErrRoleNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

func writeSystemPermissionServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrPermissionCodeAlreadyExists):
		response.BadRequestWithCode(c, response.ErrorCodePermissionCodeAlreadyExists, systemsvc.ErrPermissionCodeAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrParentPermissionNotFound):
		response.BadRequestWithCode(c, response.ErrorCodePermissionParentNotFound, systemsvc.ErrParentPermissionNotFound.Error())
	case errors.Is(err, systemsvc.ErrPermissionParentIsDescendant):
		response.BadRequestWithCode(c, response.ErrorCodePermissionParentIsDescendant, systemsvc.ErrPermissionParentIsDescendant.Error())
	case errors.Is(err, systemsvc.ErrPermissionNotFound):
		response.NotFoundWithCode(c, response.ErrorCodePermissionNotFound, systemsvc.ErrPermissionNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
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

func writeSystemDepartmentServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrDepartmentCodeAlreadyExists):
		response.BadRequestWithCode(c, response.ErrorCodeDepartmentCodeAlreadyExists, systemsvc.ErrDepartmentCodeAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrParentDepartmentNotFound):
		response.BadRequestWithCode(c, response.ErrorCodeDepartmentParentNotFound, systemsvc.ErrParentDepartmentNotFound.Error())
	case errors.Is(err, systemsvc.ErrDepartmentSelfParent):
		response.BadRequestWithCode(c, response.ErrorCodeDepartmentSelfParent, systemsvc.ErrDepartmentSelfParent.Error())
	case errors.Is(err, systemsvc.ErrDepartmentHasChildren):
		response.BadRequestWithCode(c, response.ErrorCodeDepartmentHasChildren, systemsvc.ErrDepartmentHasChildren.Error())
	case errors.Is(err, systemsvc.ErrDepartmentHasUsers):
		response.BadRequestWithCode(c, response.ErrorCodeDepartmentHasUsers, systemsvc.ErrDepartmentHasUsers.Error())
	case errors.Is(err, systemsvc.ErrDepartmentNotFound):
		response.NotFoundWithCode(c, response.ErrorCodeDepartmentNotFound, systemsvc.ErrDepartmentNotFound.Error())
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

func writeSystemOperationLogServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrOperationLogNotFound):
		response.NotFoundWithCode(c, response.ErrorCodeOperationLogNotFound, systemsvc.ErrOperationLogNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}
