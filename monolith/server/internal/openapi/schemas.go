package openapi

type routeContract struct {
	RequestSchema  string
	ResponseSchema string
	QueryParams    []Parameter
	NoRequestBody  bool
}

func coreSchemas() map[string]Schema {
	schemas := map[string]Schema{
		"EmptyResponse": objectSchema(map[string]Schema{}, nil),
		"LoginRequest": objectSchema(map[string]Schema{
			"username":     stringSchema(),
			"password":     stringSchema(),
			"captcha_id":   stringSchema(),
			"captcha_code": stringSchema(),
		}, []string{"username", "password", "captcha_id", "captcha_code"}),
		"ConsoleLoginRequest": objectSchema(map[string]Schema{
			"username": stringSchema(),
			"password": stringSchema(),
		}, []string{"username", "password"}),
		"RegisterRequest": objectSchema(map[string]Schema{
			"username": stringSchema(),
			"password": stringSchema(),
			"email":    stringSchema(),
		}, []string{"username", "password", "email"}),
		"RefreshTokenRequest": objectSchema(map[string]Schema{
			"refresh_token": stringSchema(),
		}, []string{"refresh_token"}),
		"OAuthBindRequest": objectSchema(map[string]Schema{
			"provider": enumSchema("github", "wechat"),
			"code":     stringSchema(),
			"state":    stringSchema(),
		}, []string{"provider", "code"}),
		"OAuthUnbindRequest": objectSchema(map[string]Schema{
			"provider": enumSchema("github", "wechat"),
		}, []string{"provider"}),
		"ChangePasswordRequest": objectSchema(map[string]Schema{
			"old_password": stringSchema(),
			"new_password": stringSchema(),
		}, []string{"old_password", "new_password"}),
		"VerifyTOTPLoginRequest": objectSchema(map[string]Schema{
			"challenge_id": stringSchema(),
			"code":         stringSchema(),
		}, []string{"challenge_id", "code"}),
		"TOTPVerifyRequest": objectSchema(map[string]Schema{
			"code":             stringSchema(),
			"current_password": stringSchema(),
		}, []string{"code", "current_password"}),
		"TOTPSetupRequest": objectSchema(map[string]Schema{
			"current_password": stringSchema(),
		}, []string{"current_password"}),
		"TOTPSetupResponse": objectSchema(map[string]Schema{
			"secret":       stringSchema(),
			"otp_auth_url": stringSchema(),
		}, []string{"secret", "otp_auth_url"}),
		"TOTPRecoveryCodesResponse": objectSchema(map[string]Schema{
			"recovery_codes": arraySchema(stringSchema()),
		}, []string{"recovery_codes"}),
		"UpdateProfileRequest": objectSchema(map[string]Schema{
			"nickname": stringSchema(),
			"email":    stringSchema(),
			"phone":    stringSchema(),
			"avatar":   stringSchema(),
		}, nil),
		"SystemSettingItem": objectSchema(map[string]Schema{
			"setting_key": stringSchema(),
			"value_json":  mapSchema(),
			"updated_at":  dateTimeSchema(),
		}, []string{"setting_key", "value_json"}),
		"UpsertSystemSettingRequest": objectSchema(map[string]Schema{
			"value_json": mapSchema(),
		}, []string{"value_json"}),
		"BatchUpsertSystemSettingsRequest": objectSchema(map[string]Schema{
			"settings": arraySchema(refSchema("SystemSettingItem")),
		}, []string{"settings"}),
		"LoginResponse": objectSchema(map[string]Schema{
			"user":              refSchema("UserInfo"),
			"access_token":      stringSchema(),
			"refresh_token":     stringSchema(),
			"requires_totp":     booleanSchema(),
			"totp_challenge_id": stringSchema(),
		}, nil),
		"ConsoleSessionUser": objectSchema(map[string]Schema{
			"id":                   integerSchema(),
			"username":             stringSchema(),
			"display_name":         stringSchema(),
			"role":                 stringSchema(),
			"roles":                arraySchema(stringSchema()),
			"permissions":          arraySchema(stringSchema()),
			"actor_type":           stringSchema(),
			"actor_id":             stringSchema(),
			"nickname":             stringSchema(),
			"avatar":               stringSchema(),
			"must_change_password": booleanSchema(),
		}, []string{"id", "username", "display_name", "role", "roles", "permissions", "actor_type", "actor_id", "must_change_password"}),
		"ConsoleSessionResponse": objectSchema(map[string]Schema{
			"authenticated": booleanSchema(),
			"auth_enabled":  booleanSchema(),
			"user":          refSchema("ConsoleSessionUser"),
			"expires_at":    stringSchema(),
			"ttl_sec":       integerSchema(),
			"access_token":  stringSchema(),
			"refresh_token": stringSchema(),
		}, []string{"authenticated", "auth_enabled", "user", "expires_at", "ttl_sec"}),
		"ConsoleLoginResponse": objectSchema(map[string]Schema{
			"authenticated":     booleanSchema(),
			"auth_enabled":      booleanSchema(),
			"user":              refSchema("ConsoleSessionUser"),
			"expires_at":        stringSchema(),
			"ttl_sec":           integerSchema(),
			"access_token":      stringSchema(),
			"refresh_token":     stringSchema(),
			"requires_totp":     booleanSchema(),
			"totp_challenge_id": stringSchema(),
		}, nil),
		"NotificationMessage": objectSchema(map[string]Schema{
			"id":         stringSchema(),
			"type":       stringSchema(),
			"title":      stringSchema(),
			"content":    stringSchema(),
			"user_id":    integerSchema(),
			"link":       stringSchema(),
			"created_at": dateTimeSchema(),
		}, []string{"id", "type", "title", "content", "created_at"}),
		"NotificationTicketResponse": objectSchema(map[string]Schema{
			"ticket": stringSchema(),
		}, []string{"ticket"}),
		"TokenRefreshResponse": objectSchema(map[string]Schema{
			"access_token":  stringSchema(),
			"refresh_token": stringSchema(),
		}, []string{"access_token"}),
		"RoleInfo": objectSchema(map[string]Schema{
			"id":          integerSchema(),
			"name":        stringSchema(),
			"code":        stringSchema(),
			"description": stringSchema(),
		}, []string{"id", "name", "code"}),
		"PermissionInfo": objectSchema(map[string]Schema{
			"id":        integerSchema(),
			"name":      stringSchema(),
			"code":      stringSchema(),
			"type":      integerSchema(),
			"path":      stringSchema(),
			"method":    stringSchema(),
			"parent_id": integerSchema(),
		}, []string{"id", "name", "code", "type"}),
		"UserInfo": objectSchema(map[string]Schema{
			"id":                   integerSchema(),
			"username":             stringSchema(),
			"email":                stringSchema(),
			"nickname":             stringSchema(),
			"avatar":               stringSchema(),
			"phone":                stringSchema(),
			"department_id":        integerSchema(),
			"status":               integerSchema(),
			"must_change_password": booleanSchema(),
			"password_changed_at":  dateTimeSchema(),
			"totp_enabled":         booleanSchema(),
			"roles":                arraySchema(refSchema("RoleInfo")),
			"permissions":          arraySchema(stringSchema()),
			"created_at":           dateTimeSchema(),
			"updated_at":           dateTimeSchema(),
		}, []string{"id", "username", "status"}),
		"UserListResponse": pageSchema(refSchema("UserInfo")),
		"CreateUserRequest": objectSchema(map[string]Schema{
			"username":      stringSchema(),
			"password":      stringSchema(),
			"nickname":      stringSchema(),
			"email":         stringSchema(),
			"phone":         stringSchema(),
			"department_id": integerSchema(),
			"status":        integerSchema(),
		}, []string{"username", "password"}),
		"UpdateUserRequest": objectSchema(map[string]Schema{
			"nickname":      stringSchema(),
			"email":         stringSchema(),
			"phone":         stringSchema(),
			"avatar":        stringSchema(),
			"department_id": integerSchema(),
			"status":        integerSchema(),
		}, nil),
		"UpdateUserStatusRequest": objectSchema(map[string]Schema{
			"status": integerSchema(),
		}, []string{"status"}),
		"AssignRolesRequest": objectSchema(map[string]Schema{
			"role_ids": arraySchema(integerSchema()),
		}, []string{"role_ids"}),
		"RoleItem": objectSchema(map[string]Schema{
			"id":                        integerSchema(),
			"name":                      stringSchema(),
			"code":                      stringSchema(),
			"description":               stringSchema(),
			"data_scope":                enumSchema("all", "department", "department_tree", "self", "custom", "none"),
			"data_scope_department_ids": arraySchema(integerSchema()),
			"permissions":               arraySchema(refSchema("PermissionInfo")),
			"status":                    integerSchema(),
			"created_at":                dateTimeSchema(),
			"updated_at":                dateTimeSchema(),
		}, []string{"id", "name", "code", "data_scope"}),
		"RoleListResponse": pageSchema(refSchema("RoleItem")),
		"CreateRoleRequest": objectSchema(map[string]Schema{
			"name":                      stringSchema(),
			"code":                      stringSchema(),
			"description":               stringSchema(),
			"data_scope":                enumSchema("all", "department", "department_tree", "self", "custom", "none"),
			"data_scope_department_ids": arraySchema(integerSchema()),
		}, []string{"name", "code"}),
		"UpdateRoleRequest": objectSchema(map[string]Schema{
			"name":                      stringSchema(),
			"code":                      stringSchema(),
			"description":               stringSchema(),
			"data_scope":                enumSchema("all", "department", "department_tree", "self", "custom", "none"),
			"data_scope_department_ids": arraySchema(integerSchema()),
			"status":                    integerSchema(),
		}, nil),
		"AssignPermissionsRequest": objectSchema(map[string]Schema{
			"permission_ids": arraySchema(integerSchema()),
		}, []string{"permission_ids"}),
		"MenuMeta": objectSchema(map[string]Schema{
			"title":      stringSchema(),
			"icon":       stringSchema(),
			"hidden":     booleanSchema(),
			"keepAlive":  booleanSchema(),
			"orderNo":    integerSchema(),
			"single":     booleanSchema(),
			"frameSrc":   stringSchema(),
			"frameBlank": booleanSchema(),
		}, nil),
		"MenuItem": objectSchema(map[string]Schema{
			"id":          integerSchema(),
			"name":        stringSchema(),
			"title":       stringSchema(),
			"path":        stringSchema(),
			"component":   stringSchema(),
			"icon":        stringSchema(),
			"parent_id":   integerSchema(),
			"sort":        integerSchema(),
			"status":      integerSchema(),
			"hidden":      integerSchema(),
			"permission":  stringSchema(),
			"meta":        refSchema("MenuMeta"),
			"created_at":  dateTimeSchema(),
			"updated_at":  dateTimeSchema(),
			"children":    arraySchema(refSchema("MenuItem")),
			"permissions": arraySchema(refSchema("PermissionInfo")),
		}, []string{"id", "name", "title", "path", "sort", "status"}),
		"MenuListResponse": pageSchema(refSchema("MenuItem")),
		"CreateMenuRequest": objectSchema(map[string]Schema{
			"name":           stringSchema(),
			"title":          stringSchema(),
			"path":           stringSchema(),
			"component":      stringSchema(),
			"icon":           stringSchema(),
			"parent_id":      integerSchema(),
			"sort":           integerSchema(),
			"status":         integerSchema(),
			"hidden":         integerSchema(),
			"permission":     stringSchema(),
			"permission_ids": arraySchema(integerSchema()),
			"meta":           mapSchema(),
		}, []string{"name", "title"}),
		"UpdateMenuRequest": objectSchema(map[string]Schema{
			"name":           stringSchema(),
			"title":          stringSchema(),
			"path":           stringSchema(),
			"component":      stringSchema(),
			"icon":           stringSchema(),
			"parent_id":      integerSchema(),
			"sort":           integerSchema(),
			"status":         integerSchema(),
			"hidden":         integerSchema(),
			"permission":     stringSchema(),
			"permission_ids": arraySchema(integerSchema()),
			"meta":           mapSchema(),
		}, nil),
		"FileItem": objectSchema(map[string]Schema{
			"id":               integerSchema(),
			"user_id":          integerSchema(),
			"file_name":        stringSchema(),
			"file_path":        stringSchema(),
			"file_size":        integerSchema(),
			"image_width":      integerSchema(),
			"image_height":     integerSchema(),
			"thumbnail_path":   stringSchema(),
			"thumbnail_url":    stringSchema(),
			"thumbnail_width":  integerSchema(),
			"thumbnail_height": integerSchema(),
			"file_type":        stringSchema(),
			"mime_type":        stringSchema(),
			"extension":        stringSchema(),
			"storage_type":     stringSchema(),
			"url":              stringSchema(),
			"hash":             stringSchema(),
			"created_at":       dateTimeSchema(),
			"updated_at":       dateTimeSchema(),
		}, []string{"id", "file_name", "file_size", "url"}),
		"FileListResponse": pageSchema(refSchema("FileItem")),
		"MultipleUploadResponse": objectSchema(map[string]Schema{
			"uploaded": arraySchema(refSchema("FileItem")),
			"errors":   arraySchema(stringSchema()),
			"success":  integerSchema(),
			"failed":   integerSchema(),
		}, []string{"uploaded", "errors", "success", "failed"}),
		"DeleteFilesRequest": objectSchema(map[string]Schema{
			"ids": arraySchema(integerSchema()),
		}, []string{"ids"}),
		"FileHashCheck": objectSchema(map[string]Schema{
			"exists": booleanSchema(),
			"file":   refSchema("FileItem"),
		}, []string{"exists"}),
		"FileTypeStat": objectSchema(map[string]Schema{
			"count": integerSchema(),
			"size":  integerSchema(),
		}, []string{"count", "size"}),
		"FileStats": objectSchema(map[string]Schema{
			"total":      integerSchema(),
			"total_size": integerSchema(),
			"by_type":    mapOfSchema(refSchema("FileTypeStat")),
		}, nil),
		"ScheduledJob": objectSchema(map[string]Schema{
			"id":              integerSchema(),
			"name":            stringSchema(),
			"group_name":      stringSchema(),
			"cron_expression": stringSchema(),
			"invoke_target":   stringSchema(),
			"description":     stringSchema(),
			"status":          integerSchema(),
			"concurrent":      integerSchema(),
			"last_run_time":   dateTimeSchema(),
			"next_run_time":   dateTimeSchema(),
			"created_at":      dateTimeSchema(),
			"updated_at":      dateTimeSchema(),
		}, []string{"id", "name", "cron_expression", "status"}),
		"JobListResponse": pageSchema(refSchema("ScheduledJob")),
		"SaveJobRequest": objectSchema(map[string]Schema{
			"name":            stringSchema(),
			"group_name":      stringSchema(),
			"cron_expression": stringSchema(),
			"invoke_target":   stringSchema(),
			"description":     stringSchema(),
			"status":          integerSchema(),
			"concurrent":      integerSchema(),
		}, []string{"name", "cron_expression", "invoke_target"}),
		"JobLogCleanupRequest": objectSchema(map[string]Schema{
			"retention_days": integerSchema(),
		}, nil),
		"JobLogCleanupResult": objectSchema(map[string]Schema{
			"retention_days": integerSchema(),
			"cutoff_time":    dateTimeSchema(),
			"deleted_rows":   integerSchema(),
		}, []string{"retention_days", "cutoff_time", "deleted_rows"}),
		"JobAbnormalStatus": objectSchema(map[string]Schema{
			"id":                   integerSchema(),
			"name":                 stringSchema(),
			"group_name":           stringSchema(),
			"status":               integerSchema(),
			"reason":               stringSchema(),
			"last_run_time":        dateTimeSchema(),
			"last_failure_time":    dateTimeSchema(),
			"last_failure_message": stringSchema(),
		}, []string{"id", "name", "status", "reason"}),
		"JobHealthCheck": objectSchema(map[string]Schema{
			"total":         integerSchema(),
			"enabled":       integerSchema(),
			"paused":        integerSchema(),
			"recent_failed": integerSchema(),
			"last_run_time": dateTimeSchema(),
			"abnormal_jobs": arraySchema(refSchema("JobAbnormalStatus")),
			"window_hours":  integerSchema(),
			"checked_at":    dateTimeSchema(),
		}, []string{"total", "enabled", "paused", "recent_failed", "abnormal_jobs", "window_hours", "checked_at"}),
	}

	for name, schema := range consoleModuleSchemas() {
		schemas[name] = schema
	}

	for envelopeName, schemaName := range map[string]string{
		"EmptyEnvelope":                "EmptyResponse",
		"LoginResponseEnvelope":        "LoginResponse",
		"ConsoleLoginResponseEnvelope": "ConsoleLoginResponse",
		"ConsoleSessionEnvelope":       "ConsoleSessionResponse",
		"NotificationMessageEnvelope":  "NotificationMessage",
		"NotificationTicketEnvelope":   "NotificationTicketResponse",
		"TOTPSetupEnvelope":            "TOTPSetupResponse",
		"TOTPRecoveryCodesEnvelope":    "TOTPRecoveryCodesResponse",
		"SystemSettingEnvelope":        "SystemSettingItem",
		"SystemSettingArrayEnvelope":   "SystemSettingItemArray",
		"TokenRefreshEnvelope":         "TokenRefreshResponse",
		"UserEnvelope":                 "UserInfo",
		"UserListEnvelope":             "UserListResponse",
		"RoleEnvelope":                 "RoleItem",
		"RoleListEnvelope":             "RoleListResponse",
		"RoleArrayEnvelope":            "RoleItemArray",
		"MenuEnvelope":                 "MenuItem",
		"MenuListEnvelope":             "MenuListResponse",
		"MenuTreeEnvelope":             "MenuItemArray",
		"FileEnvelope":                 "FileItem",
		"FileListEnvelope":             "FileListResponse",
		"FileHashCheckEnvelope":        "FileHashCheck",
		"MultipleUploadEnvelope":       "MultipleUploadResponse",
		"FileStatsEnvelope":            "FileStats",
		"JobEnvelope":                  "ScheduledJob",
		"JobListEnvelope":              "JobListResponse",
		"JobHealthEnvelope":            "JobHealthCheck",
		"JobLogCleanupResultEnvelope":  "JobLogCleanupResult",
		"DepartmentEnvelope":           "DepartmentItem",
		"DepartmentListEnvelope":       "DepartmentListResponse",
		"DepartmentArrayEnvelope":      "DepartmentItemArray",
		"PermissionEnvelope":           "PermissionItem",
		"PermissionListEnvelope":       "PermissionListResponse",
		"PermissionTreeEnvelope":       "PermissionItemArray",
		"DictTypeEnvelope":             "DictTypeItem",
		"DictTypeListEnvelope":         "DictTypeListResponse",
		"DictTypeArrayEnvelope":        "DictTypeItemArray",
		"DictItemEnvelope":             "DictItem",
		"DictItemListEnvelope":         "DictItemListResponse",
		"DictItemArrayEnvelope":        "DictItemArray",
		"DictDataArrayEnvelope":        "DictDataArray",
		"DictDataMapEnvelope":          "DictDataMap",
		"NoticeEnvelope":               "NoticeItem",
		"NoticeListEnvelope":           "NoticeListResponse",
		"NoticeArrayEnvelope":          "NoticeItemArray",
		"OperationLogEnvelope":         "OperationLogItem",
		"OperationLogListEnvelope":     "OperationLogListResponse",
		"OperationLogStatsEnvelope":    "OperationLogStats",
		"ClearLogsEnvelope":            "ClearLogsResponse",
		"LoginLogEnvelope":             "LoginLogItem",
		"LoginLogListEnvelope":         "LoginLogListResponse",
		"LoginLogArrayEnvelope":        "LoginLogItemArray",
		"LoginStatsEnvelope":           "LoginStats",
		"LoginTrendEnvelope":           "LoginTrendItemArray",
		"OnlineUserListEnvelope":       "OnlineUserListResponse",
		"OnlineUserCountEnvelope":      "OnlineUserCountResponse",
		"ServerInfoEnvelope":           "ServerInfo",
		"MySQLInfoEnvelope":            "MySQLInfo",
		"RedisInfoEnvelope":            "RedisInfo",
	} {
		schemas[envelopeName] = envelopeFor(schemaRefOrArray(schemaName))
	}

	return schemas
}

