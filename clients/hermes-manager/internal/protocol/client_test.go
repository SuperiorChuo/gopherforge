package protocol

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCanonicalRequestContract(t *testing.T) {
	got := string(CanonicalRequest(
		"post",
		"/api/v1/ai/hermes/device/heartbeat",
		"source=manager",
		"1785240000",
		"abcdefghijklmnop",
		"ABCDEF",
	))
	want := "POST\n/api/v1/ai/hermes/device/heartbeat\nsource=manager\n1785240000\nabcdefghijklmnop\nabcdef"
	if got != want {
		t.Fatalf("canonical request = %q, want %q", got, want)
	}
}

func TestClientSignedRequestAndDesiredStateVerification(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	devicePublic, devicePrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate device key: %v", err)
	}
	controlPublic, controlPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate control key: %v", err)
	}
	unsigned := UnsignedDesiredState{
		SchemaVersion:  1,
		Revision:       3,
		DesiredState:   "active",
		LeaseExpiresAt: now.Add(time.Hour),
		Profile:        "company",
		Model:          DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/proxy", Model: "approved-model"},
		Toolsets:       []string{"clarify", "memory", "no_mcp", "todo"},
		Privacy:        Privacy{},
	}
	payload, err := json.Marshal(unsigned)
	if err != nil {
		t.Fatalf("marshal unsigned state: %v", err)
	}
	desired := DesiredState{
		UnsignedDesiredState: unsigned,
		Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(controlPrivate, payload)),
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil {
			t.Errorf("read request: %v", readErr)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		digest := sha256.Sum256(body)
		digestHex := hex.EncodeToString(digest[:])
		if request.Header.Get("X-Hermes-Body-SHA256") != digestHex {
			t.Errorf("body digest header mismatch")
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		timestamp := request.Header.Get("X-Hermes-Timestamp")
		if timestamp != strconv.FormatInt(now.Unix(), 10) {
			t.Errorf("timestamp = %q", timestamp)
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		canonical := CanonicalRequest(
			request.Method,
			request.URL.EscapedPath(),
			request.URL.RawQuery,
			timestamp,
			request.Header.Get("X-Hermes-Nonce"),
			digestHex,
		)
		signature, decodeErr := base64.RawStdEncoding.DecodeString(request.Header.Get("X-Hermes-Signature"))
		if decodeErr != nil || !ed25519.Verify(devicePublic, canonical, signature) {
			t.Errorf("device signature did not verify")
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(Envelope[DesiredState]{Code: 200, Message: "success", Data: desired})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	client.now = func() time.Time { return now }
	state, err := client.DesiredState(t.Context(), &Identity{
		DeviceID:               "11111111-2222-4333-8444-555555555555",
		PrivateKey:             devicePrivate,
		PolicySigningPublicKey: base64.RawStdEncoding.EncodeToString(controlPublic),
	})
	if err != nil {
		t.Fatalf("desired state: %v", err)
	}
	if state.Revision != 3 || state.DesiredState != "active" {
		t.Fatalf("desired state = %+v", state)
	}
}

func TestDesiredStateRejectsSignedPolicyOutsideP1Allowlist(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	state := DesiredState{UnsignedDesiredState: UnsignedDesiredState{
		SchemaVersion:  1,
		DesiredState:   "active",
		LeaseExpiresAt: time.Now().Add(time.Hour),
		Profile:        "company",
		Toolsets:       []string{"clarify", "memory", "no_mcp", "todo", "terminal"},
		Privacy:        Privacy{},
	}}
	payload, err := json.Marshal(state.UnsignedDesiredState)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	state.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	err = state.Verify(base64.RawStdEncoding.EncodeToString(publicKey), time.Now())
	if err == nil || !strings.Contains(err.Error(), "toolsets") {
		t.Fatalf("Verify() error = %v, want toolset rejection", err)
	}
}

// P0 回归：服务端把 pending_action 签进期望状态载荷（omitempty）。客户端结构体
// 若缺该字段，重序列化验签在管理员排入动作的瞬间开始失败、全员同步中断。
func TestDesiredStateVerifiesWithPendingAction(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	unsigned := UnsignedDesiredState{
		SchemaVersion:  1,
		Revision:       5,
		DesiredState:   "active",
		LeaseExpiresAt: now.Add(time.Hour),
		Profile:        "company",
		Toolsets:       []string{"clarify", "memory", "no_mcp", "todo"},
		PendingAction:  "retry_update",
	}
	payload, err := json.Marshal(unsigned)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// 载荷里必须真的带上该字段（omitempty 只在空串时省略）
	if !strings.Contains(string(payload), `"pending_action":"retry_update"`) {
		t.Fatalf("payload missing pending_action: %s", payload)
	}
	signed := DesiredState{
		UnsignedDesiredState: unsigned,
		Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
	}
	if err := signed.Verify(base64.RawStdEncoding.EncodeToString(publicKey), now); err != nil {
		t.Fatalf("desired state with pending action must verify: %v", err)
	}

	// 空动作时字段必须被省略——否则轮到「无动作」的期望状态验签失败
	unsigned.PendingAction = ""
	emptyPayload, _ := json.Marshal(unsigned)
	if strings.Contains(string(emptyPayload), "pending_action") {
		t.Fatalf("empty pending_action must be omitted: %s", emptyPayload)
	}

	// 枚举外的动作即使签名有效也要拒绝——客户端不提供任意命令通道
	unsigned.PendingAction = "rm -rf /"
	badPayload, _ := json.Marshal(unsigned)
	bad := DesiredState{
		UnsignedDesiredState: unsigned,
		Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, badPayload)),
	}
	if err := bad.Verify(base64.RawStdEncoding.EncodeToString(publicKey), now); err == nil {
		t.Fatal("arbitrary pending action must be rejected even with a valid signature")
	}
}

// 三期：company_cluster 策略（dispatcher 开放工具集）必须能通过验签。
func TestDesiredStateVerifiesCompanyClusterToolsets(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	unsigned := UnsignedDesiredState{
		SchemaVersion:  1,
		Revision:       6,
		DesiredState:   "active",
		LeaseExpiresAt: now.Add(time.Hour),
		Profile:        "company_cluster",
		Toolsets:       []string{"clarify", "memory", "no_mcp", "todo", "delegation", "skills", "mcp"},
	}
	payload, _ := json.Marshal(unsigned)
	signed := DesiredState{
		UnsignedDesiredState: unsigned,
		Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
	}
	if err := signed.Verify(base64.RawStdEncoding.EncodeToString(publicKey), now); err != nil {
		t.Fatalf("company_cluster with dispatcher toolsets must verify: %v", err)
	}

	// 缺 P1 基线工具集（没有 todo）→ 拒绝
	unsigned.Toolsets = []string{"clarify", "memory", "no_mcp", "delegation"}
	payload, _ = json.Marshal(unsigned)
	signed.UnsignedDesiredState = unsigned
	signed.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	if err := signed.Verify(base64.RawStdEncoding.EncodeToString(publicKey), now); err == nil {
		t.Fatal("company_cluster missing P1 baseline toolset must be rejected")
	}

	// 越出 cluster 白名单（terminal）→ 拒绝
	unsigned.Toolsets = []string{"clarify", "memory", "no_mcp", "todo", "terminal"}
	payload, _ = json.Marshal(unsigned)
	signed.UnsignedDesiredState = unsigned
	signed.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	if err := signed.Verify(base64.RawStdEncoding.EncodeToString(publicKey), now); err == nil {
		t.Fatal("company_cluster with out-of-allowlist toolset must be rejected")
	}
}

// 三期：Task 字段字节一致回归（镜像 pending_action）。非 nil 必须进载荷、
// nil 必须省略（P1 payload 字节稳定）、畸形即使签名有效也拒。
func TestDesiredStateVerifiesWithTask(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	unsigned := UnsignedDesiredState{
		SchemaVersion:  1,
		Revision:       7,
		DesiredState:   "active",
		LeaseExpiresAt: now.Add(time.Hour),
		Profile:        "company_cluster",
		Toolsets:       []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		Task: &DesiredStateTask{
			TaskID:      11,
			RunID:       22,
			Description: "调研并输出报告",
			Model:       "gpt-4o",
		},
	}
	payload, _ := json.Marshal(unsigned)
	if !strings.Contains(string(payload), `"task":{"task_id":11,"run_id":22,"description":"调研并输出报告","model":"gpt-4o"}`) {
		t.Fatalf("payload missing task field: %s", payload)
	}
	signed := DesiredState{
		UnsignedDesiredState: unsigned,
		Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
	}
	if err := signed.Verify(base64.RawStdEncoding.EncodeToString(publicKey), now); err != nil {
		t.Fatalf("desired state with task must verify: %v", err)
	}

	// nil Task 必须被省略（P1 无任务 payload 字节不变）
	unsigned.Task = nil
	emptyPayload, _ := json.Marshal(unsigned)
	if strings.Contains(string(emptyPayload), `"task"`) {
		t.Fatalf("nil task must be omitted: %s", emptyPayload)
	}

	// 畸形任务（run_id=0）即使签名有效也要拒绝
	unsigned.Task = &DesiredStateTask{TaskID: 11, RunID: 0, Description: "x"}
	badPayload, _ := json.Marshal(unsigned)
	bad := DesiredState{
		UnsignedDesiredState: unsigned,
		Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, badPayload)),
	}
	if err := bad.Verify(base64.RawStdEncoding.EncodeToString(publicKey), now); err == nil {
		t.Fatal("malformed task must be rejected even with a valid signature")
	}

	// 超长描述也要拒
	unsigned.Task = &DesiredStateTask{TaskID: 11, RunID: 22, Description: strings.Repeat("x", 10001)}
	longPayload, _ := json.Marshal(unsigned)
	long := DesiredState{
		UnsignedDesiredState: unsigned,
		Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, longPayload)),
	}
	if err := long.Verify(base64.RawStdEncoding.EncodeToString(publicKey), now); err == nil {
		t.Fatal("overlong task description must be rejected even with a valid signature")
	}
}

