package store

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestClassifyHangup(t *testing.T) {
	cases := []struct {
		cause, want string
	}{
		{"", ""},
		{"NORMAL_CLEARING", "ok"},
		{"unallocated_number", "empty"},
		{"NO_ROUTE_DESTINATION", "empty"},
		{"USER_BUSY", "busy"},
		{"NO_ANSWER", "noanswer"},
		{"NO_USER_RESPONSE", "noanswer"},
		{"SUBSCRIBER_ABSENT", "unavailable"},
		{"RECOVERY_ON_TIMER_EXPIRE", "unavailable"},
		{"CALL_REJECTED", "rejected"},
		{"MANAGER_REQUEST", "local"},
		{"SOMETHING_NEW", "other"},
	}
	for _, tc := range cases {
		if got := ClassifyHangup(tc.cause); got != tc.want {
			t.Errorf("ClassifyHangup(%q)=%q want %q", tc.cause, got, tc.want)
		}
	}
	if HangupClassLabel("empty") != "空号" {
		t.Fatalf("label empty")
	}
}

func TestApplyEventWritesHangupClass(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Call{}); err != nil {
		t.Fatal(err)
	}
	st := NewWithDB(db)
	got, err := st.ApplyEvent(context.Background(), "call.hangup", &Call{
		CallID: "c1", Status: "hangup", HangupCause: "UNALLOCATED_NUMBER",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.HangupClass != "empty" {
		t.Fatalf("class=%q", got.HangupClass)
	}
}
