package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// M5 协议契约：Cron 为最后一个 omitempty 字段——空时省略（P1 payload 字节不变），
// 非空时以 "cron" key 输出。manager 侧 mirror 必须逐字一致（签名字节校验）。
func TestDesiredStateCronJSON(t *testing.T) {
	empty := hermesUnsignedDesiredState{SchemaVersion: 1}
	rawEmpty, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	if strings.Contains(string(rawEmpty), "cron") {
		t.Fatalf("empty state must omit cron, got %s", rawEmpty)
	}

	full := hermesUnsignedDesiredState{
		SchemaVersion: 1,
		Cron: []HermesDesiredStateCronJob{
			{ID: 5, Name: "daily", CronExpression: "0 9 * * 1-5", Prompt: "日报", Enabled: true},
		},
	}
	rawFull, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("marshal full: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(rawFull, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cronArr, ok := back["cron"].([]any)
	if !ok || len(cronArr) != 1 {
		t.Fatalf("full state cron = %v, want one entry", back["cron"])
	}
	first := cronArr[0].(map[string]any)
	if first["job_id"] != float64(5) || first["prompt"] != "日报" {
		t.Fatalf("cron entry = %v", first)
	}
}

func TestValidateHermesCronExpression(t *testing.T) {
	cases := []struct {
		expr string
		ok   bool
	}{
		{"0 9 * * 1-5", true},      // 工作日 9 点
		{"*/15 * * * *", true},     // 每 15 分钟
		{"0 0 1 * *", true},        // 每月 1 号 0 点
		{"30 8 * * 1,3,5", true},   // 列表
		{"0 9 ? * 1", true},        // ? 日 + 周 1
		{"0 9 * * ?", true},        // ? 周
		{"0 9 * * * extra", false}, // 6 段
		{"0 9 * *", false},         // 4 段
		{"60 9 * * *", false},      // 分越界
		{"0 24 * * *", false},      // 时越界
		{"0 9 0 * *", false},       // 日 0
		{"0 9 * 13 *", false},      // 月越界
		{"0 9 * * 8", false},       // 周越界
		{"x 9 * * *", false},       // 非法值
		{"0 9 * ? *", false},       // ? 在月非法
		{"? 9 * * *", false},       // ? 在分非法
	}
	for _, c := range cases {
		err := validateHermesCronExpression(c.expr)
		if (err == nil) != c.ok {
			t.Errorf("validate(%q) = %v, want ok=%v", c.expr, err, c.ok)
		}
	}
}

func TestCreateCronJobValidation(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	valid := HermesCronJobRequest{
		Name:           "daily-report",
		CronExpression: "0 9 * * 1-5",
		ActionType:     "prompt",
		Prompt:         "生成昨日工作日报",
		Enabled:        true,
	}
	job, err := svc.CreateCronJob(ctx, 1, valid)
	if err != nil {
		t.Fatalf("create valid cron job: %v", err)
	}
	if job.Name != "daily-report" || !job.Enabled {
		t.Fatalf("job = %+v, want name/enabled set", job)
	}

	// 缺 prompt
	bad := valid
	bad.Prompt = ""
	if _, err := svc.CreateCronJob(ctx, 1, bad); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("empty prompt error = %v, want invalid input", err)
	}
	// 非法表达式
	bad = valid
	bad.CronExpression = "60 9 * * *"
	if _, err := svc.CreateCronJob(ctx, 1, bad); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("bad expr error = %v, want invalid input", err)
	}
	// 不支持的 action
	bad = valid
	bad.ActionType = "shell"
	if _, err := svc.CreateCronJob(ctx, 1, bad); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("bad action error = %v, want invalid input", err)
	}
}

