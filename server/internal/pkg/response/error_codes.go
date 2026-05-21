package response

import "net/http"

// ErrorCode is a stable machine-readable API error code.
type ErrorCode string

const (
	ErrorCodeBadRequest          ErrorCode = "BAD_REQUEST"
	ErrorCodeUnauthorized        ErrorCode = "UNAUTHORIZED"
	ErrorCodeForbidden           ErrorCode = "FORBIDDEN"
	ErrorCodeNotFound            ErrorCode = "NOT_FOUND"
	ErrorCodeTooManyRequests     ErrorCode = "TOO_MANY_REQUESTS"
	ErrorCodeServiceUnavailable  ErrorCode = "SERVICE_UNAVAILABLE"
	ErrorCodeInternalServerError ErrorCode = "INTERNAL_SERVER_ERROR"

	ErrorCodeAuthProfileValidationFailed  ErrorCode = "AUTH_PROFILE_VALIDATION_FAILED"
	ErrorCodeAuthPasswordValidationFailed ErrorCode = "AUTH_PASSWORD_VALIDATION_FAILED"
	ErrorCodeAuthInvalidCaptcha           ErrorCode = "AUTH_INVALID_CAPTCHA"
	ErrorCodeAuthInvalidCredentials       ErrorCode = "AUTH_INVALID_CREDENTIALS"
	ErrorCodeAuthUserDisabled             ErrorCode = "AUTH_USER_DISABLED"
	ErrorCodeAuthOldPasswordIncorrect     ErrorCode = "AUTH_OLD_PASSWORD_INCORRECT"
	ErrorCodeAuthUsernameAlreadyExists    ErrorCode = "AUTH_USERNAME_ALREADY_EXISTS"
	ErrorCodeAuthEmailAlreadyExists       ErrorCode = "AUTH_EMAIL_ALREADY_EXISTS"
	ErrorCodeAuthPhoneAlreadyExists       ErrorCode = "AUTH_PHONE_ALREADY_EXISTS"
	ErrorCodeAuthUserNotFound             ErrorCode = "AUTH_USER_NOT_FOUND"
	ErrorCodeAuthTokenExpired             ErrorCode = "AUTH_TOKEN_EXPIRED"
	ErrorCodeAuthTokenInvalid             ErrorCode = "AUTH_TOKEN_INVALID"
	ErrorCodeAuthTokenRevoked             ErrorCode = "AUTH_TOKEN_REVOKED"

	ErrorCodeUsernameAlreadyExists ErrorCode = "USER_USERNAME_ALREADY_EXISTS"
	ErrorCodeEmailAlreadyExists    ErrorCode = "USER_EMAIL_ALREADY_EXISTS"
	ErrorCodeUserNotFound          ErrorCode = "USER_NOT_FOUND"

	ErrorCodeRoleCodeAlreadyExists                  ErrorCode = "ROLE_CODE_ALREADY_EXISTS"
	ErrorCodeRoleInvalidDataScope                   ErrorCode = "ROLE_INVALID_DATA_SCOPE"
	ErrorCodeRoleCustomDataScopeRequiresDepartments ErrorCode = "ROLE_CUSTOM_DATA_SCOPE_REQUIRES_DEPARTMENTS"
	ErrorCodeRoleNotFound                           ErrorCode = "ROLE_NOT_FOUND"
	ErrorCodePermissionCodeAlreadyExists            ErrorCode = "PERMISSION_CODE_ALREADY_EXISTS"
	ErrorCodePermissionParentNotFound               ErrorCode = "PERMISSION_PARENT_NOT_FOUND"
	ErrorCodePermissionParentIsDescendant           ErrorCode = "PERMISSION_PARENT_IS_DESCENDANT"
	ErrorCodePermissionNotFound                     ErrorCode = "PERMISSION_NOT_FOUND"
	ErrorCodeMenuParentNotFound                     ErrorCode = "MENU_PARENT_NOT_FOUND"
	ErrorCodeMenuParentIsDescendant                 ErrorCode = "MENU_PARENT_IS_DESCENDANT"
	ErrorCodeMenuHasChildren                        ErrorCode = "MENU_HAS_CHILDREN"
	ErrorCodeMenuNotFound                           ErrorCode = "MENU_NOT_FOUND"
	ErrorCodeDepartmentCodeAlreadyExists            ErrorCode = "DEPARTMENT_CODE_ALREADY_EXISTS"
	ErrorCodeDepartmentParentNotFound               ErrorCode = "DEPARTMENT_PARENT_NOT_FOUND"
	ErrorCodeDepartmentSelfParent                   ErrorCode = "DEPARTMENT_SELF_PARENT"
	ErrorCodeDepartmentHasChildren                  ErrorCode = "DEPARTMENT_HAS_CHILDREN"
	ErrorCodeDepartmentHasUsers                     ErrorCode = "DEPARTMENT_HAS_USERS"
	ErrorCodeDepartmentNotFound                     ErrorCode = "DEPARTMENT_NOT_FOUND"
	ErrorCodeDictTypeCodeAlreadyExists              ErrorCode = "DICT_TYPE_CODE_ALREADY_EXISTS"
	ErrorCodeDictTypeNotFound                       ErrorCode = "DICT_TYPE_NOT_FOUND"
	ErrorCodeDictItemNotFound                       ErrorCode = "DICT_ITEM_NOT_FOUND"
	ErrorCodeNoticeNotFound                         ErrorCode = "NOTICE_NOT_FOUND"
	ErrorCodeFileNotFoundOrPermissionDenied         ErrorCode = "FILE_NOT_FOUND_OR_PERMISSION_DENIED"
	ErrorCodeFileEmpty                              ErrorCode = "FILE_EMPTY"
	ErrorCodeFileTooLarge                           ErrorCode = "FILE_TOO_LARGE"
	ErrorCodeFileTypeNotAllowed                     ErrorCode = "FILE_TYPE_NOT_ALLOWED"
	ErrorCodeOperationLogNotFound                   ErrorCode = "OPERATION_LOG_NOT_FOUND"
)

func defaultErrorCodeForHTTPStatus(status int) ErrorCode {
	switch status {
	case http.StatusBadRequest:
		return ErrorCodeBadRequest
	case http.StatusUnauthorized:
		return ErrorCodeUnauthorized
	case http.StatusForbidden:
		return ErrorCodeForbidden
	case http.StatusNotFound:
		return ErrorCodeNotFound
	case http.StatusTooManyRequests:
		return ErrorCodeTooManyRequests
	case http.StatusServiceUnavailable:
		return ErrorCodeServiceUnavailable
	default:
		if status >= http.StatusInternalServerError {
			return ErrorCodeInternalServerError
		}
		return ErrorCodeBadRequest
	}
}
