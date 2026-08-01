package audittrail

// UserTarget returns the audited users-table contract shared by auth and identity.
func UserTarget(model any) Target {
	return Target{
		Model:       model,
		Table:       "users",
		TargetType:  "user",
		TenantField: "tenant_id",
		SnapshotFields: []string{
			"id", "tenant_id", "is_platform_admin", "username", "password", "nickname",
			"email", "phone", "avatar", "department_id", "must_change_password",
			"password_changed_at", "totp_secret", "totp_enabled", "status", "created_at", "updated_at",
		},
		FieldMasks: map[string]string{"email": "email", "phone": "phone"},
	}
}

// RoleTarget returns the audited roles-table contract.
func RoleTarget(model any) Target {
	return Target{
		Model:       model,
		Table:       "roles",
		TargetType:  "role",
		TenantField: "tenant_id",
		SnapshotFields: []string{
			"id", "tenant_id", "name", "code", "description", "data_scope", "created_at", "updated_at",
		},
	}
}

// DepartmentTarget returns the audited departments-table contract.
func DepartmentTarget(model any) Target {
	return Target{
		Model:       model,
		Table:       "departments",
		TargetType:  "department",
		TenantField: "tenant_id",
		SnapshotFields: []string{
			"id", "tenant_id", "name", "code", "parent_id", "leader", "leader_user_id",
			"phone", "email", "sort", "status", "created_at", "updated_at",
		},
		FieldMasks: map[string]string{"email": "email", "phone": "phone"},
	}
}

// MenuTarget returns the audited global menus-table contract. Menu rows are
// platform-owned, so their audit records live in the platform tenant rather
// than whichever business tenant the operator happened to be acting as.
func MenuTarget(model any) Target {
	return Target{
		Model:         model,
		Table:         "menus",
		TargetType:    "menu",
		FixedTenantID: 1,
		SnapshotFields: []string{
			"id", "name", "title", "icon", "path", "component", "parent_id", "sort",
			"status", "hidden", "permission", "created_at", "updated_at",
		},
	}
}
