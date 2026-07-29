package store

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/go-admin-kit/services/bpm/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewWithDB(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Stats 的平均时长已从"拉 1000/2000 行进 Go 端累加"改为 SQL 侧 AVG。
// 本测试固定数据，断言 SQL 聚合结果与手工计算的期望一致，并覆盖租户隔离。
func TestStatsAvgHoursPushedDownMatchesManual(t *testing.T) {
	s := newTestStore(t)
	const tenant, other uint64 = 1, 2

	if err := s.db.Create(&[]model.ProcessDefinition{
		{TenantID: tenant, Key: "leave", Version: 1, Name: "请假-旧", Status: "archived"},
		{TenantID: tenant, Key: "leave", Version: 2, Name: "请假", Status: "active"},
		{TenantID: tenant, Key: "expense", Version: 1, Name: "报销", Status: "active"},
	}).Error; err != nil {
		t.Fatal(err)
	}

	base := time.Now().Add(-72 * time.Hour)
	fin := func(h float64) *time.Time {
		v := base.Add(time.Duration(h * float64(time.Hour)))
		return &v
	}
	// leave: 已结束 2 条（2h、4h → avg 3.0）+ 运行中 1 条（不计入均值）
	// expense: 已结束 1 条（1.5h → avg 1.5）
	// other 租户: 100h，绝不能污染 tenant 的均值
	if err := s.db.Create(&[]model.ProcessInstance{
		{TenantID: tenant, DefinitionID: 2, DefinitionKey: "leave", Title: "a", BizType: "t", BizID: "1",
			Status: model.InstApproved, InitiatorID: 1, CreatedAt: base, FinishedAt: fin(2)},
		{TenantID: tenant, DefinitionID: 2, DefinitionKey: "leave", Title: "b", BizType: "t", BizID: "2",
			Status: model.InstRejected, InitiatorID: 1, CreatedAt: base, FinishedAt: fin(4)},
		{TenantID: tenant, DefinitionID: 2, DefinitionKey: "leave", Title: "c", BizType: "t", BizID: "3",
			Status: model.InstRunning, InitiatorID: 1, CreatedAt: base},
		{TenantID: tenant, DefinitionID: 3, DefinitionKey: "expense", Title: "d", BizType: "t", BizID: "4",
			Status: model.InstApproved, InitiatorID: 1, CreatedAt: base, FinishedAt: fin(1.5)},
		{TenantID: other, DefinitionID: 9, DefinitionKey: "leave", Title: "x", BizType: "t", BizID: "9",
			Status: model.InstApproved, InitiatorID: 9, CreatedAt: base, FinishedAt: fin(100)},
	}).Error; err != nil {
		t.Fatal(err)
	}

	stats, err := s.Stats(tenant)
	if err != nil {
		t.Fatal(err)
	}

	byKey := map[string]DefStatsItem{}
	for _, d := range stats.Definitions {
		byKey[d.DefinitionKey] = d
	}
	if len(byKey) != 2 {
		t.Fatalf("定义聚合应只含本租户 2 个 key，实得 %+v", stats.Definitions)
	}

	leave := byKey["leave"]
	if leave.Total != 3 || leave.Approved != 1 || leave.Rejected != 1 || leave.Running != 1 {
		t.Errorf("leave 计数 = %+v，want total3/appr1/rej1/run1", leave)
	}
	if leave.AvgHours != 3.0 {
		t.Errorf("leave AvgHours = %v, want 3.0（(2+4)/2，运行中不计）", leave.AvgHours)
	}
	if leave.Name != "请假" {
		t.Errorf("leave Name = %q, want 请假（最新版本名）", leave.Name)
	}

	expense := byKey["expense"]
	if expense.Total != 1 || expense.AvgHours != 1.5 {
		t.Errorf("expense = %+v, want total1 avg1.5", expense)
	}

	if stats.StatusCounts[model.InstApproved] != 2 {
		t.Errorf("StatusCounts[approved] = %d, want 2（他租户不计）",
			stats.StatusCounts[model.InstApproved])
	}
}

// 节点瓶颈的 AVG(created→acted) 同样下推 SQL。
func TestStatsNodeAvgHoursPushedDown(t *testing.T) {
	s := newTestStore(t)
	const tenant, other uint64 = 1, 2

	base := time.Now().Add(-48 * time.Hour)
	acted := func(h float64) *time.Time {
		v := base.Add(time.Duration(h * float64(time.Hour)))
		return &v
	}
	// 审批节点：已处理 2 条（1h、3h → 2.0），待办 1 条（acted_at 空，不计）
	if err := s.db.Create(&[]model.Task{
		{TenantID: tenant, InstanceID: 1, NodeID: "n1", NodeName: "审批", AssigneeID: 1,
			Status: model.TaskApproved, CreatedAt: base, ActedAt: acted(1)},
		{TenantID: tenant, InstanceID: 2, NodeID: "n1", NodeName: "审批", AssigneeID: 1,
			Status: model.TaskApproved, CreatedAt: base, ActedAt: acted(3)},
		{TenantID: tenant, InstanceID: 3, NodeID: "n1", NodeName: "审批", AssigneeID: 1,
			Status: model.TaskPending, CreatedAt: base},
		{TenantID: other, InstanceID: 4, NodeID: "n1", NodeName: "审批", AssigneeID: 9,
			Status: model.TaskApproved, CreatedAt: base, ActedAt: acted(90)},
	}).Error; err != nil {
		t.Fatal(err)
	}

	stats, err := s.Stats(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.NodeBottlenecks) != 1 {
		t.Fatalf("节点聚合 = %+v，只应含本租户「审批」", stats.NodeBottlenecks)
	}
	n := stats.NodeBottlenecks[0]
	if n.NodeName != "审批" || n.Acted != 2 || n.AvgHours != 2.0 {
		t.Errorf("节点 = %+v, want 审批/acted2/avg2.0（待办与他租户均不计）", n)
	}
}
