package model

import "testing"

func TestAuditLogTableName(t *testing.T) {
	if got := (AuditLog{}).TableName(); got != "wm_audit_log" {
		t.Fatalf("table name = %q, want %q", got, "wm_audit_log")
	}
}
