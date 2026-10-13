package service

import (
	"context"
	"testing"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
)

// M3 MCP 管理：白名单校验、CRUD、集群绑定、buildDesiredState 携带 McpServers。

func createHermesTestMcpServer(t *testing.T, svc *HermesService, ctx context.Context, name string) *localmodel.HermesMcpServer {
	t.Helper()
	server, err := svc.CreateMcpServer(ctx, 1, CreateHermesMcpServerRequest{
		Name:        name,
		DisplayName: name + " display",
		Description: "测试 MCP",
		Type:        "sse",
		URL:         "https://crm.internal/v1/sse",
		Headers:     map[string]string{"Authorization": "Bearer ${MCP_" + name + "_API_KEY}"},
		SecretJSON:  map[string]string{"MCP_" + name + "_API_KEY": "sk-test"},
	})
	if err != nil {
		t.Fatalf("create mcp server %s: %v", name, err)
	}
	return server
}

func TestCreateMcpServerValidatesWhitelist(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)

	// 禁用命令（bash -c）拒
	if _, err := svc.CreateMcpServer(ctx, 1, CreateHermesMcpServerRequest{Name: "x", Type: "stdio", Command: "bash", Args: []string{"-c", "rm"}}); err == nil {
		t.Fatal("bash command must be rejected")
	}
	// 非白名单包拒
	if _, err := svc.CreateMcpServer(ctx, 1, CreateHermesMcpServerRequest{Name: "x", Type: "stdio", Command: "npx", Args: []string{"-y", "random-package"}}); err == nil {
		t.Fatal("non-allowlisted package must be rejected")
	}
	// 非 loopback http 拒
	if _, err := svc.CreateMcpServer(ctx, 1, CreateHermesMcpServerRequest{Name: "x", Type: "http", URL: "http://evil.example.com/mcp"}); err == nil {
		t.Fatal("non-loopback http url must be rejected")
	}
	// 未声明密钥的 ${VAR} 拒
	if _, err := svc.CreateMcpServer(ctx, 1, CreateHermesMcpServerRequest{
		Name: "x", Type: "sse", URL: "https://ok.example.com/mcp",
		Headers: map[string]string{"Authorization": "Bearer ${MCP_UNDECLARED_KEY}"},
	}); err == nil {
		t.Fatal("undeclared secret var must be rejected")
	}

	server := createHermesTestMcpServer(t, svc, ctx, "company-crm")
	if server.Status != localmodel.HermesMcpPending {
		t.Fatalf("new server status = %q, want pending", server.Status)
	}
	if server.SecretJSON != nil {
		t.Fatal("create response must not include secret_json")
	}
	// 审批
	if err := svc.ApproveMcpServer(ctx, 1, server.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// 同名校重复拒
	if _, err := svc.CreateMcpServer(ctx, 1, CreateHermesMcpServerRequest{Name: "company-crm", Type: "sse", URL: "https://crm.internal/v1/sse"}); err == nil {
		t.Fatal("duplicate mcp server name must be rejected")
	}
}

func TestBindClusterMcpServerRequiresApproved(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestCluster(t, svc, ctx, "mcp-cluster", clusterPolicy.ID)

	pending := createHermesTestMcpServer(t, svc, ctx, "pending-mcp")
	if err := svc.BindClusterMcpServer(ctx, 1, cluster.ID, pending.ID); err == nil {
		t.Fatal("binding a pending mcp server must be rejected")
	}

	approved := createHermesTestMcpServer(t, svc, ctx, "approved-mcp")
	if err := svc.ApproveMcpServer(ctx, 1, approved.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := svc.BindClusterMcpServer(ctx, 1, cluster.ID, approved.ID); err != nil {
		t.Fatalf("bind approved server: %v", err)
	}
	if err := svc.BindClusterMcpServer(ctx, 1, cluster.ID, approved.ID); err == nil {
		t.Fatal("duplicate bind must be rejected")
	}
}

func TestBuildDesiredStateCarriesApprovedClusterMcpServers(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, companyPolicy.ID)

	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestCluster(t, svc, ctx, "mcp-cluster", clusterPolicy.ID)

	approved := createHermesTestMcpServer(t, svc, ctx, "company-crm")
	if err := svc.ApproveMcpServer(ctx, 1, approved.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := svc.BindClusterMcpServer(ctx, 1, cluster.ID, approved.ID); err != nil {
		t.Fatalf("bind: %v", err)
	}

	device := enrollHermesTestDevice(t, svc, store, ctx)
	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "lead", Role: "lead", OwnerUserID: 1, DeviceID: device.DeviceID, ClusterID: cluster.ID, Enabled: true,
	}); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	state, err := svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state: %v", err)
	}
	if len(state.McpServers) != 1 || state.McpServers[0].Name != "company-crm" {
		t.Fatalf("desired state mcp = %+v, want [company-crm]", state.McpServers)
	}
	if state.McpServers[0].Headers["Authorization"] != "Bearer ${MCP_company-crm_API_KEY}" {
		t.Fatalf("mcp headers should carry ${VAR} template, got %v", state.McpServers[0].Headers)
	}
}
