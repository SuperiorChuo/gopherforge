package excel

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
)

// 导出 → 读回 roundtrip：表头/数据/下载头齐备。
func TestExportReadRoundtrip(t *testing.T) {
	s, err := NewSheet("用户", []string{"用户名", "昵称", "状态"}, []float64{20, 20, 10})
	if err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	if err := s.AppendRow("alice", "爱丽丝", "启用"); err != nil {
		t.Fatalf("AppendRow: %v", err)
	}
	if err := s.AppendRow("bob", "", 1); err != nil {
		t.Fatalf("AppendRow2: %v", err)
	}
	rec := httptest.NewRecorder()
	if err := s.WriteHTTP(rec, "users.xlsx"); err != nil {
		t.Fatalf("WriteHTTP: %v", err)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "spreadsheetml") {
		t.Fatalf("Content-Type 不对: %s", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "users.xlsx") {
		t.Fatalf("Content-Disposition 不对: %s", cd)
	}

	rows, err := ReadFirstSheet(bytes.NewReader(rec.Body.Bytes()), 100)
	if err != nil {
		t.Fatalf("ReadFirstSheet: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("应 3 行（含表头），got %d: %v", len(rows), rows)
	}
	if rows[0][0] != "用户名" || rows[1][0] != "alice" || rows[1][1] != "爱丽丝" {
		t.Fatalf("内容不符: %v", rows)
	}
	if Cell(rows[2], 1) != "" || Cell(rows[2], 99) != "" {
		t.Fatalf("Cell 防御取值不符: %v", rows[2])
	}
}

// 行数超限报错而非截断。
func TestReadRowLimit(t *testing.T) {
	s, err := NewSheet("", []string{"A"}, nil)
	if err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := s.AppendRow(i); err != nil {
			t.Fatalf("AppendRow: %v", err)
		}
	}
	rec := httptest.NewRecorder()
	if err := s.WriteHTTP(rec, ""); err != nil {
		t.Fatalf("WriteHTTP: %v", err)
	}
	if _, err := ReadFirstSheet(bytes.NewReader(rec.Body.Bytes()), 3); err == nil {
		t.Fatal("超限应报错")
	}
	if _, err := ReadFirstSheet(bytes.NewReader(rec.Body.Bytes()), 5); err != nil {
		t.Fatalf("未超限不应报错: %v", err)
	}
}
