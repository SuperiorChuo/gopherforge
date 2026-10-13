package service

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

// Hermes M2：技能市场。控制面管理技能（上传/审批/版本），经签名 desired-state
// 下发技能清单，manager 下载 tarball → sha256 校验 → 直解压进员工 profile 的
// skills 树。技能绑定在集群层（按集群/策略粒度决策）。

const (
	// maxHermesSkillContentBytes 是技能 tarball 上限（知识文档，非二进制程序）。
	maxHermesSkillContentBytes = 16 << 20 // 16 MB
)

// hermesSkillNamePattern 是技能名白名单：会成为 profile skills 目录名，必须
// 文件系统安全（小写字母数字 + . _ -）。与 manager 侧 validateSkills 一致。
var hermesSkillNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,79}$`)

// hermesSkillVersionPattern 复用 artifact 版本规则：不可变 tag。
var hermesSkillVersionPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._+-]{0,38}[A-Za-z0-9])?$`)

// hermesSkillMovingRefs 复用 artifact 的移动引用拒绝集。
var hermesSkillMovingRefs = map[string]bool{"latest": true, "main": true, "master": true, "head": true, "dev": true}

type CreateHermesSkillRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Signature   string `json:"signature"`
}

// CreateSkill 录入技能（pending）。content 由上传 handler 提供（multipart）。
func (s *HermesService) CreateSkill(ctx context.Context, actorID uint, request CreateHermesSkillRequest, content []byte) (*localmodel.HermesSkill, error) {
	if actorID == 0 {
		return nil, fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	request.Name = strings.ToLower(strings.TrimSpace(request.Name))
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.Description = strings.TrimSpace(request.Description)
	request.Version = strings.TrimSpace(request.Version)
	request.Signature = strings.TrimSpace(request.Signature)

	if !hermesSkillNamePattern.MatchString(request.Name) {
		return nil, fmt.Errorf("%w: invalid skill name", ErrHermesInvalidInput)
	}
	if len(request.DisplayName) > 120 {
		return nil, fmt.Errorf("%w: display name too long", ErrHermesInvalidInput)
	}
	if len(request.Description) > maxHermesTaskDescriptionLen {
		return nil, fmt.Errorf("%w: description too long", ErrHermesInvalidInput)
	}
	if !hermesSkillVersionPattern.MatchString(request.Version) {
		return nil, fmt.Errorf("%w: skill version must be a safe release tag", ErrHermesInvalidInput)
	}
	if hermesSkillMovingRefs[strings.ToLower(request.Version)] {
		return nil, fmt.Errorf("%w: skill version must be immutable, not a moving ref", ErrHermesInvalidInput)
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("%w: skill content is required", ErrHermesInvalidInput)
	}
	if len(content) > maxHermesSkillContentBytes {
		return nil, fmt.Errorf("%w: skill content exceeds %d bytes", ErrHermesInvalidInput, maxHermesSkillContentBytes)
	}
	// 包格式校验（上传与官方市场导入共用）：tar(.gz) 可解、SKILL.md 存在、
	// frontmatter 合法。此前上传路径零校验，审批人看不见包内容——这里收口。
	if _, err := parseSkillArchiveMeta(content); err != nil {
		return nil, err
	}

	sum := sha256.Sum256(content)
	sha256Hex := hex.EncodeToString(sum[:])
	signatureStatus := localmodel.HermesSkillSignatureUnverified
	if len(request.Signature) > 0 {
		// 供应链验签：签名对象 = sha256 hex（canonical 短串，与下游 sha256 链路衔接）。
		// 未配置信任根时降级不验签（同 policy signer 语义）。
		if s.skillVerifier != nil {
			decoded, err := decodeHermesBase64(request.Signature)
			if err != nil || len(decoded) != ed25519.SignatureSize {
				return nil, fmt.Errorf("%w: skill signature is not valid base64 Ed25519", ErrHermesInvalidInput)
			}
			if !ed25519.Verify(s.skillVerifier, []byte(sha256Hex), decoded) {
				return nil, fmt.Errorf("%w: skill signature does not match content hash", ErrHermesInvalidInput)
			}
			signatureStatus = localmodel.HermesSkillSignatureVerified
		}
	}
	skill := &localmodel.HermesSkill{
		TenantID:        tenant.FromContextOrDefault(ctx),
		Name:            request.Name,
		DisplayName:     request.DisplayName,
		Description:     request.Description,
		Version:         request.Version,
		SHA256:          sha256Hex,
		Signature:       request.Signature,
		SignatureStatus: signatureStatus,
		Content:         content,
		Status:          localmodel.HermesSkillPending,
		CreatedAt:       s.now().UTC(),
		UpdatedAt:       s.now().UTC(),
	}
	if err := s.dao.CreateSkill(ctx, skill); err != nil {
		return nil, err
	}
	skill.Content = nil // 不把 bytea 回给创建响应
	return skill, nil
}

func (s *HermesService) ListSkills(ctx context.Context, page, pageSize int) ([]localmodel.HermesSkill, int64, error) {
	page, pageSize = normalizeHermesPage(page, pageSize)
	return s.dao.ListSkills(ctx, page, pageSize)
}

func (s *HermesService) ApproveSkill(ctx context.Context, actorID uint, id uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	return s.dao.SetSkillStatus(ctx, id, localmodel.HermesSkillPending, localmodel.HermesSkillApproved, actorID, s.now().UTC())
}

func (s *HermesService) RejectSkill(ctx context.Context, actorID uint, id uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	return s.dao.SetSkillStatus(ctx, id, localmodel.HermesSkillPending, localmodel.HermesSkillRejected, actorID, s.now().UTC())
}

func (s *HermesService) RevokeSkill(ctx context.Context, actorID uint, id uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	return s.dao.SetSkillStatus(ctx, id, localmodel.HermesSkillApproved, localmodel.HermesSkillRevoked, actorID, s.now().UTC())
}

// BindClusterSkill 把已审批技能绑定到集群。集群须存在且启用，技能须已审批。
func (s *HermesService) BindClusterSkill(ctx context.Context, actorID uint, clusterID, skillID uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	cluster, err := s.dao.GetCluster(ctx, clusterID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: cluster not found", ErrHermesInvalidInput)
		}
		return err
	}
	if !cluster.Enabled {
		return fmt.Errorf("%w: cluster is disabled", ErrHermesInvalidInput)
	}
	skill, err := s.dao.GetSkill(ctx, skillID)
	if err != nil {
		return err
	}
	if skill.Status != localmodel.HermesSkillApproved {
		return fmt.Errorf("%w: skill is not approved", ErrHermesInvalidInput)
	}
	return s.dao.BindSkillToCluster(ctx, clusterID, skillID, actorID, s.now().UTC())
}

