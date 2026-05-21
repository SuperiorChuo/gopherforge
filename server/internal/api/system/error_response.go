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

func writeSystemRoleServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrRoleCodeAlreadyExists):
		response.BadRequest(c, systemsvc.ErrRoleCodeAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrInvalidRoleDataScope):
		response.BadRequest(c, systemsvc.ErrInvalidRoleDataScope.Error())
	case errors.Is(err, systemsvc.ErrCustomDataScopeRequiresDepartments):
		response.BadRequest(c, systemsvc.ErrCustomDataScopeRequiresDepartments.Error())
	case errors.Is(err, systemsvc.ErrRoleNotFound):
		response.NotFound(c, systemsvc.ErrRoleNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

func writeSystemPermissionServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrPermissionCodeAlreadyExists):
		response.BadRequest(c, systemsvc.ErrPermissionCodeAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrParentPermissionNotFound):
		response.BadRequest(c, systemsvc.ErrParentPermissionNotFound.Error())
	case errors.Is(err, systemsvc.ErrPermissionParentIsDescendant):
		response.BadRequest(c, systemsvc.ErrPermissionParentIsDescendant.Error())
	case errors.Is(err, systemsvc.ErrPermissionNotFound):
		response.NotFound(c, systemsvc.ErrPermissionNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

func writeSystemMenuServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrParentMenuNotFound):
		response.BadRequest(c, systemsvc.ErrParentMenuNotFound.Error())
	case errors.Is(err, systemsvc.ErrMenuParentIsDescendant):
		response.BadRequest(c, systemsvc.ErrMenuParentIsDescendant.Error())
	case errors.Is(err, systemsvc.ErrMenuHasChildren):
		response.BadRequest(c, systemsvc.ErrMenuHasChildren.Error())
	case errors.Is(err, systemsvc.ErrMenuNotFound):
		response.NotFound(c, systemsvc.ErrMenuNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

func writeSystemDepartmentServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrDepartmentCodeAlreadyExists):
		response.BadRequest(c, systemsvc.ErrDepartmentCodeAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrParentDepartmentNotFound):
		response.BadRequest(c, systemsvc.ErrParentDepartmentNotFound.Error())
	case errors.Is(err, systemsvc.ErrDepartmentSelfParent):
		response.BadRequest(c, systemsvc.ErrDepartmentSelfParent.Error())
	case errors.Is(err, systemsvc.ErrDepartmentHasChildren):
		response.BadRequest(c, systemsvc.ErrDepartmentHasChildren.Error())
	case errors.Is(err, systemsvc.ErrDepartmentHasUsers):
		response.BadRequest(c, systemsvc.ErrDepartmentHasUsers.Error())
	case errors.Is(err, systemsvc.ErrDepartmentNotFound):
		response.NotFound(c, systemsvc.ErrDepartmentNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

func writeSystemDictServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrDictTypeCodeAlreadyExists):
		response.BadRequest(c, systemsvc.ErrDictTypeCodeAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrDictTypeNotFound):
		response.NotFound(c, systemsvc.ErrDictTypeNotFound.Error())
	case errors.Is(err, systemsvc.ErrDictItemNotFound):
		response.NotFound(c, systemsvc.ErrDictItemNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}
