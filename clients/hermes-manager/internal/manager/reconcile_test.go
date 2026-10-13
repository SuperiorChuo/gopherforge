package manager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/adapter"
	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
	"github.com/go-admin-kit/clients/hermes-manager/internal/state"
)

// fakeAdapter 记录调用序列，行为由字段驱动。
type fakeAdapter struct {
	calls                 []string
	installedVer          string
	previousVer           string
	running               bool
	installErr            error
	installHook           func(protocol.DesiredStateArtifact)
	startErr              error
	stopErr               error
	restartErr            error
	runningErr            error
	profileErr            error
	modelTokenErr         error
	appliedModelToken     string
	rollbackErr           error
	startLeavesStopped    bool
	stopLeavesRunning     bool
	rollbackLeavesStopped bool
	profileChanged        bool
	digest                string
	// 三期：API server 管道与任务下发
	appliedAPIServerKey string
	pushedTask          protocol.DesiredStateTask
	pushTaskErr         error
	pushTaskResult      string
	// Kanban 里程碑
	workerProfileNames []string
	workerProfileErr   error
	ensuredBoardSlug   string
	ensureBoardErr     error
	createdKanban      protocol.DesiredStateKanban
	createdKanbanKey   string
	createKanbanErr    error
	createdTaskID      string
	showStatus         string
	showSummary        string
	showErr            error
	showCalls          int
	// M2 技能市场
	appliedSkillBundles []adapter.SkillBundle
	applySkillsErr      error
	// M3 MCP 管理
	appliedMcpSecrets   map[string]string
	applyMcpSecretsErr  error
}

func (f *fakeAdapter) Install(_ context.Context, artifact protocol.DesiredStateArtifact) error {
	f.calls = append(f.calls, "install")
	if f.installHook != nil {
		f.installHook(artifact)
	}
	if f.installErr != nil {
		return f.installErr
	}
	f.previousVer = f.installedVer
	f.installedVer = artifact.Version
	// 真实 Install 切指针前会停旧进程。
	f.running = false
	return nil
}

func (f *fakeAdapter) InstalledVersion() (string, error) { return f.installedVer, nil }

func (f *fakeAdapter) ApplyProfile(_ context.Context, _ *protocol.DesiredState, _ string) (string, bool, error) {
	f.calls = append(f.calls, "apply_profile")
	if f.profileErr != nil {
		return "", false, f.profileErr
	}
	digest := f.digest
	if digest == "" {
		digest = "digest-1"
	}
	return digest, f.profileChanged, nil
}

func (f *fakeAdapter) ApplyModelToken(_ context.Context, modelToken string) error {
	f.calls = append(f.calls, "apply_model_token")
	if f.modelTokenErr != nil {
		return f.modelTokenErr
	}
	f.appliedModelToken = modelToken
	return nil
}

func (f *fakeAdapter) ApplyAPIServerKey(_ context.Context, apiServerKey string) error {
	f.calls = append(f.calls, "apply_api_server")
	if apiServerKey != "" {
		f.appliedAPIServerKey = apiServerKey
	}
	return nil
}

func (f *fakeAdapter) PushTask(_ context.Context, task protocol.DesiredStateTask, _ string) (string, error) {
	f.calls = append(f.calls, "push_task")
	if f.pushTaskErr != nil {
		return "", f.pushTaskErr
	}
	f.pushedTask = task
	return f.pushTaskResult, nil
}

func (f *fakeAdapter) ApplyWorkerProfiles(_ context.Context, desired *protocol.DesiredState) error {
	f.calls = append(f.calls, "apply_worker_profiles")
	if f.workerProfileErr != nil {
		return f.workerProfileErr
	}
	if desired != nil && desired.Kanban != nil {
		for _, worker := range desired.Kanban.Workers {
			f.workerProfileNames = append(f.workerProfileNames, worker.Name)
		}
	}
	return nil
}

func (f *fakeAdapter) EnsureKanbanBoard(_ context.Context, boardSlug string) error {
	f.calls = append(f.calls, "ensure_kanban_board")
	if f.ensureBoardErr != nil {
		return f.ensureBoardErr
	}
	f.ensuredBoardSlug = boardSlug
	return nil
}