// Kanban 里程碑：validateKanban 拒绝不安全的 worker 名/slug/assignee∉workers。
func TestDesiredStateVerifiesKanban(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signAndVerify := func(unsigned UnsignedDesiredState) error {
		payload, _ := json.Marshal(unsigned)
		signed := DesiredState{
			UnsignedDesiredState: unsigned,
			Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
		}
		return signed.Verify(base64.RawStdEncoding.EncodeToString(publicKey), now)
	}

	valid := UnsignedDesiredState{
		SchemaVersion: 1, Revision: 9, DesiredState: "active", LeaseExpiresAt: now.Add(time.Hour),
		Profile: "company_cluster", Toolsets: []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		Kanban: &DesiredStateKanban{
			TaskID: 1, RunID: 2, BoardSlug: "payments", Title: "调研", Body: "输出",
			Assignee: "worker-a", Workers: []DesiredStateKanbanWorker{{Name: "worker-a"}, {Name: "worker-b"}},
		},
	}
	if err := signAndVerify(valid); err != nil {
		t.Fatalf("valid kanban must verify: %v", err)
	}

	// worker 名含路径分隔符 / 必须拒（会成为文件系统目录名）
	bad := valid
	bad.Kanban.Workers = []DesiredStateKanbanWorker{{Name: "../escape"}}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("worker name with ../ must be rejected")
	}

	// assignee ∉ workers 必须拒
	bad = valid
	bad.Kanban.Assignee = "not-in-set"
	if err := signAndVerify(bad); err == nil {
		t.Fatal("assignee not in worker set must be rejected")
	}

	// slug 含大写/点必须拒
	bad = valid
	bad.Kanban.BoardSlug = "Bad.Slug"
	if err := signAndVerify(bad); err == nil {
		t.Fatal("board slug with dot must be rejected")
	}

	// worker 重复必须拒
	bad = valid
	bad.Kanban.Workers = []DesiredStateKanbanWorker{{Name: "worker-a"}, {Name: "worker-a"}}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("duplicate worker names must be rejected")
	}

	// 非 company_cluster 但带 Kanban 必须拒
	bad = valid
	bad.Profile = "company"
	if err := signAndVerify(bad); err == nil {
		t.Fatal("kanban on company profile must be rejected")
	}

	// Kanban nil 时字节稳定（P1/phase-3 payload 不变）
	plain := valid
	plain.Kanban = nil
	payload, _ := json.Marshal(plain)
	if strings.Contains(string(payload), `"kanban"`) {
		t.Fatalf("nil kanban must be omitted: %s", payload)
	}
}