func consoleModuleSchemas() map[string]Schema {
	return map[string]Schema{
		"DepartmentItem": objectSchema(map[string]Schema{
			"id":         integerSchema(),
			"name":       stringSchema(),
			"code":       stringSchema(),
			"parent_id":  integerSchema(),
			"leader":     stringSchema(),
			"phone":      stringSchema(),
			"email":      stringSchema(),
			"sort":       integerSchema(),
			"status":     integerSchema(),
			"created_at": dateTimeSchema(),
			"updated_at": dateTimeSchema(),
			"children":   arraySchema(refSchema("DepartmentItem")),
		}, []string{"id", "name", "code", "parent_id", "sort", "status"}),
		"DepartmentListResponse": pageSchema(refSchema("DepartmentItem")),
		"CreateDepartmentRequest": objectSchema(map[string]Schema{
			"name":      stringSchema(),
			"code":      stringSchema(),
			"parent_id": integerSchema(),
			"leader":    stringSchema(),
			"phone":     stringSchema(),
			"email":     stringSchema(),
			"sort":      integerSchema(),
			"status":    integerSchema(),
		}, []string{"name", "code"}),
		"UpdateDepartmentRequest": objectSchema(map[string]Schema{
			"name":      stringSchema(),
			"parent_id": integerSchema(),
			"leader":    stringSchema(),
			"phone":     stringSchema(),
			"email":     stringSchema(),
			"sort":      integerSchema(),
			"status":    integerSchema(),
		}, nil),
		"PermissionItem": objectSchema(map[string]Schema{
			"id":          integerSchema(),
			"name":        stringSchema(),
			"code":        stringSchema(),
			"description": stringSchema(),
			"type":        integerSchema(),
			"path":        stringSchema(),
			"method":      stringSchema(),
			"parent_id":   integerSchema(),
			"sort":        integerSchema(),
			"status":      integerSchema(),
			"created_at":  dateTimeSchema(),
			"updated_at":  dateTimeSchema(),
			"children":    arraySchema(refSchema("PermissionItem")),
		}, []string{"id", "name", "code", "type", "parent_id"}),
		"PermissionListResponse": pageSchema(refSchema("PermissionItem")),
		"CreatePermissionRequest": objectSchema(map[string]Schema{
			"name":        stringSchema(),
			"code":        stringSchema(),
			"description": stringSchema(),
			"type":        integerSchema(),
			"path":        stringSchema(),
			"method":      stringSchema(),
			"parent_id":   integerSchema(),
		}, []string{"name", "code", "type"}),
		"UpdatePermissionRequest": objectSchema(map[string]Schema{
			"name":        stringSchema(),
			"description": stringSchema(),
			"path":        stringSchema(),
			"method":      stringSchema(),
			"parent_id":   integerSchema(),
		}, nil),
		"DictTypeItem": objectSchema(map[string]Schema{
			"id":          integerSchema(),
			"name":        stringSchema(),
			"code":        stringSchema(),
			"description": stringSchema(),
			"status":      integerSchema(),
			"created_at":  dateTimeSchema(),
			"updated_at":  dateTimeSchema(),
			"items":       arraySchema(refSchema("DictItem")),
		}, []string{"id", "name", "code", "status"}),
		"DictTypeListResponse": pageSchema(refSchema("DictTypeItem")),
		"CreateDictTypeRequest": objectSchema(map[string]Schema{
			"name":        stringSchema(),
			"code":        stringSchema(),
			"description": stringSchema(),
			"status":      integerSchema(),
		}, []string{"name", "code"}),
		"UpdateDictTypeRequest": objectSchema(map[string]Schema{
			"name":        stringSchema(),
			"description": stringSchema(),
			"status":      integerSchema(),
		}, nil),
		"DictItem": objectSchema(map[string]Schema{
			"id":           integerSchema(),
			"dict_type_id": integerSchema(),
			"label":        stringSchema(),
			"value":        stringSchema(),
			"sort":         integerSchema(),
			"status":       integerSchema(),
			"remark":       stringSchema(),
			"created_at":   dateTimeSchema(),
			"updated_at":   dateTimeSchema(),
		}, []string{"id", "dict_type_id", "label", "value", "sort", "status"}),
		"DictItemListResponse": pageSchema(refSchema("DictItem")),
		"CreateDictItemRequest": objectSchema(map[string]Schema{
			"dict_type_id": integerSchema(),
			"label":        stringSchema(),
			"value":        stringSchema(),
			"sort":         integerSchema(),
			"status":       integerSchema(),
			"remark":       stringSchema(),
		}, []string{"dict_type_id", "label", "value"}),
		"UpdateDictItemRequest": objectSchema(map[string]Schema{
			"label":  stringSchema(),
			"value":  stringSchema(),
			"sort":   integerSchema(),
			"status": integerSchema(),
			"remark": stringSchema(),
		}, nil),
		"DictData": objectSchema(map[string]Schema{
			"id":           integerSchema(),
			"dict_type_id": integerSchema(),
			"label":        stringSchema(),
			"value":        stringSchema(),
			"sort":         integerSchema(),
			"status":       integerSchema(),
			"remark":       stringSchema(),
		}, []string{"label", "value"}),
		"DictDataMap": mapOfSchema(arraySchema(refSchema("DictData"))),
		"NoticeItem": objectSchema(map[string]Schema{
			"id":         integerSchema(),
			"title":      stringSchema(),
			"content":    stringSchema(),
			"type":       integerSchema(),
			"status":     integerSchema(),
			"creator_id": integerSchema(),
			"creator":    stringSchema(),
			"start_time": dateTimeSchema(),
			"end_time":   dateTimeSchema(),
			"created_at": dateTimeSchema(),
			"updated_at": dateTimeSchema(),
		}, []string{"id", "title", "content", "type", "status", "creator_id", "creator"}),
		"NoticeListResponse": pageSchema(refSchema("NoticeItem")),
		"CreateNoticeRequest": objectSchema(map[string]Schema{
			"title":      stringSchema(),
			"content":    stringSchema(),
			"type":       integerSchema(),
			"status":     integerSchema(),
			"start_time": dateTimeSchema(),
			"end_time":   dateTimeSchema(),
		}, []string{"title", "content"}),
		"UpdateNoticeRequest": objectSchema(map[string]Schema{
			"title":      stringSchema(),
			"content":    stringSchema(),
			"type":       integerSchema(),
			"status":     integerSchema(),
			"start_time": dateTimeSchema(),
			"end_time":   dateTimeSchema(),
		}, nil),
		"UpdateNoticeStatusRequest": objectSchema(map[string]Schema{
			"status": integerSchema(),
		}, []string{"status"}),
		"OperationLogItem": objectSchema(map[string]Schema{
			"id":            integerSchema(),
			"user_id":       integerSchema(),
			"username":      stringSchema(),
			"actor_type":    stringSchema(),
			"actor_id":      stringSchema(),
			"request_id":    stringSchema(),
			"module":        stringSchema(),
			"action":        stringSchema(),
			"method":        stringSchema(),
			"path":          stringSchema(),
			"query":         stringSchema(),
			"request_body":  stringSchema(),
			"response_body": stringSchema(),
			"status":        integerSchema(),
			"ip":            stringSchema(),
			"user_agent":    stringSchema(),
			"latency":       integerSchema(),
			"error_msg":     stringSchema(),
			"created_at":    dateTimeSchema(),
		}, []string{"id", "method", "path", "status", "created_at"}),
		"OperationLogListResponse": pageSchema(refSchema("OperationLogItem")),
		"OperationLogStats": objectSchema(map[string]Schema{
			"total":       integerSchema(),
			"success":     integerSchema(),
			"failed":      integerSchema(),
			"today":       integerSchema(),
			"by_module":   mapOfSchema(integerSchema()),
			"by_method":   mapOfSchema(integerSchema()),
			"by_status":   mapOfSchema(integerSchema()),
			"error_count": integerSchema(),
		}, []string{"total", "by_module", "by_method", "error_count"}),
		"ClearLogsRequest": objectSchema(map[string]Schema{
			"days": integerSchema(),
		}, []string{"days"}),
		"ClearLogsResponse": objectSchema(map[string]Schema{
			"deleted_count": integerSchema(),
		}, []string{"deleted_count"}),
		"LoginLogItem": objectSchema(map[string]Schema{
			"id":         integerSchema(),
			"user_id":    integerSchema(),
			"username":   stringSchema(),
			"login_type": integerSchema(),
			"status":     integerSchema(),
			"ip":         stringSchema(),
			"location":   stringSchema(),
			"device":     stringSchema(),
			"os":         stringSchema(),
			"browser":    stringSchema(),
			"user_agent": stringSchema(),
			"message":    stringSchema(),
			"created_at": dateTimeSchema(),
		}, []string{"id", "username", "login_type", "status", "ip", "created_at"}),
		"LoginLogListResponse": pageSchema(refSchema("LoginLogItem")),
		"LoginStats": objectSchema(map[string]Schema{
			"total":       integerSchema(),
			"success":     integerSchema(),
			"failed":      integerSchema(),
			"today":       integerSchema(),
			"today_users": integerSchema(),
			"today_count": integerSchema(),
			"total_users": integerSchema(),
			"by_type":     mapOfSchema(integerSchema()),
			"by_device":   mapOfSchema(integerSchema()),
			"by_browser":  mapOfSchema(integerSchema()),
		}, []string{"total", "success", "failed", "today_users", "by_device", "by_browser"}),
		"LoginTrendItem": objectSchema(map[string]Schema{
			"date":    stringSchema(),
			"count":   integerSchema(),
			"success": integerSchema(),
			"failed":  integerSchema(),
		}, []string{"date", "count", "success", "failed"}),
		"OnlineUserItem": objectSchema(map[string]Schema{
			"user_id":                 integerSchema(),
			"username":                stringSchema(),
			"nickname":                stringSchema(),
			"ip":                      stringSchema(),
			"location":                stringSchema(),
			"browser":                 stringSchema(),
			"os":                      stringSchema(),
			"login_time":              dateTimeSchema(),
			"token_id":                stringSchema(),
			"access_token_expires_at": dateTimeSchema(),
		}, []string{"user_id", "username", "nickname", "ip", "location", "browser", "os", "login_time", "token_id"}),
		"OnlineUserListResponse": objectSchema(map[string]Schema{
			"list":  arraySchema(refSchema("OnlineUserItem")),
			"total": integerSchema(),
		}, []string{"list", "total"}),
		"OnlineUserCountResponse": objectSchema(map[string]Schema{
			"count": integerSchema(),
		}, []string{"count"}),
		"ServerOSInfo": objectSchema(map[string]Schema{
			"go_os":         stringSchema(),
			"arch":          stringSchema(),
			"compiler":      stringSchema(),
			"go_version":    stringSchema(),
			"num_goroutine": integerSchema(),
			"hostname":      stringSchema(),
			"platform":      stringSchema(),
			"boot_time":     stringSchema(),
		}, []string{"go_os", "arch", "compiler", "go_version", "num_goroutine", "hostname", "platform", "boot_time"}),
		"ServerCPUInfo": objectSchema(map[string]Schema{
			"model_name":   stringSchema(),
			"cores":        integerSchema(),
			"used_percent": numberSchema(),
		}, []string{"model_name", "cores", "used_percent"}),
		"ServerMemoryInfo": objectSchema(map[string]Schema{
			"total":        integerSchema(),
			"used":         integerSchema(),
			"free":         integerSchema(),
			"used_percent": numberSchema(),
		}, []string{"total", "used", "free", "used_percent"}),
		"ServerDiskInfo": objectSchema(map[string]Schema{
			"total":        integerSchema(),
			"used":         integerSchema(),
			"free":         integerSchema(),
			"used_percent": numberSchema(),
		}, []string{"total", "used", "free", "used_percent"}),
		"ServerInfo": objectSchema(map[string]Schema{
			"os":      refSchema("ServerOSInfo"),
			"runtime": refSchema("ServerOSInfo"),
			"cpu":     refSchema("ServerCPUInfo"),
			"memory":  refSchema("ServerMemoryInfo"),
			"disk":    refSchema("ServerDiskInfo"),
		}, []string{"os", "cpu", "memory", "disk"}),
		"MySQLDatabaseInfo": objectSchema(map[string]Schema{
			"host":        stringSchema(),
			"port":        integerSchema(),
			"name":        stringSchema(),
			"charset":     stringSchema(),
			"collation":   stringSchema(),
			"table_count": integerSchema(),
			"size_bytes":  integerSchema(),
			"size":        stringSchema(),
		}, []string{"host", "port", "name", "table_count", "size_bytes", "size"}),
		"MySQLConnectionInfo": objectSchema(map[string]Schema{
			"max_open_conns":       integerSchema(),
			"open_conns":           integerSchema(),
			"in_use":               integerSchema(),
			"idle":                 integerSchema(),
			"wait_count":           integerSchema(),
			"wait_duration":        stringSchema(),
			"threads_connected":    integerSchema(),
			"threads_running":      integerSchema(),
			"max_connections":      integerSchema(),
			"max_used_connections": integerSchema(),
			"total_connections":    integerSchema(),
		}, []string{"max_open_conns", "open_conns", "in_use", "idle"}),
		"MySQLQueryInfo": objectSchema(map[string]Schema{
			"questions":    integerSchema(),
			"qps":          numberSchema(),
			"slow_queries": integerSchema(),
			"selects":      integerSchema(),
			"inserts":      integerSchema(),
			"updates":      integerSchema(),
			"deletes":      integerSchema(),
		}, []string{"questions", "qps", "slow_queries"}),
		"MySQLTrafficInfo": objectSchema(map[string]Schema{
			"bytes_received":       integerSchema(),
			"bytes_sent":           integerSchema(),
			"bytes_received_human": stringSchema(),
			"bytes_sent_human":     stringSchema(),
		}, []string{"bytes_received", "bytes_sent", "bytes_received_human", "bytes_sent_human"}),
		"MySQLInfo": objectSchema(map[string]Schema{
			"status":         stringSchema(),
			"version":        stringSchema(),
			"uptime":         stringSchema(),
			"uptime_seconds": integerSchema(),
			"database":       refSchema("MySQLDatabaseInfo"),
			"connections":    refSchema("MySQLConnectionInfo"),
			"queries":        refSchema("MySQLQueryInfo"),
			"traffic":        refSchema("MySQLTrafficInfo"),
		}, []string{"status", "version", "uptime", "uptime_seconds", "connections"}),
		"RedisServerInfo": objectSchema(map[string]Schema{
			"version":        stringSchema(),
			"os":             stringSchema(),
			"mode":           stringSchema(),
			"uptime":         stringSchema(),
			"uptime_seconds": integerSchema(),
			"arch_bits":      stringSchema(),
			"process_id":     integerSchema(),
			"tcp_port":       integerSchema(),
		}, []string{"version", "os", "mode", "uptime", "uptime_seconds"}),
		"RedisMemoryInfo": objectSchema(map[string]Schema{
			"used":          stringSchema(),
			"peak":          stringSchema(),
			"lua":           stringSchema(),
			"fragmentation": stringSchema(),
			"used_bytes":    integerSchema(),
			"peak_bytes":    integerSchema(),
			"rss":           stringSchema(),
			"maxmemory":     stringSchema(),
			"mem_allocator": stringSchema(),
			"dataset":       stringSchema(),
			"overhead":      stringSchema(),
		}, []string{"used", "peak", "lua", "fragmentation", "used_bytes", "peak_bytes"}),
		"RedisStatsInfo": objectSchema(map[string]Schema{
			"connections":                stringSchema(),
			"ops":                        stringSchema(),
			"keys":                       integerSchema(),
			"hit_rate":                   stringSchema(),
			"total_connections_received": integerSchema(),
			"total_commands_processed":   integerSchema(),
			"keyspace_hits":              integerSchema(),
			"keyspace_misses":            integerSchema(),
			"expired_keys":               integerSchema(),
			"evicted_keys":               integerSchema(),
		}, []string{"connections", "ops", "keys", "hit_rate"}),
		"RedisClientsInfo": objectSchema(map[string]Schema{
			"connected": integerSchema(),
			"blocked":   integerSchema(),
			"tracking":  integerSchema(),
		}, []string{"connected", "blocked", "tracking"}),
		"RedisPoolInfo": objectSchema(map[string]Schema{
			"hits":        integerSchema(),
			"misses":      integerSchema(),
			"timeouts":    integerSchema(),
			"total_conns": integerSchema(),
			"idle_conns":  integerSchema(),
			"stale_conns": integerSchema(),
		}, []string{"hits", "misses", "timeouts", "total_conns", "idle_conns", "stale_conns"}),
		"RedisKeyspaceInfo": objectSchema(map[string]Schema{
			"dbsize": integerSchema(),
			"dbs":    mapOfSchema(mapOfSchema(integerSchema())),
		}, []string{"dbsize", "dbs"}),
		"RedisInfo": objectSchema(map[string]Schema{
			"status":   stringSchema(),
			"server":   refSchema("RedisServerInfo"),
			"memory":   refSchema("RedisMemoryInfo"),
			"stats":    refSchema("RedisStatsInfo"),
			"clients":  refSchema("RedisClientsInfo"),
			"pool":     refSchema("RedisPoolInfo"),
			"keyspace": refSchema("RedisKeyspaceInfo"),
		}, []string{"status", "server", "memory", "stats", "keyspace"}),
	}
}