func (f *fakeAdapter) CreateKanbanTask(_ context.Context, task protocol.DesiredStateKanban, idempotencyKey string) (string, error) {
	f.calls = append(f.calls, "create_kanban_task")
	if f.createKanbanErr != nil {
		return "", f.createKanbanErr
	}
	f.createdKanban = task
	f.createdKanbanKey = idempotencyKey
	if f.createdTaskID == "" {
		f.createdTaskID = "t_00000000"
	}
	return f.createdTaskID, nil
}

func (f *fakeAdapter) ShowKanbanTask(_ context.Context, _, _ string) (*protocol.KanbanTaskStatus, error) {
	f.calls = append(f.calls, "show_kanban_task")
	f.showCalls++
	if f.showErr != nil {
		return nil, f.showErr
	}
	return &protocol.KanbanTaskStatus{ID: f.createdTaskID, Status: f.showStatus, LatestSummary: f.showSummary}, nil
}

func (f *fakeAdapter) ApplySkills(_ context.Context, bundles []adapter.SkillBundle) error {
	f.calls = append(f.calls, "apply_skills")
	if f.applySkillsErr != nil {
		return f.applySkillsErr
	}
	f.appliedSkillBundles = append(f.appliedSkillBundles, bundles...)
	return nil
}

func (f *fakeAdapter) ApplyMcpSecrets(_ context.Context, secrets map[string]string) error {
	f.calls = append(f.calls, "apply_mcp_secrets")
	if f.applyMcpSecretsErr != nil {
		return f.applyMcpSecretsErr
	}
	f.appliedMcpSecrets = secrets
	return nil
}

func (f *fakeAdapter) Start(context.Context) error {
	f.calls = append(f.calls, "start")
	if f.startErr != nil {
		return f.startErr
	}
	if !f.startLeavesStopped {
		f.running = true
	}
	return nil
}

func (f *fakeAdapter) Stop(context.Context) error {
	f.calls = append(f.calls, "stop")
	if f.stopErr != nil {
		return f.stopErr
	}
	if !f.stopLeavesRunning {
		f.running = false
	}
	return nil
}

func (f *fakeAdapter) Restart(ctx context.Context) error {
	f.calls = append(f.calls, "restart")
	if f.restartErr != nil {
		return f.restartErr
	}
	f.running = true
	return nil
}

func (f *fakeAdapter) Running(context.Context) (bool, error) {
	f.calls = append(f.calls, "running")
	err := f.runningErr
	f.runningErr = nil
	return f.running, err
}

func (f *fakeAdapter) Rollback(context.Context) error {
	f.calls = append(f.calls, "rollback")
	if f.rollbackErr != nil {
		return f.rollbackErr
	}
	f.installedVer = f.previousVer
	f.running = !f.rollbackLeavesStopped
	return nil
}

type recordedEvent struct{ Type, Result, Code string }

func collect(events *[]recordedEvent) eventSink {
	return func(eventType, result, errorCode string, _ uint64) {
		*events = append(*events, recordedEvent{eventType, result, errorCode})
	}
}

type recordedAppliedEvent struct {
	Type, Result string
	Applied      uint64
}

func collectApplied(events *[]recordedAppliedEvent) eventSink {
	return func(eventType, result, _ string, appliedRevision uint64) {
		*events = append(*events, recordedAppliedEvent{eventType, result, appliedRevision})
	}
}

func callIndex(calls []string, target string) int {
	for index, call := range calls {
		if call == target {
			return index
		}
	}
	return -1
}

func activeDesired(revision uint64) *protocol.DesiredState {
	return &protocol.DesiredState{UnsignedDesiredState: protocol.UnsignedDesiredState{
		Revision:     revision,
		DesiredState: "active",
		Hermes:       protocol.DesiredStateArtifact{ID: 1, Version: "v2026.7.20"},
	}}
}