// M2 技能市场：validateSkills 拒绝不安全技能名/坏摘要/重复 id。
func TestDesiredStateVerifiesSkills(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signAndVerify := func(unsigned UnsignedDesiredState) error {
		payload, _ := json.Marshal(unsigned)
		signed := DesiredState{
			UnsignedDesiredState: unsigned,
			Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
		}
		return signed.Verify(base64.RawStdEncoding.EncodeToString(publicKey), now)
	}

	valid := UnsignedDesiredState{
		SchemaVersion: 1, Revision: 9, DesiredState: "active", LeaseExpiresAt: now.Add(time.Hour),
		Profile: "company_cluster", Toolsets: []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		Skills: []DesiredStateSkill{{
			ID: 5, Name: "weekly-report", Version: "1.0.0", SHA256: strings.Repeat("a", 64),
			SourceURL: "/api/v1/ai/hermes/device/skills/5/download",
		}},
	}
	if err := signAndVerify(valid); err != nil {
		t.Fatalf("valid skills must verify: %v", err)
	}

	// 技能名含 / 必须拒（会成为目录名）
	bad := valid
	bad.Skills = []DesiredStateSkill{{ID: 5, Name: "../escape", Version: "1.0.0", SHA256: strings.Repeat("a", 64), SourceURL: "/api/v1/ai/hermes/device/skills/5/download"}}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("skill name with ../ must be rejected")
	}

	// 摘要非 64-hex 必须拒
	bad = valid
	bad.Skills = []DesiredStateSkill{{ID: 5, Name: "weekly-report", Version: "1.0.0", SHA256: "not-hex", SourceURL: "/api/v1/ai/hermes/device/skills/5/download"}}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("bad skill digest must be rejected")
	}

	// source_url 非本控制面相对路径必须拒
	bad = valid
	bad.Skills = []DesiredStateSkill{{ID: 5, Name: "weekly-report", Version: "1.0.0", SHA256: strings.Repeat("a", 64), SourceURL: "/evil/download"}}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("non-skills source url must be rejected")
	}

	// 重复 id 必须拒
	bad = valid
	bad.Skills = []DesiredStateSkill{
		{ID: 5, Name: "a", Version: "1.0.0", SHA256: strings.Repeat("a", 64), SourceURL: "/api/v1/ai/hermes/device/skills/5/download"},
		{ID: 5, Name: "b", Version: "1.0.0", SHA256: strings.Repeat("a", 64), SourceURL: "/api/v1/ai/hermes/device/skills/5/download"},
	}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("duplicate skill ids must be rejected")
	}

	// 空 Skills 必须被省略（P1 payload 字节不变）
	plain := valid
	plain.Skills = nil
	payload, _ := json.Marshal(plain)
	if strings.Contains(string(payload), `"skills"`) {
		t.Fatalf("empty skills must be omitted: %s", payload)
	}
}