// 描述的三态语义：传值=更新、传空串=清空、不传（nil）=不改。
// 曾有 bug：UpdateCronJob 完全不读 Description，前端编辑弹窗发了但静默丢。
func TestUpdateCronJobDescription(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	initial := "初始描述"
	job, err := svc.CreateCronJob(ctx, 1, HermesCronJobRequest{
		Name: "desc", CronExpression: "0 9 * * *", ActionType: "prompt", Prompt: "p",
		Enabled: true, Description: &initial,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if job.Description != "初始描述" {
		t.Fatalf("created description = %q", job.Description)
	}

	changed := "改后的描述"
	updated, err := svc.UpdateCronJob(ctx, 1, job.ID, HermesCronJobRequest{Description: &changed, Enabled: true})
	if err != nil {
		t.Fatalf("update description: %v", err)
	}
	if updated.Description != "改后的描述" {
		t.Fatalf("updated description = %q, want 改后的描述", updated.Description)
	}

	empty := ""
	cleared, err := svc.UpdateCronJob(ctx, 1, job.ID, HermesCronJobRequest{Description: &empty, Enabled: true})
	if err != nil {
		t.Fatalf("clear description: %v", err)
	}
	if cleared.Description != "" {
		t.Fatalf("cleared description = %q, want empty", cleared.Description)
	}

	kept, err := svc.UpdateCronJob(ctx, 1, job.ID, HermesCronJobRequest{Name: "desc2", Enabled: true})
	if err != nil {
		t.Fatalf("update without description: %v", err)
	}
	if kept.Description != "" || kept.Name != "desc2" {
		t.Fatalf("nil description must keep value, got %q / name %q", kept.Description, kept.Name)
	}
}

func TestListCronJobsAndToggle(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	_, err := svc.CreateCronJob(ctx, 1, HermesCronJobRequest{
		Name: "a", CronExpression: "0 9 * * 1-5", ActionType: "prompt", Prompt: "p", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	_, err = svc.CreateCronJob(ctx, 1, HermesCronJobRequest{
		Name: "b", CronExpression: "0 0 * * *", ActionType: "prompt", Prompt: "p2", Enabled: false,
	})
	if err != nil {
		t.Fatalf("create b: %v", err)
	}
	jobs, total, err := svc.ListCronJobs(ctx, 1, 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 || len(jobs) != 2 {
		t.Fatalf("list = %d/%d, want 2", len(jobs), total)
	}

	// 按名字找启用的 a，toggle → disabled
	var aID uint64
	for _, j := range jobs {
		if j.Name == "a" {
			aID = j.ID
			break
		}
	}
	toggled, err := svc.ToggleCronJob(ctx, 1, aID)
	if err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if toggled.Enabled {
		t.Fatalf("toggled job still enabled")
	}
}

func TestReportDeviceCronRun(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	job, err := svc.CreateCronJob(ctx, 1, HermesCronJobRequest{
		Name: "report", CronExpression: "0 9 * * *", ActionType: "prompt", Prompt: "p", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	// 合法上报
	if err := svc.ReportDeviceCronRun(ctx, "device-1", job.ID, "completed", "日报已生成"); err != nil {
		t.Fatalf("report: %v", err)
	}
	runs, total, err := svc.ListCronRuns(ctx, job.ID, 1, 20)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if total != 1 || len(runs) != 1 || runs[0].Status != "completed" || runs[0].DeviceID != "device-1" {
		t.Fatalf("runs = %+v/%d, want one completed device-1 run", runs, total)
	}

	// 非法状态
	if err := svc.ReportDeviceCronRun(ctx, "device-1", job.ID, "running", "x"); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("bad status error = %v, want invalid input", err)
	}
	// 空 result
	if err := svc.ReportDeviceCronRun(ctx, "device-1", job.ID, "completed", " "); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("empty result error = %v, want invalid input", err)
	}
	// 不存在的 job
	if err := svc.ReportDeviceCronRun(ctx, "device-1", 99999, "completed", "x"); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("missing job error = %v, want invalid input", err)
	}
}

func TestBuildDesiredStateCronEnabledOnly(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	// 建两个 job：一个启用一个停用
	if _, err := svc.CreateCronJob(ctx, 1, HermesCronJobRequest{
		Name: "on", CronExpression: "0 9 * * 1-5", ActionType: "prompt", Prompt: "p", Enabled: true,
	}); err != nil {
		t.Fatalf("create on: %v", err)
	}
	if _, err := svc.CreateCronJob(ctx, 1, HermesCronJobRequest{
		Name: "off", CronExpression: "0 0 * * *", ActionType: "prompt", Prompt: "p2", Enabled: false,
	}); err != nil {
		t.Fatalf("create off: %v", err)
	}
	jobs, err := svc.buildDesiredStateCron(ctx)
	if err != nil {
		t.Fatalf("build cron: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Name != "on" {
		t.Fatalf("desired cron = %+v, want only enabled 'on'", jobs)
	}
	if strings.TrimSpace(jobs[0].CronExpression) == "" || jobs[0].Prompt == "" {
		t.Fatalf("desired cron job missing fields: %+v", jobs[0])
	}
}
