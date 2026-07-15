package model

import "testing"

func TestAuditLogTableName(t *testing.T) {
	if got := (AuditLog{}).TableName(); got != "audit_logs" {
		t.Fatalf("table name = %q, want %q", got, "audit_logs")
	}
}