func contractFor(method, path string) (routeContract, bool) {
	contracts := map[string]routeContract{
		"POST /api/v1/login":                     {RequestSchema: "LoginRequest", ResponseSchema: "LoginResponseEnvelope"},
		"POST /api/v1/login/2fa/verify":          {RequestSchema: "VerifyTOTPLoginRequest", ResponseSchema: "LoginResponseEnvelope"},
		"POST /api/v1/auth/login":                {RequestSchema: "ConsoleLoginRequest", ResponseSchema: "ConsoleLoginResponseEnvelope"},
		"POST /api/v1/auth/login/2fa/verify":     {RequestSchema: "VerifyTOTPLoginRequest", ResponseSchema: "ConsoleSessionEnvelope"},
		"GET /api/v1/ws/notifications":           {ResponseSchema: "NotificationMessageEnvelope", QueryParams: []Parameter{queryParam("ticket", stringSchema())}},
		"POST /api/v1/ws/notifications/ticket":   {ResponseSchema: "NotificationTicketEnvelope", NoRequestBody: true},
		"POST /api/v1/register":                  {RequestSchema: "RegisterRequest", ResponseSchema: "UserEnvelope"},
		"POST /api/v1/refresh":                   {RequestSchema: "RefreshTokenRequest", ResponseSchema: "TokenRefreshEnvelope"},
		"POST /api/v1/logout":                    {RequestSchema: "RefreshTokenRequest", ResponseSchema: "EmptyEnvelope"},
		"POST /api/v1/oauth/bind":                {RequestSchema: "OAuthBindRequest", ResponseSchema: "EmptyEnvelope"},
		"POST /api/v1/oauth/unbind":              {RequestSchema: "OAuthUnbindRequest", ResponseSchema: "EmptyEnvelope"},
		"GET /api/v1/user/me":                    {ResponseSchema: "UserEnvelope"},
		"PUT /api/v1/user/profile":               {RequestSchema: "UpdateProfileRequest", ResponseSchema: "UserEnvelope"},
		"PUT /api/v1/user/password":              {RequestSchema: "ChangePasswordRequest", ResponseSchema: "EmptyEnvelope"},
		"POST /api/v1/user/2fa/setup":            {RequestSchema: "TOTPSetupRequest", ResponseSchema: "TOTPSetupEnvelope"},
		"POST /api/v1/user/2fa/enable":           {RequestSchema: "TOTPVerifyRequest", ResponseSchema: "TOTPRecoveryCodesEnvelope"},
		"POST /api/v1/user/2fa/disable":          {RequestSchema: "TOTPVerifyRequest", ResponseSchema: "EmptyEnvelope"},
		"POST /api/v1/user/2fa/recovery-codes":   {RequestSchema: "TOTPVerifyRequest", ResponseSchema: "TOTPRecoveryCodesEnvelope"},
		"GET /api/v1/user/menus":                 {ResponseSchema: "MenuTreeEnvelope"},
		"GET /api/v1/users":                      {ResponseSchema: "UserListEnvelope", QueryParams: pagingQueryParams("keyword", "status")},
		"POST /api/v1/users":                     {RequestSchema: "CreateUserRequest", ResponseSchema: "UserEnvelope"},
		"GET /api/v1/users/{id}":                 {ResponseSchema: "UserEnvelope"},
		"PUT /api/v1/users/{id}":                 {RequestSchema: "UpdateUserRequest", ResponseSchema: "UserEnvelope"},
		"DELETE /api/v1/users/{id}":              {ResponseSchema: "EmptyEnvelope"},
		"PUT /api/v1/users/{id}/status":          {RequestSchema: "UpdateUserStatusRequest", ResponseSchema: "EmptyEnvelope"},
		"POST /api/v1/users/{id}/roles":          {RequestSchema: "AssignRolesRequest", ResponseSchema: "EmptyEnvelope"},
		"GET /api/v1/roles":                      {ResponseSchema: "RoleListEnvelope", QueryParams: pagingQueryParams("keyword")},
		"GET /api/v1/roles/all":                  {ResponseSchema: "RoleArrayEnvelope"},
		"GET /api/v1/roles/{id}":                 {ResponseSchema: "RoleEnvelope"},
		"POST /api/v1/roles":                     {RequestSchema: "CreateRoleRequest", ResponseSchema: "RoleEnvelope"},
		"PUT /api/v1/roles/{id}":                 {RequestSchema: "UpdateRoleRequest", ResponseSchema: "RoleEnvelope"},
		"DELETE /api/v1/roles/{id}":              {ResponseSchema: "EmptyEnvelope"},
		"POST /api/v1/roles/{id}/permissions":    {RequestSchema: "AssignPermissionsRequest", ResponseSchema: "EmptyEnvelope"},
		"GET /api/v1/menus":                      {ResponseSchema: "MenuListEnvelope", QueryParams: pagingQueryParams("keyword", "status")},
		"GET /api/v1/menus/tree":                 {ResponseSchema: "MenuTreeEnvelope", QueryParams: []Parameter{queryParam("status", integerSchema())}},
		"GET /api/v1/menus/{id}":                 {ResponseSchema: "MenuEnvelope"},
		"POST /api/v1/menus":                     {RequestSchema: "CreateMenuRequest", ResponseSchema: "MenuEnvelope"},
		"PUT /api/v1/menus/{id}":                 {RequestSchema: "UpdateMenuRequest", ResponseSchema: "MenuEnvelope"},
		"DELETE /api/v1/menus/{id}":              {ResponseSchema: "EmptyEnvelope"},
		"GET /api/v1/departments":                {ResponseSchema: "DepartmentListEnvelope", QueryParams: pagingQueryParams("keyword", "status")},
		"GET /api/v1/departments/tree":           {ResponseSchema: "DepartmentArrayEnvelope", QueryParams: []Parameter{queryParam("status", integerSchema())}},
		"GET /api/v1/departments/all":            {ResponseSchema: "DepartmentArrayEnvelope", QueryParams: []Parameter{queryParam("status", integerSchema())}},
		"GET /api/v1/departments/{id}":           {ResponseSchema: "DepartmentEnvelope"},
		"POST /api/v1/departments":               {RequestSchema: "CreateDepartmentRequest", ResponseSchema: "DepartmentEnvelope"},
		"PUT /api/v1/departments/{id}":           {RequestSchema: "UpdateDepartmentRequest", ResponseSchema: "DepartmentEnvelope"},
		"DELETE /api/v1/departments/{id}":        {ResponseSchema: "EmptyEnvelope"},
		"GET /api/v1/permissions":                {ResponseSchema: "PermissionListEnvelope", QueryParams: pagingQueryParams("keyword", "type")},
		"GET /api/v1/permissions/tree":           {ResponseSchema: "PermissionTreeEnvelope"},
		"GET /api/v1/permissions/{id}":           {ResponseSchema: "PermissionEnvelope"},
		"POST /api/v1/permissions":               {RequestSchema: "CreatePermissionRequest", ResponseSchema: "PermissionEnvelope"},
		"PUT /api/v1/permissions/{id}":           {RequestSchema: "UpdatePermissionRequest", ResponseSchema: "PermissionEnvelope"},
		"DELETE /api/v1/permissions/{id}":        {ResponseSchema: "EmptyEnvelope"},
		"GET /api/v1/dict-types":                 {ResponseSchema: "DictTypeListEnvelope", QueryParams: pagingQueryParams("keyword", "status")},
		"GET /api/v1/dict-types/all":             {ResponseSchema: "DictTypeArrayEnvelope"},
		"GET /api/v1/dict-types/{id}":            {ResponseSchema: "DictTypeEnvelope"},
		"GET /api/v1/dict-types/{id}/items":      {ResponseSchema: "DictItemArrayEnvelope"},
		"POST /api/v1/dict-types":                {RequestSchema: "CreateDictTypeRequest", ResponseSchema: "DictTypeEnvelope"},
		"PUT /api/v1/dict-types/{id}":            {RequestSchema: "UpdateDictTypeRequest", ResponseSchema: "DictTypeEnvelope"},
		"DELETE /api/v1/dict-types/{id}":         {ResponseSchema: "EmptyEnvelope"},
		"GET /api/v1/dict-items":                 {ResponseSchema: "DictItemListEnvelope", QueryParams: pagingQueryParams("type_id", "keyword", "status")},
		"GET /api/v1/dict-items/{id}":            {ResponseSchema: "DictItemEnvelope"},
		"POST /api/v1/dict-items":                {RequestSchema: "CreateDictItemRequest", ResponseSchema: "DictItemEnvelope"},
		"PUT /api/v1/dict-items/{id}":            {RequestSchema: "UpdateDictItemRequest", ResponseSchema: "DictItemEnvelope"},
		"DELETE /api/v1/dict-items/{id}":         {ResponseSchema: "EmptyEnvelope"},
		"GET /api/v1/dicts/{code}":               {ResponseSchema: "DictDataArrayEnvelope"},
		"GET /api/v1/dicts":                      {ResponseSchema: "DictDataMapEnvelope", QueryParams: []Parameter{queryParam("codes", stringSchema())}},
		"GET /api/v1/dicts/all":                  {ResponseSchema: "DictDataMapEnvelope"},
		"GET /api/v1/system-settings":            {ResponseSchema: "SystemSettingArrayEnvelope", QueryParams: []Parameter{queryParam("group", stringSchema())}},
		"POST /api/v1/system-settings/batch":     {RequestSchema: "BatchUpsertSystemSettingsRequest", ResponseSchema: "SystemSettingArrayEnvelope"},
		"GET /api/v1/system-settings/{key}":      {ResponseSchema: "SystemSettingEnvelope"},
		"PUT /api/v1/system-settings/{key}":      {RequestSchema: "UpsertSystemSettingRequest", ResponseSchema: "SystemSettingEnvelope"},
		"DELETE /api/v1/system-settings/{key}":   {ResponseSchema: "EmptyEnvelope"},
		"GET /api/v1/notices":                    {ResponseSchema: "NoticeListEnvelope", QueryParams: pagingQueryParams("type", "status", "keyword")},
		"GET /api/v1/notices/active":             {ResponseSchema: "NoticeArrayEnvelope", QueryParams: []Parameter{queryParam("type", integerSchema())}},
		"GET /api/v1/notices/{id}":               {ResponseSchema: "NoticeEnvelope"},
		"POST /api/v1/notices":                   {RequestSchema: "CreateNoticeRequest", ResponseSchema: "NoticeEnvelope"},
		"PUT /api/v1/notices/{id}":               {RequestSchema: "UpdateNoticeRequest", ResponseSchema: "NoticeEnvelope"},
		"DELETE /api/v1/notices/{id}":            {ResponseSchema: "EmptyEnvelope"},
		"PUT /api/v1/notices/{id}/status":        {RequestSchema: "UpdateNoticeStatusRequest", ResponseSchema: "EmptyEnvelope"},
		"GET /api/v1/operation-logs":             {ResponseSchema: "OperationLogListEnvelope", QueryParams: pagingQueryParams("user_id", "username", "actor_type", "actor_id", "request_id", "method", "path", "module", "action", "status", "start_time", "end_time")},
		"GET /api/v1/operation-logs/stats":       {ResponseSchema: "OperationLogStatsEnvelope", QueryParams: []Parameter{queryParam("start_time", stringSchema()), queryParam("end_time", stringSchema())}},
		"GET /api/v1/operation-logs/{id}":        {ResponseSchema: "OperationLogEnvelope"},
		"DELETE /api/v1/operation-logs/clear":    {RequestSchema: "ClearLogsRequest", ResponseSchema: "ClearLogsEnvelope"},
		"GET /api/v1/operation-logs/export":      {ResponseSchema: "OperationLogListEnvelope", QueryParams: pagingQueryParams("user_id", "username", "actor_type", "actor_id", "request_id", "method", "path", "module", "action", "status", "start_time", "end_time")},
		"GET /api/v1/login-logs":                 {ResponseSchema: "LoginLogListEnvelope", QueryParams: pagingQueryParams("user_id", "username", "ip", "status", "login_type", "start_time", "end_time")},
		"GET /api/v1/login-logs/my":              {ResponseSchema: "LoginLogListEnvelope", QueryParams: pagingQueryParams("username", "ip", "status", "login_type", "start_time", "end_time")},
		"GET /api/v1/login-logs/stats":           {ResponseSchema: "LoginStatsEnvelope", QueryParams: []Parameter{queryParam("start_time", stringSchema()), queryParam("end_time", stringSchema())}},
		"GET /api/v1/login-logs/trend":           {ResponseSchema: "LoginTrendEnvelope", QueryParams: []Parameter{queryParam("days", integerSchema())}},
		"GET /api/v1/login-logs/last":            {ResponseSchema: "LoginLogEnvelope"},
		"GET /api/v1/login-logs/user/{user_id}":  {ResponseSchema: "LoginLogListEnvelope", QueryParams: pagingQueryParams("username", "ip", "status", "login_type", "start_time", "end_time")},
		"DELETE /api/v1/login-logs/clear":        {RequestSchema: "ClearLogsRequest", ResponseSchema: "ClearLogsEnvelope"},
		"GET /api/v1/online-users":               {ResponseSchema: "OnlineUserListEnvelope"},
		"GET /api/v1/online-users/count":         {ResponseSchema: "OnlineUserCountEnvelope"},
		"DELETE /api/v1/online-users/{token_id}": {ResponseSchema: "EmptyEnvelope"},
		"GET /api/v1/monitor/server":             {ResponseSchema: "ServerInfoEnvelope"},
		"GET /api/v1/monitor/mysql":              {ResponseSchema: "MySQLInfoEnvelope"},
		"GET /api/v1/monitor/redis":              {ResponseSchema: "RedisInfoEnvelope"},
		"GET /api/v1/files":                      {ResponseSchema: "FileListEnvelope", QueryParams: pagingQueryParams("keyword", "file_type", "storage_type")},
		"GET /api/v1/files/my":                   {ResponseSchema: "FileListEnvelope", QueryParams: pagingQueryParams("keyword", "file_type", "storage_type")},
		"GET /api/v1/files/stats":                {ResponseSchema: "FileStatsEnvelope", QueryParams: []Parameter{queryParam("user_id", integerSchema())}},
		"GET /api/v1/files/hash/check":           {ResponseSchema: "FileHashCheckEnvelope", QueryParams: []Parameter{requiredQueryParam("hash", stringSchema())}},
		"GET /api/v1/files/{id}":                 {ResponseSchema: "FileEnvelope"},
		"POST /api/v1/files/upload":              {ResponseSchema: "FileEnvelope"},
		"POST /api/v1/files/upload/multiple":     {ResponseSchema: "MultipleUploadEnvelope"},
		"DELETE /api/v1/files/batch":             {RequestSchema: "DeleteFilesRequest", ResponseSchema: "EmptyEnvelope"},
		"GET /api/v1/monitor/jobs":               {ResponseSchema: "JobListEnvelope", QueryParams: pagingQueryParams("name", "status")},
		"GET /api/v1/monitor/jobs/health":        {ResponseSchema: "JobHealthEnvelope", QueryParams: []Parameter{queryParam("window_hours", integerSchema())}},
		"POST /api/v1/monitor/jobs":              {RequestSchema: "SaveJobRequest", ResponseSchema: "JobEnvelope"},
		"PUT /api/v1/monitor/jobs/{id}":          {RequestSchema: "SaveJobRequest", ResponseSchema: "JobEnvelope"},
		"DELETE /api/v1/monitor/jobs/{id}":       {ResponseSchema: "EmptyEnvelope"},
		"POST /api/v1/monitor/jobs/{id}/start":   {ResponseSchema: "EmptyEnvelope", NoRequestBody: true},
		"POST /api/v1/monitor/jobs/{id}/stop":    {ResponseSchema: "EmptyEnvelope", NoRequestBody: true},
		"POST /api/v1/monitor/jobs/{id}/run":     {ResponseSchema: "EmptyEnvelope", NoRequestBody: true},
		"POST /api/v1/monitor/job-logs/cleanup":  {RequestSchema: "JobLogCleanupRequest", ResponseSchema: "JobLogCleanupResultEnvelope"},
	}
	contract, ok := contracts[method+" "+path]
	return contract, ok
}

