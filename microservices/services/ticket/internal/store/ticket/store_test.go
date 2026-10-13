package store

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/ticket/internal/model"
	"gorm.io/gorm"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	st, err := NewWithDB(db)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	return st
}

func TestCreateAndStatusMachine(t *testing.T) {
	st := openTest(t)
	tk, err := st.CreateTicket(context.Background(), 1, CreateTicketInput{
		Title: "访客报障", Description: "无法登录", CreatorUserID: 9,
		Source: model.SourceIM, ConversationPublicID: "conv-abc",
		ContactName: "张三", ContactPhone: "13800138000",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tk.TicketNo != "TK-1" {
		t.Fatalf("ticket_no = %q", tk.TicketNo)
	}
	if tk.Status != model.StatusOpen {
		t.Fatalf("status = %s", tk.Status)
	}
	if tk.AssigneeUserID != 9 {
		t.Fatalf("default assignee = %d", tk.AssigneeUserID)
	}
	// M1.5：未指定 due_at 时按 normal SLA 自动填
	if tk.DueAt == nil {
		t.Fatal("expected default SLA due_at")
	}

	// open → in_progress
	tk, err = st.ChangeStatus(context.Background(), tk.ID, 1, 9, model.StatusInProgress, "开始处理")
	if err != nil {
		t.Fatalf("to in_progress: %v", err)
	}
	// in_progress → resolved
	tk, err = st.ChangeStatus(context.Background(), tk.ID, 1, 9, model.StatusResolved, "已修好")
	if err != nil {
		t.Fatalf("to resolved: %v", err)
	}
	if tk.ResolvedAt == nil {
		t.Fatal("resolved_at nil")
	}
	// resolved → closed
	tk, err = st.ChangeStatus(context.Background(), tk.ID, 1, 9, model.StatusClosed, "")
	if err != nil {
		t.Fatalf("to closed: %v", err)
	}
	// closed → open (reopen)
	tk, err = st.ChangeStatus(context.Background(), tk.ID, 1, 9, model.StatusOpen, "复现")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	// illegal: open → open is ok (noop), open cannot jump invalid
	if _, err := st.ChangeStatus(context.Background(), tk.ID, 1, 9, "bogus", ""); err == nil {
		t.Fatal("expected invalid status")
	}

	events, err := st.ListEvents(tk.ID, 1)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) < 4 {
		t.Fatalf("events len = %d", len(events))
	}
}

func TestAssignAutoProgress(t *testing.T) {
	st := openTest(t)
	tk, err := st.CreateTicket(context.Background(), 1, CreateTicketInput{
		Title: "指派测试", CreatorUserID: 1, AssigneeUserID: 0,
	})
	// CreateTicket forces assignee to creator when 0
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// force unassigned open state for assign path
	_ = st.db.Model(tk).Updates(map[string]any{"assignee_user_id": 0, "status": model.StatusOpen}).Error
	tk, err = st.Assign(context.Background(), tk.ID, 1, 1, 42)
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if tk.AssigneeUserID != 42 {
		t.Fatalf("assignee = %d", tk.AssigneeUserID)
	}
	if tk.Status != model.StatusInProgress {
		t.Fatalf("status after assign = %s", tk.Status)
	}
}

func TestCommentAndListFilter(t *testing.T) {
	st := openTest(t)
	due := time.Now().Add(-time.Hour)
	_, err := st.CreateTicket(context.Background(), 1, CreateTicketInput{
		Title: "超时单", CreatorUserID: 1, DueAt: &due, Priority: model.PriorityHigh,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	tk2, err := st.CreateTicket(context.Background(), 1, CreateTicketInput{
		Title: "普通单", CreatorUserID: 2, Source: model.SourceCall,
	})
	if err != nil {
		t.Fatalf("create2: %v", err)
	}
	if _, err := st.AddComment(context.Background(), tk2.ID, 1, 2, "跟进一下"); err != nil {
		t.Fatalf("comment: %v", err)
	}

	list, total, err := st.ListTickets(1, TicketFilter{OverdueOnly: true, Page: Page{Page: 1, PageSize: 20}})
	if err != nil {
		t.Fatalf("list overdue: %v", err)
	}
	if total != 1 || len(list) != 1 || list[0].Title != "超时单" {
		t.Fatalf("overdue list total=%d list=%v", total, list)
	}

	// 指派给用户 2 的超时单
	over, _ := st.CreateTicket(context.Background(), 1, CreateTicketInput{
		Title: "我的超时", CreatorUserID: 2, AssigneeUserID: 2, DueAt: &due, Priority: model.PriorityUrgent,
	})
	_ = over
	stt, err := st.Stats(1, 2)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stt.Open < 1 || stt.Overdue < 1 {
		t.Fatalf("stats = %+v", stt)
	}
	if stt.MineOpen < 1 || stt.MineOverdue < 1 {
		t.Fatalf("mine stats = %+v", stt)
	}
}

func TestDefaultSLAByPriority(t *testing.T) {
	if DefaultSLAHours(model.PriorityUrgent) != 4 {
		t.Fatalf("urgent hours")
	}
	if DefaultSLAHours(model.PriorityNormal) != 72 {
		t.Fatalf("normal hours")
	}
	d := DefaultDueAt(model.PriorityHigh, time.Now())
	if d == nil {
		t.Fatal("high due nil")
	}
}

func TestTenantIsolation(t *testing.T) {
	st := openTest(t)
	a, _ := st.CreateTicket(context.Background(), 1, CreateTicketInput{Title: "t1", CreatorUserID: 1})
	_, _ = st.CreateTicket(context.Background(), 2, CreateTicketInput{Title: "t2", CreatorUserID: 1})
	if _, err := st.GetTicket(a.ID, 2); err == nil {
		t.Fatal("expected not found across tenant")
	}
	list, total, err := st.ListTickets(1, TicketFilter{Page: Page{Page: 1, PageSize: 10}})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("tenant1 list total=%d err=%v", total, err)
	}
}