func (s *HermesService) UnbindClusterSkill(ctx context.Context, actorID uint, clusterID, skillID uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	return s.dao.UnbindSkillFromCluster(ctx, clusterID, skillID)
}

func (s *HermesService) ListClusterSkills(ctx context.Context, clusterID uint64) ([]localmodel.HermesSkill, error) {
	return s.dao.ListClusterSkills(ctx, clusterID)
}

// GetSkillContent 取技能 tarball（下载端点，服务端只放行已审批）。
func (s *HermesService) GetSkillContent(ctx context.Context, id uint64) ([]byte, error) {
	return s.dao.GetSkillContent(ctx, id)
}

// buildDesiredStateSkills 载入集群绑定的已审批技能，构造成签名载荷的 Skills。
func (s *HermesService) buildDesiredStateSkills(ctx context.Context, clusterID uint64) ([]HermesDesiredStateSkill, error) {
	skills, err := s.dao.ListApprovedSkillsForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, nil
	}
	out := make([]HermesDesiredStateSkill, 0, len(skills))
	for _, skill := range skills {
		out = append(out, HermesDesiredStateSkill{
			ID:        skill.ID,
			Name:      skill.Name,
			Version:   skill.Version,
			SHA256:    skill.SHA256,
			SourceURL: fmt.Sprintf("/api/v1/ai/hermes/device/skills/%d/download", skill.ID),
		})
	}
	return out, nil
}

var _ = dao.ErrHermesSkillDuplicate