func envelopeFor(data Schema) Schema {
	return objectSchema(map[string]Schema{
		"code":    integerSchema(),
		"message": stringSchema(),
		"data":    data,
	}, []string{"code", "message", "data"})
}

func schemaRefOrArray(name string) Schema {
	switch name {
	case "RoleItemArray":
		return arraySchema(refSchema("RoleItem"))
	case "MenuItemArray":
		return arraySchema(refSchema("MenuItem"))
	case "DepartmentItemArray":
		return arraySchema(refSchema("DepartmentItem"))
	case "PermissionItemArray":
		return arraySchema(refSchema("PermissionItem"))
	case "DictTypeItemArray":
		return arraySchema(refSchema("DictTypeItem"))
	case "DictItemArray":
		return arraySchema(refSchema("DictItem"))
	case "DictDataArray":
		return arraySchema(refSchema("DictData"))
	case "NoticeItemArray":
		return arraySchema(refSchema("NoticeItem"))
	case "SystemSettingItemArray":
		return arraySchema(refSchema("SystemSettingItem"))
	case "LoginLogItemArray":
		return arraySchema(refSchema("LoginLogItem"))
	case "LoginTrendItemArray":
		return arraySchema(refSchema("LoginTrendItem"))
	default:
		return refSchema(name)
	}
}