// M3 MCP 管理：validateMcpServers 拒绝不安全命令/URL/重复 id。
func TestDesiredStateVerifiesMcpServers(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signAndVerify := func(unsigned UnsignedDesiredState) error {
		payload, _ := json.Marshal(unsigned)
		signed := DesiredState{
			UnsignedDesiredState: unsigned,
			Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
		}
		return signed.Verify(base64.RawStdEncoding.EncodeToString(publicKey), now)
	}

	valid := UnsignedDesiredState{
		SchemaVersion: 1, Revision: 9, DesiredState: "active", LeaseExpiresAt: now.Add(time.Hour),
		Profile: "company_cluster", Toolsets: []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		McpServers: []DesiredStateMcpServer{{
			ID: 3, Name: "company-crm", Type: "sse", URL: "https://crm.internal/v1/sse",
			Headers: map[string]string{"Authorization": "Bearer ${MCP_COMPANY_CRM_API_KEY}"},
		}},
	}
	if err := signAndVerify(valid); err != nil {
		t.Fatalf("valid mcp servers must verify: %v", err)
	}

	// 禁用命令（bash）拒
	bad := valid
	bad.McpServers = []DesiredStateMcpServer{{ID: 3, Name: "x", Type: "stdio", Command: "bash", Args: []string{"-c", "rm -rf"}}}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("bash command must be rejected")
	}

	// 不安全 argv（shell 元字符）拒
	bad = valid
	bad.McpServers = []DesiredStateMcpServer{{ID: 3, Name: "x", Type: "stdio", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp;curl"}}}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("unsafe argv must be rejected")
	}

	// 非白名单包拒
	bad = valid
	bad.McpServers = []DesiredStateMcpServer{{ID: 3, Name: "x", Type: "stdio", Command: "npx", Args: []string{"-y", "random-package"}}}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("non-allowlisted package must be rejected")
	}

	// 非 loopback http 拒
	bad = valid
	bad.McpServers = []DesiredStateMcpServer{{ID: 3, Name: "x", Type: "http", URL: "http://evil.example.com/mcp"}}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("non-loopback http url must be rejected")
	}

	// 重复 id 拒
	bad = valid
	bad.McpServers = []DesiredStateMcpServer{
		{ID: 3, Name: "a", Type: "sse", URL: "https://a.invalid"},
		{ID: 3, Name: "b", Type: "sse", URL: "https://b.invalid"},
	}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("duplicate mcp ids must be rejected")
	}

	// 空 McpServers 必须被省略（P1 payload 字节不变）
	plain := valid
	plain.McpServers = nil
	payload, _ := json.Marshal(plain)
	if strings.Contains(string(payload), `"mcp_servers"`) {
		t.Fatalf("empty mcp_servers must be omitted: %s", payload)
	}
}

