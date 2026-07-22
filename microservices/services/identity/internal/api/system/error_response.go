package system

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	authsvc "github.com/go-admin-kit/services/identity/internal/service/auth"
	systemsvc "github.com/go-admin-kit/services/identity/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

func internalServerError(c *gin.Context, message string, err error) {
	if logger.Logger != nil && err != nil {
		logger.Error(message, logger.Err(err))
	}
	response.InternalServerError(c, message)
}

func writeSystemUserServiceError(c *gin.Context, operation string, err error) {
	var passwordValidationErr authsvc.PasswordValidationError
	switch {
	case errors.As(err, &passwordValidationErr):
		response.BadRequestWithCode(c, response.ErrorCodeAuthPasswordValidationFailed, passwordValidationErr.Error())
	case errors.Is(err, systemsvc.ErrUsernameAlreadyExists):
		response.BadRequestWithCode(c, response.ErrorCodeUsernameAlreadyExists, systemsvc.ErrUsernameAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrEmailAlreadyExists):
		response.BadRequestWithCode(c, response.ErrorCodeEmailAlreadyExists, systemsvc.ErrEmailAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrUserNotFound):
		response.NotFoundWithCode(c, response.ErrorCodeUserNotFound, systemsvc.ErrUserNotFound.Error())
	case errors.Is(err, systemsvc.ErrRoleNotInTenant), errors.Is(err, systemsvc.ErrDepartmentNotInTenant):
		response.BadRequest(c, err.Error())
	case errors.Is(err, systemsvc.ErrTenantUserQuota):
		response.BadRequest(c, err.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

func writeSystemRoleServiceError(c *gin.Context, operation string, err error) {
	var exceedErr *systemsvc.PermissionsExceedPackageError
	switch {
	case errors.As(err, &exceedErr):
		// 越界分配：把具体越界权限码回给前端，便于租户管理员定位。
		response.BadRequest(c, exceedErr.Error())
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
