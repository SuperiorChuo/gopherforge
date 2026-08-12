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

// EdgeTLSCertificateTarget returns the audited platform-owned certificate
// lifecycle contract. Secret material, encrypted envelopes and provider error
// strings are deliberately absent from the snapshot whitelist.
func EdgeTLSCertificateTarget(model any) Target {
	return Target{
		Model:         model,
		Table:         "edge_tls_certificates",
		TargetType:    "edge_tls_certificate",
		FixedTenantID: 1,
		SnapshotFields: []string{
			"id", "domain", "email", "status", "provider", "is_staging",
			"not_before", "not_after", "cert_fingerprint_sha256",
			"deployment_mode", "deployment_provider", "auto_renew_enabled",
			"renew_at", "last_renewal_at", "deployment_status",
			"deployed_fingerprint_sha256", "deployed_at", "serving_status",
			"serving_fingerprint_sha256", "serving_not_after", "serving_issuer",
			"serving_checked_at", "created_at", "updated_at",
		},
	}
}