func objectSchema(properties map[string]Schema, required []string) Schema {
	return Schema{Type: "object", Properties: properties, Required: required}
}

func mapSchema() Schema {
	return Schema{Type: "object", AdditionalProperties: true}
}

func mapOfSchema(value Schema) Schema {
	return Schema{Type: "object", AdditionalProperties: value}
}

func pageSchema(item Schema) Schema {
	return objectSchema(map[string]Schema{
		"list":      arraySchema(item),
		"total":     integerSchema(),
		"page":      integerSchema(),
		"page_size": integerSchema(),
	}, []string{"list", "total", "page", "page_size"})
}

func refSchema(name string) Schema {
	return Schema{Ref: "#/components/schemas/" + name}
}

func stringSchema() Schema {
	return Schema{Type: "string"}
}

func dateTimeSchema() Schema {
	return Schema{Type: "string", Format: "date-time"}
}

func integerSchema() Schema {
	return Schema{Type: "integer", Format: "int64"}
}

func numberSchema() Schema {
	return Schema{Type: "number", Format: "double"}
}

func booleanSchema() Schema {
	return Schema{Type: "boolean"}
}

func enumSchema(values ...string) Schema {
	return Schema{Type: "string", Enum: values}
}

func arraySchema(item Schema) Schema {
	return Schema{Type: "array", Items: &item}
}

func queryParam(name string, schema Schema) Parameter {
	return Parameter{Name: name, In: "query", Required: false, Schema: schema}
}

func requiredQueryParam(name string, schema Schema) Parameter {
	return Parameter{Name: name, In: "query", Required: true, Schema: schema}
}

func pagingQueryParams(names ...string) []Parameter {
	params := []Parameter{
		queryParam("page", integerSchema()),
		queryParam("page_size", integerSchema()),
	}
	for _, name := range names {
		schema := stringSchema()
		if name == "status" || name == "type" || name == "type_id" || name == "user_id" || name == "login_type" {
			schema = integerSchema()
		}
		params = append(params, queryParam(name, schema))
	}
	return params
}