func TestDesiredStateVerifiesCron(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signAndVerify := func(unsigned UnsignedDesiredState) error {
		payload, _ := json.Marshal(unsigned)
		signed := DesiredState{
			UnsignedDesiredState: unsigned,
			Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
		}
		return signed.Verify(base64.RawStdEncoding.EncodeToString(publicKey), now)
	}

	valid := UnsignedDesiredState{
		SchemaVersion: 1, Revision: 9, DesiredState: "active", LeaseExpiresAt: now.Add(time.Hour),
		Profile: "company_cluster", Toolsets: []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		Cron: []DesiredStateCronJob{{
			ID: 5, Name: "daily-report", CronExpression: "0 9 * * 1-5", Prompt: "生成昨日工作日报", Enabled: true,
		}},
	}
	if err := signAndVerify(valid); err != nil {
		t.Fatalf("valid cron must verify: %v", err)
	}

	// 坏表达式拒
	bad := valid
	bad.Cron = []DesiredStateCronJob{{ID: 5, Name: "x", CronExpression: "60 9 * * *", Prompt: "p", Enabled: true}}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("bad cron expression must be rejected")
	}

	// enabled 任务缺 prompt 拒
	bad = valid
	bad.Cron = []DesiredStateCronJob{{ID: 5, Name: "x", CronExpression: "0 9 * * 1-5", Prompt: "", Enabled: true}}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("enabled cron without prompt must be rejected")
	}

	// 重复 ID 拒
	bad = valid
	bad.Cron = []DesiredStateCronJob{
		{ID: 5, Name: "a", CronExpression: "0 9 * * 1-5", Prompt: "p", Enabled: true},
		{ID: 5, Name: "b", CronExpression: "0 9 * * 1-5", Prompt: "q", Enabled: true},
	}
	if err := signAndVerify(bad); err == nil {
		t.Fatal("duplicate cron job id must be rejected")
	}

	// 空 cron 省略（P1 payload 字节稳定）
	empty := valid
	empty.Cron = nil
	payload, _ := json.Marshal(empty)
	if strings.Contains(string(payload), `"cron"`) {
		t.Fatalf("empty cron must be omitted: %s", payload)
	}
}
