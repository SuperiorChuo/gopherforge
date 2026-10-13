package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sort"
	"testing"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
)

// M2 技能市场：技能 CRUD + 审批 + 集群绑定 + buildDesiredState 携带 Skills。

// buildHermesTestSkillTarGz 构造合法技能包：<name>/SKILL.md（含 frontmatter）。
func buildHermesTestSkillTarGz(t *testing.T, name string, extra map[string]string) []byte {
	t.Helper()
	skillMD := "---\nname: " + name + "\ndescription: 测试技能包\n---\nProcedure\n"
	files := map[string]string{name + "/SKILL.md": skillMD}
	for p, content := range extra {
		files[name+"/"+p] = content
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		content := files[p]
		if err := tw.WriteHeader(&tar.Header{Name: p, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func createHermesTestSkill(t *testing.T, svc *HermesService, ctx context.Context, name, version string) *localmodel.HermesSkill {
	t.Helper()
	skill, err := svc.CreateSkill(ctx, 1, CreateHermesSkillRequest{
		Name: name, DisplayName: name + " display", Description: "测试技能", Version: version,
	}, buildHermesTestSkillTarGz(t, name, nil))
	if err != nil {
		t.Fatalf("create skill %s: %v", name, err)
	}
	return skill
}

func TestCreateSkillValidatesAndApproves(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)

	// 非法技能名（含空格/路径分隔符）拒
	if _, err := svc.CreateSkill(ctx, 1, CreateHermesSkillRequest{Name: "bad name", Version: "1.0.0"}, []byte("x")); err == nil {
		t.Fatal("skill name with space must be rejected")
	}
	if _, err := svc.CreateSkill(ctx, 1, CreateHermesSkillRequest{Name: "../escape", Version: "1.0.0"}, []byte("x")); err == nil {
		t.Fatal("skill name with path traversal must be rejected")
	}
	// 移动版本引用拒
	if _, err := svc.CreateSkill(ctx, 1, CreateHermesSkillRequest{Name: "skill", Version: "latest"}, []byte("x")); err == nil {
		t.Fatal("moving version must be rejected")
	}
	// 空 content 拒
	if _, err := svc.CreateSkill(ctx, 1, CreateHermesSkillRequest{Name: "skill", Version: "1.0.0"}, nil); err == nil {
		t.Fatal("empty content must be rejected")
	}

	skill := createHermesTestSkill(t, svc, ctx, "weekly-report", "1.0.0")
	if skill.Status != localmodel.HermesSkillPending {
		t.Fatalf("new skill status = %q, want pending", skill.Status)
	}
	if len(skill.SHA256) != 64 {
		t.Fatalf("sha256 length = %d, want 64", len(skill.SHA256))
	}
	// content 不应回给创建响应
	if skill.Content != nil {
		t.Fatal("create response must not include content")
	}

	// 审批
	if err := svc.ApproveSkill(ctx, 1, skill.ID); err != nil {
		t.Fatalf("approve skill: %v", err)
	}
	// 同 (name, version) 重复录入拒
	if _, err := svc.CreateSkill(ctx, 1, CreateHermesSkillRequest{Name: "weekly-report", Version: "1.0.0"}, []byte("y")); err == nil {
		t.Fatal("duplicate (name, version) must be rejected")
	}
	// 吊销已批准技能
	if err := svc.RevokeSkill(ctx, 1, skill.ID); err != nil {
		t.Fatalf("revoke skill: %v", err)
	}
}

func TestBindClusterSkillRequiresApproved(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestCluster(t, svc, ctx, "skill-cluster", clusterPolicy.ID)

	pending := createHermesTestSkill(t, svc, ctx, "pending-skill", "1.0.0")
	// 未审批不能绑
	if err := svc.BindClusterSkill(ctx, 1, cluster.ID, pending.ID); err == nil {
		t.Fatal("binding a pending skill must be rejected")
	}

	approved := createHermesTestSkill(t, svc, ctx, "approved-skill", "1.0.0")
	if err := svc.ApproveSkill(ctx, 1, approved.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := svc.BindClusterSkill(ctx, 1, cluster.ID, approved.ID); err != nil {
		t.Fatalf("bind approved skill: %v", err)
	}
	// 重复绑拒
	if err := svc.BindClusterSkill(ctx, 1, cluster.ID, approved.ID); err == nil {
		t.Fatal("duplicate bind must be rejected")
	}
	bound, err := svc.ListClusterSkills(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("list cluster skills: %v", err)
	}
	if len(bound) != 1 || bound[0].ID != approved.ID {
		t.Fatalf("bound skills = %+v, want [approved-skill]", bound)
	}
}

// 集群内设备 desired-state 携带已审批+绑定技能；非集群设备省略。
func TestBuildDesiredStateCarriesApprovedClusterSkills(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, companyPolicy.ID)

	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestCluster(t, svc, ctx, "skills-cluster", clusterPolicy.ID)

	// 已审批并绑定
	approved := createHermesTestSkill(t, svc, ctx, "weekly-report", "1.0.0")
	if err := svc.ApproveSkill(ctx, 1, approved.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := svc.BindClusterSkill(ctx, 1, cluster.ID, approved.ID); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// 集群内设备
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
	if len(state.Skills) != 1 || state.Skills[0].Name != "weekly-report" {
		t.Fatalf("desired state skills = %+v, want [weekly-report]", state.Skills)
	}
}

// M7 供应链验签：信任根配置后，带有效签名 → verified、无效 → 拒、空 → unverified；
// 未配置信任根 → 降级不验签（现行为不变）。
func TestCreateSkillSignatureVerification(t *testing.T) {
	// 生成测试信任根（私钥签 sha256，公钥验）
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	publicKey := append(ed25519.PublicKey(nil), privateKey.Public().(ed25519.PublicKey)...)

	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	svc.skillVerifier = publicKey

	content := buildHermesTestSkillTarGz(t, "sig-skill", nil)
	sha := sha256.Sum256(content)
	shaHex := hex.EncodeToString(sha[:])
	sig := base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(shaHex)))

	// 有效签名 → verified
	skill, err := svc.CreateSkill(ctx, 1, CreateHermesSkillRequest{
		Name: "sig-skill", Version: "1.0.0", Signature: sig,
	}, content)
	if err != nil {
		t.Fatalf("signed skill: %v", err)
	}
	if skill.SignatureStatus != localmodel.HermesSkillSignatureVerified {
		t.Fatalf("signature_status = %q, want verified", skill.SignatureStatus)
	}

	// 无效签名 → 拒
	if _, err := svc.CreateSkill(ctx, 1, CreateHermesSkillRequest{
		Name: "bad-sig", Version: "1.0.0", Signature: base64.RawStdEncoding.EncodeToString([]byte("not-a-real-signature")),
	}, content); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("invalid signature error = %v, want invalid input", err)
	}

	// 空签名 → unverified
	skill2, err := svc.CreateSkill(ctx, 1, CreateHermesSkillRequest{Name: "no-sig", Version: "1.0.0"}, content)
	if err != nil {
		t.Fatalf("unsigned skill: %v", err)
	}
	if skill2.SignatureStatus != localmodel.HermesSkillSignatureUnverified {
		t.Fatalf("signature_status = %q, want unverified", skill2.SignatureStatus)
	}

	// 未配置信任根 → 降级不验签（带签名也落库，不拒）
	svc.skillVerifier = nil
	skill3, err := svc.CreateSkill(ctx, 1, CreateHermesSkillRequest{Name: "no-key", Version: "1.0.0", Signature: sig}, content)
	if err != nil {
		t.Fatalf("skill without verifier must not be rejected: %v", err)
	}
	if skill3.SignatureStatus != localmodel.HermesSkillSignatureUnverified {
		t.Fatalf("no-verifier signature_status = %q, want unverified", skill3.SignatureStatus)
	}
}