func TestReconcileFreshInstallAndStart(t *testing.T) {
	adp := &fakeAdapter{}
	var events []recordedEvent
	config := &state.Config{}

	out := reconcile(context.Background(), adp, config, activeDesired(3), "", false, collect(&events))

	if out.ActualState != "active" || out.HealthStatus != "healthy" || out.LastErrorCode != "" {
		t.Fatalf("outcome = %+v", out)
	}
	if out.AppliedRevision != 3 || out.HermesVersion != "v2026.7.20" {
		t.Fatalf("revision/version = %d/%s", out.AppliedRevision, out.HermesVersion)
	}
	want := []recordedEvent{{"install", "succeeded", ""}, {"start", "succeeded", ""}}
	if len(events) != len(want) || events[0] != want[0] || events[1] != want[1] {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestReconcileInstallFailureKeepsAppliedRevision(t *testing.T) {
	adp := &fakeAdapter{installErr: adapter.Coded(adapter.ErrCodeDigestMismatch, errors.New("bad digest"))}
	var events []recordedEvent
	config := &state.Config{AppliedPolicyRevision: 2}

	out := reconcile(context.Background(), adp, config, activeDesired(3), "", false, collect(&events))

	if out.ActualState != "error" || out.LastErrorCode != adapter.ErrCodeDigestMismatch {
		t.Fatalf("outcome = %+v", out)
	}
	// 失败不得谎报已应用新策略
	if out.AppliedRevision != 2 {
		t.Fatalf("applied revision = %d, want 2 (unchanged)", out.AppliedRevision)
	}
	if len(events) != 1 || events[0] != (recordedEvent{"install", "failed", adapter.ErrCodeDigestMismatch}) {
		t.Fatalf("events = %v", events)
	}
}

func TestReconcileUpdateInstallFailureRemainsQuarantined(t *testing.T) {
	desired := activeDesired(12)
	desired.PendingAction = "retry_update"
	adp := &fakeAdapter{
		installedVer: "v-old",
		running:      true,
		installErr:   adapter.Coded(adapter.ErrCodeDigestMismatch, errors.New("bad digest")),
	}
	config := &state.Config{AppliedPolicyRevision: 11, HermesVersion: "v-old"}
	var events []recordedEvent

	out := reconcile(context.Background(), adp, config, desired, "", false, collect(&events))
	applyOutcome(config, desired, out, time.Now())

	if !out.UpdateQuarantined || config.FailedUpdateArtifactID != desired.Hermes.ID ||
		config.FailedUpdatePolicyRevision != desired.Revision || config.FailedUpdateVersion != desired.Hermes.Version {
		t.Fatalf("failed update quarantine outcome/config = %+v / %+v", out, config)
	}
	if len(events) != 1 || events[0] != (recordedEvent{"update", "failed", adapter.ErrCodeDigestMismatch}) {
		t.Fatalf("events = %v", events)
	}
}

func TestReconcileQuarantinesRolledBackTargetUntilExplicitRetry(t *testing.T) {
	desired := activeDesired(12)
	config := &state.Config{
		AppliedPolicyRevision:      11,
		HermesVersion:              "v-old",
		ConfigDigest:               "old-digest",
		LastErrorCode:              adapter.ErrCodeProfileFailed,
		FailedUpdateArtifactID:     desired.Hermes.ID,
		FailedUpdatePolicyRevision: desired.Revision,
		FailedUpdateVersion:        desired.Hermes.Version,
	}
	adp := &fakeAdapter{installedVer: "v-old", running: true}

	out := reconcile(context.Background(), adp, config, desired, "", false, func(string, string, string, uint64) {})
	if !out.UpdateQuarantined || out.HermesVersion != "v-old" || out.AppliedRevision != 11 {
		t.Fatalf("quarantined outcome = %+v", out)
	}
	for _, call := range adp.calls {
		if call == "install" || call == "apply_profile" {
			t.Fatalf("quarantined target must not be retried, calls = %v", adp.calls)
		}
	}

	adp.calls = nil
	desired.PendingAction = "retry_update"
	out = reconcile(context.Background(), adp, config, desired, "", false, func(string, string, string, uint64) {})
	if out.UpdateQuarantined {
		t.Fatalf("successful retry must clear quarantine: %+v", out)
	}
	if len(adp.calls) == 0 || adp.calls[0] != "install" {
		t.Fatalf("retry_update must retry installation, calls = %v", adp.calls)
	}
}

func TestReconcileQuarantinedTargetStillAppliesRotatedModelToken(t *testing.T) {
	desired := activeDesired(12)
	config := &state.Config{
		AppliedPolicyRevision:      11,
		HermesVersion:              "v-old",
		ConfigDigest:               "old-digest",
		LastErrorCode:              adapter.ErrCodeProfileFailed,
		FailedUpdateArtifactID:     desired.Hermes.ID,
		FailedUpdatePolicyRevision: desired.Revision,
		FailedUpdateVersion:        desired.Hermes.Version,
	}
	adp := &fakeAdapter{installedVer: "v-old", running: true}

	out := reconcile(context.Background(), adp, config, desired, "fresh-token", true, func(string, string, string, uint64) {})

	if !out.UpdateQuarantined || !out.ModelTokenApplied {
		t.Fatalf("quarantined token rotation outcome = %+v", out)
	}
	if adp.appliedModelToken != "fresh-token" {
		t.Fatalf("applied model token = %q, want fresh token", adp.appliedModelToken)
	}
	applyToken := callIndex(adp.calls, "apply_model_token")
	restart := callIndex(adp.calls, "restart")
	if applyToken < 0 || restart < 0 || applyToken > restart {
		t.Fatalf("token must be persisted before old version restarts, calls = %v", adp.calls)
	}
	if callIndex(adp.calls, "install") >= 0 || callIndex(adp.calls, "apply_profile") >= 0 {
		t.Fatalf("quarantined target must not reapply failed artifact/policy, calls = %v", adp.calls)
	}
}

func TestReconcileZeroArtifactIDDoesNotMatchQuarantine(t *testing.T) {
	desired := activeDesired(12)
	desired.Hermes = protocol.DesiredStateArtifact{}
	config := &state.Config{
		AppliedPolicyRevision:      11,
		HermesVersion:              "v-old",
		FailedUpdateArtifactID:     0,
		FailedUpdatePolicyRevision: desired.Revision,
		FailedUpdateVersion:        desired.Hermes.Version,
	}
	adp := &fakeAdapter{installedVer: "v-old", running: true}

	out := reconcile(context.Background(), adp, config, desired, "", false, func(string, string, string, uint64) {})

	if out.UpdateQuarantined {
		t.Fatalf("artifact id 0 must not match quarantine: %+v", out)
	}
	if out.ActualState != "active" || out.HealthStatus != "healthy" || out.AppliedRevision != desired.Revision {
		t.Fatalf("zero-artifact policy must reconcile normally: %+v", out)
	}
	foundProfile := false
	for _, call := range adp.calls {
		if call == "apply_profile" {
			foundProfile = true
		}
	}
	if !foundProfile {
		t.Fatalf("zero artifact id was incorrectly short-circuited, calls = %v", adp.calls)
	}
}

func TestReconcileStopsWhenDesiredStopped(t *testing.T) {
	adp := &fakeAdapter{running: true, installedVer: "v2026.7.20"}
	var events []recordedEvent
	desired := activeDesired(4)
	desired.DesiredState = "stopped"

	out := reconcile(context.Background(), adp, &state.Config{}, desired, "", false, collect(&events))

	if out.ActualState != "stopped" || out.HealthStatus != "healthy" {
		t.Fatalf("outcome = %+v", out)
	}
	if len(events) != 1 || events[0].Type != "stop" || events[0].Result != "succeeded" {
		t.Fatalf("events = %v", events)
	}
	if adp.running {
		t.Fatal("process should be stopped")
	}
}

func TestReconcileRevokedStopsWithoutWipe(t *testing.T) {
	adp := &fakeAdapter{running: true}
	desired := activeDesired(5)
	desired.DesiredState = "revoked"
	var events []recordedEvent

	out := reconcile(context.Background(), adp, &state.Config{}, desired, "", false, collect(&events))

	if out.ActualState != "revoked" {
		t.Fatalf("outcome = %+v", out)
	}
	for _, call := range adp.calls {
		if call == "rollback" || call == "install" {
			t.Fatalf("revoke must only stop, got calls %v", adp.calls)
		}
	}
	if len(events) != 1 || events[0] != (recordedEvent{"revoke", "succeeded", ""}) {
		t.Fatalf("events = %v, want revoke confirmation", events)
	}
}

func TestReconcileDoesNotConfirmStopOrRevokeWhileProcessStillRuns(t *testing.T) {
	tests := []struct {
		name      string
		desired   string
		eventType string
	}{
		{name: "stopped", desired: "stopped", eventType: "stop"},
		{name: "revoked", desired: "revoked", eventType: "revoke"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			adp := &fakeAdapter{running: true, stopLeavesRunning: true}
			desired := activeDesired(6)
			desired.DesiredState = tt.desired
			var events []recordedEvent

			out := reconcile(ctx, adp, &state.Config{AppliedPolicyRevision: 5}, desired, "", false, collect(&events))

			if out.ActualState != "error" || out.HealthStatus != "unhealthy" || out.LastErrorCode != adapter.ErrCodeStopFailed {
				t.Fatalf("outcome = %+v, want terminal stop failure", out)
			}
			if out.AppliedRevision != 5 {
				t.Fatalf("failed stop applied revision = %d, want unchanged 5", out.AppliedRevision)
			}
			want := recordedEvent{tt.eventType, "failed", adapter.ErrCodeStopFailed}
			if len(events) != 1 || events[0] != want {
				t.Fatalf("events = %v, want %v", events, want)
			}
			if !adp.running {
				t.Fatal("test precondition lost: fake process should still be running")
			}
		})
	}
}

func TestReconcileUpdateRollsBackPostInstallFailures(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		prepare func(*fakeAdapter, *protocol.DesiredState)
	}{
		{
			name: "apply profile",
			code: adapter.ErrCodeProfileFailed,
			prepare: func(adp *fakeAdapter, _ *protocol.DesiredState) {
				adp.profileErr = adapter.Coded(adapter.ErrCodeProfileFailed, errors.New("profile failed"))
			},
		},
		{
			name: "restart",
			code: adapter.ErrCodeStartFailed,
			prepare: func(adp *fakeAdapter, desired *protocol.DesiredState) {
				desired.PendingAction = "restart"
				adp.restartErr = adapter.Coded(adapter.ErrCodeStartFailed, errors.New("restart failed"))
			},
		},
		{
			name: "running check",
			code: "PROCESS_STATUS_FAILED",
			prepare: func(adp *fakeAdapter, _ *protocol.DesiredState) {
				adp.runningErr = adapter.Coded("PROCESS_STATUS_FAILED", errors.New("status failed"))
			},
		},
		{
			name: "start",
			code: adapter.ErrCodeStartFailed,
			prepare: func(adp *fakeAdapter, _ *protocol.DesiredState) {
				adp.startErr = adapter.Coded(adapter.ErrCodeStartFailed, errors.New("start failed"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := &fakeAdapter{installedVer: "v-old", running: true}
			desired := activeDesired(12)
			config := &state.Config{
				AppliedPolicyRevision: 11,
				ConfigDigest:          "digest-old",
				HermesVersion:         "v-old",
			}
			tt.prepare(adp, desired)
			var events []recordedEvent

			out := reconcile(context.Background(), adp, config, desired, "", false, collect(&events))

			if out.AppliedRevision != 11 || out.ConfigDigest != "digest-old" || out.HermesVersion != "v-old" {
				t.Fatalf("rolled-back state = %+v", out)
			}
			if out.ActualState != "active" || out.HealthStatus != "degraded" || out.LastErrorCode != tt.code {
				t.Fatalf("rolled-back health = %+v", out)
			}
			if adp.installedVer != "v-old" || !adp.running {
				t.Fatalf("adapter did not restore old running version: version=%s running=%t calls=%v", adp.installedVer, adp.running, adp.calls)
			}
			wantEvents := []recordedEvent{
				{"update", "failed", tt.code},
				{"rollback", "succeeded", ""},
			}
			if len(events) != len(wantEvents) || events[0] != wantEvents[0] || events[1] != wantEvents[1] {
				t.Fatalf("events = %v, want %v", events, wantEvents)
			}
		})
	}
}

func TestReconcileReportsRollbackFailure(t *testing.T) {
	adp := &fakeAdapter{
		installedVer: "v-old",
		running:      true,
		profileErr:   adapter.Coded(adapter.ErrCodeProfileFailed, errors.New("profile failed")),
		rollbackErr:  adapter.Coded(adapter.ErrCodeRollbackFailed, errors.New("rollback failed")),
	}
	config := &state.Config{
		AppliedPolicyRevision:          11,
		ConfigDigest:                   "digest-old",
		HermesVersion:                  "v-old",
		UpdateInProgressArtifactID:     1,
		UpdateInProgressPolicyRevision: 12,
		UpdateInProgressVersion:        "v2026.7.20",
	}
	var events []recordedEvent

	out := reconcile(context.Background(), adp, config, activeDesired(12), "", false, collect(&events))

	wantEvents := []recordedEvent{
		{"update", "failed", adapter.ErrCodeProfileFailed},
		{"rollback", "failed", adapter.ErrCodeRollbackFailed},
	}
	if len(events) != len(wantEvents) || events[0] != wantEvents[0] || events[1] != wantEvents[1] {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
	if out.ActualState != "error" || out.HealthStatus != "unhealthy" || out.LastErrorCode != adapter.ErrCodeRollbackFailed {
		t.Fatalf("outcome = %+v", out)
	}
	applyOutcome(config, activeDesired(12), out, time.Now())
	if !out.PreserveUpdateInProgress || config.UpdateInProgressArtifactID != 1 || config.FailedUpdateArtifactID != 0 {
		t.Fatalf("failed rollback must preserve recovery intent without entering quarantine: outcome=%+v config=%+v", out, config)
	}
}

func TestReconcileReportsRollbackFailureWhenOldVersionDoesNotStayRunning(t *testing.T) {
	adp := &fakeAdapter{
		installedVer:          "v-old",
		running:               true,
		profileErr:            adapter.Coded(adapter.ErrCodeProfileFailed, errors.New("profile failed")),
		rollbackLeavesStopped: true,
	}
	config := &state.Config{AppliedPolicyRevision: 11, ConfigDigest: "digest-old", HermesVersion: "v-old"}
	var events []recordedEvent
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out := reconcile(ctx, adp, config, activeDesired(12), "", false, collect(&events))

	wantEvents := []recordedEvent{
		{"update", "failed", adapter.ErrCodeProfileFailed},
		{"rollback", "failed", adapter.ErrCodeStartFailed},
	}
	if len(events) != len(wantEvents) || events[0] != wantEvents[0] || events[1] != wantEvents[1] {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
	if out.ActualState != "error" || out.HealthStatus != "unhealthy" || out.LastErrorCode != adapter.ErrCodeStartFailed {
		t.Fatalf("outcome = %+v", out)
	}
	if out.HermesVersion != "v-old" || !out.UpdateQuarantined {
		t.Fatalf("failed rollback must retain old version and quarantine target: %+v", out)
	}
}

func TestReconcileRollsBackWhenStartReturnsBeforeProcessIsRunning(t *testing.T) {
	previousInterval := processStartCheckInterval
	processStartCheckInterval = time.Millisecond
	t.Cleanup(func() { processStartCheckInterval = previousInterval })
	adp := &fakeAdapter{
		installedVer:       "v-old",
		running:            true,
		startLeavesStopped: true,
	}
	config := &state.Config{AppliedPolicyRevision: 11, ConfigDigest: "digest-old", HermesVersion: "v-old"}
	var events []recordedEvent
	out := reconcile(context.Background(), adp, config, activeDesired(12), "", false, collect(&events))

	wantEvents := []recordedEvent{
		{"update", "failed", adapter.ErrCodeStartFailed},
		{"rollback", "succeeded", ""},
	}
	if len(events) != len(wantEvents) || events[0] != wantEvents[0] || events[1] != wantEvents[1] {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
	if out.ActualState != "active" || out.HealthStatus != "degraded" || out.HermesVersion != "v-old" {
		t.Fatalf("rolled-back outcome = %+v", out)
	}
}

func TestReconcileUpdateSucceedsOnlyAfterProcessConverges(t *testing.T) {
	adp := &fakeAdapter{installedVer: "v-old", running: true}
	var events []recordedEvent

	out := reconcile(context.Background(), adp, &state.Config{AppliedPolicyRevision: 11}, activeDesired(12), "", false, collect(&events))

	if out.ActualState != "active" || out.AppliedRevision != 12 {
		t.Fatalf("outcome = %+v", out)
	}
	if len(events) != 2 || events[0] != (recordedEvent{"start", "succeeded", ""}) || events[1] != (recordedEvent{"update", "succeeded", ""}) {
		t.Fatalf("events = %v, want start succeeded before update succeeded", events)
	}
}

func TestReconcileTerminalEventsCarryTheAppliedRevisionAtEmission(t *testing.T) {
	adp := &fakeAdapter{installedVer: "v-old", running: true}
	var events []recordedAppliedEvent

	reconcile(
		context.Background(),
		adp,
		&state.Config{AppliedPolicyRevision: 11, HermesVersion: "v-old"},
		activeDesired(12),
		"",
		false,
		collectApplied(&events),
	)

	if len(events) != 2 {
		t.Fatalf("events = %+v, want start and update terminal events", events)
	}
	if events[0] != (recordedAppliedEvent{"start", "succeeded", 11}) {
		t.Fatalf("start event = %+v, want pre-convergence revision 11", events[0])
	}
	if events[1] != (recordedAppliedEvent{"update", "succeeded", 12}) {
		t.Fatalf("update event = %+v, want converged revision 12", events[1])
	}

	for _, desiredState := range []string{"stopped", "revoked"} {
		t.Run(desiredState, func(t *testing.T) {
			adp := &fakeAdapter{installedVer: "v2026.7.20", running: true}
			desired := activeDesired(14)
			desired.DesiredState = desiredState
			var terminal []recordedAppliedEvent

			reconcile(
				context.Background(), adp,
				&state.Config{AppliedPolicyRevision: 13}, desired,
				"", false, collectApplied(&terminal),
			)

			if len(terminal) != 1 || terminal[0].Result != "succeeded" || terminal[0].Applied != 14 {
				t.Fatalf("terminal event = %+v, want applied revision 14", terminal)
			}
		})
	}
}

func TestReconcileResumesPersistedUpdateAndRollsBackAcrossSync(t *testing.T) {
	desired := activeDesired(12)
	adp := &fakeAdapter{
		installedVer: "v2026.7.20",
		previousVer:  "v-old",
		running:      false,
		profileErr:   adapter.Coded(adapter.ErrCodeProfileFailed, errors.New("profile failed after manager restart")),
	}
	config := &state.Config{
		AppliedPolicyRevision:          11,
		HermesVersion:                  "v-old",
		ConfigDigest:                   "digest-old",
		UpdateInProgressArtifactID:     desired.Hermes.ID,
		UpdateInProgressPolicyRevision: desired.Revision,
		UpdateInProgressVersion:        desired.Hermes.Version,
	}
	var events []recordedEvent

	out := reconcile(context.Background(), adp, config, desired, "", false, collect(&events))
	applyOutcome(config, desired, out, time.Now())

	if callIndex(adp.calls, "install") >= 0 {
		t.Fatalf("persisted update must resume after pointer switch, not reinstall: %v", adp.calls)
	}
	if callIndex(adp.calls, "rollback") < 0 || adp.installedVer != "v-old" {
		t.Fatalf("persisted failed update was not rolled back: version=%q calls=%v", adp.installedVer, adp.calls)
	}
	if out.ActualState != "active" || out.HealthStatus != "degraded" || out.HermesVersion != "v-old" || !out.UpdateQuarantined {
		t.Fatalf("recovered update outcome = %+v", out)
	}
	if config.UpdateInProgressArtifactID != 0 || config.UpdateInProgressPolicyRevision != 0 || config.UpdateInProgressVersion != "" {
		t.Fatalf("completed rollback must clear in-progress marker: %+v", config)
	}
	wantEvents := []recordedEvent{
		{"update", "failed", adapter.ErrCodeProfileFailed},
		{"rollback", "succeeded", ""},
	}
	if len(events) != len(wantEvents) || events[0] != wantEvents[0] || events[1] != wantEvents[1] {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
}

func TestReconcileDriftOnlyWhenSameRevision(t *testing.T) {
	// 同 revision 下磁盘内容偏离 = 员工篡改 → 漂移事件
	adp := &fakeAdapter{installedVer: "v2026.7.20", running: true, profileChanged: true}
	var events []recordedEvent
	config := &state.Config{AppliedPolicyRevision: 6}
	reconcile(context.Background(), adp, config, activeDesired(6), "", false, collect(&events))
	found := false
	for _, e := range events {
		if e.Type == "policy_drift" {
			found = true
		}
	}
	if !found {
		t.Fatalf("same-revision change must report drift, events = %v", events)
	}

	// revision 变了 = 正常策略更新，不是漂移
	events = nil
	config = &state.Config{AppliedPolicyRevision: 6}
	reconcile(context.Background(), adp, config, activeDesired(7), "", false, collect(&events))
	for _, e := range events {
		if e.Type == "policy_drift" {
			t.Fatalf("revision change must not be drift, events = %v", events)
		}
	}
}

func TestReconcilePendingActions(t *testing.T) {
	// restart：真动作 + 终态事件
	adp := &fakeAdapter{installedVer: "v2026.7.20", running: true}
	var events []recordedEvent
	desired := activeDesired(8)
	desired.PendingAction = "restart"
	reconcile(context.Background(), adp, &state.Config{AppliedPolicyRevision: 8}, desired, "", false, collect(&events))
	restarted := false
	for _, call := range adp.calls {
		if call == "restart" {
			restarted = true
		}
	}
	if !restarted {
		t.Fatalf("restart action must call Restart, calls = %v", adp.calls)
	}
	sawStart := false
	for _, e := range events {
		if e.Type == "start" && e.Result == "succeeded" {
			sawStart = true
		}
	}
	if !sawStart {
		t.Fatalf("restart must emit terminal start event, events = %v", events)
	}

	// retry_update 且版本已一致：不重复安装，但必须补终态事件让服务端清动作
	adp = &fakeAdapter{installedVer: "v2026.7.20", running: true}
	events = nil
	desired = activeDesired(8)
	desired.PendingAction = "retry_update"
	reconcile(context.Background(), adp, &state.Config{AppliedPolicyRevision: 8}, desired, "", false, collect(&events))
	for _, call := range adp.calls {
		if call == "install" {
			t.Fatalf("matching version must not reinstall, calls = %v", adp.calls)
		}
	}
	sawUpdate := false
	for _, e := range events {
		if e.Type == "update" && e.Result == "succeeded" {
			sawUpdate = true
		}
	}
	if !sawUpdate {
		t.Fatalf("retry_update must emit terminal update event, events = %v", events)
	}
}

// 换发模型 token 后必须重启：上游 Hermes 只在启动时读 .env，不重启新凭据不生效，
// 而服务端换发即吊销旧 token——运行中的 gateway 会拿着废凭据一直失败。
func TestReconcileRestartsAfterTokenRotation(t *testing.T) {
	adp := &fakeAdapter{installedVer: "v2026.7.20", running: true}
	var events []recordedEvent

	reconcile(context.Background(), adp, &state.Config{AppliedPolicyRevision: 9}, activeDesired(9), "fresh-token", true, collect(&events))

	restarted := false
	for _, call := range adp.calls {
		if call == "restart" {
			restarted = true
		}
	}
	if !restarted {
		t.Fatalf("token rotation must restart hermes, calls = %v", adp.calls)
	}

	// 没换 token 时正在运行就不该重启（避免无谓打断员工会话）
	adp = &fakeAdapter{installedVer: "v2026.7.20", running: true}
	events = nil
	reconcile(context.Background(), adp, &state.Config{AppliedPolicyRevision: 9}, activeDesired(9), "", false, collect(&events))
	for _, call := range adp.calls {
		if call == "restart" || call == "start" {
			t.Fatalf("steady state must not touch the process, calls = %v", adp.calls)
		}
	}
}

func TestModelTokenDueWhenRestoringActiveState(t *testing.T) {
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	mgr := &Manager{now: func() time.Time { return now }}
	future := now.Add(time.Hour)

	if !mgr.modelTokenDue(&state.Config{ActualState: "stopped", ModelTokenExpiresAt: future}) {
		t.Fatal("stopped device must refresh its revoked token when becoming active")
	}
	if !mgr.modelTokenDue(&state.Config{ActualState: "revoked", ModelTokenExpiresAt: future}) {
		t.Fatal("revoked device must refresh its revoked token when becoming active")
	}
	if mgr.modelTokenDue(&state.Config{ActualState: "active", ModelTokenExpiresAt: future}) {
		t.Fatal("active device with sufficient token lifetime must not rotate")
	}
	if !mgr.modelTokenRequired(&state.Config{ActualState: "stopped", ModelTokenExpiresAt: future}) {
		t.Fatal("stopped device has no usable token even when the old expiry is in the future")
	}
	if !mgr.modelTokenRequired(&state.Config{ActualState: "active", ModelTokenExpiresAt: now}) {
		t.Fatal("expired token must be required before an active process can continue")
	}
	if mgr.modelTokenRequired(&state.Config{ActualState: "active", ModelTokenExpiresAt: future}) {
		t.Fatal("active device may keep using a still-valid token when early renewal fails")
	}
}
