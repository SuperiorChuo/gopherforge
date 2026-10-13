package service

import (
	"context"
	"fmt"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"net/url"
	"regexp"
	"strings"
)

func (s *HermesService) ListArtifacts(ctx context.Context, page, pageSize int, approvedOnly bool) ([]localmodel.HermesArtifact, int64, error) {
	page, pageSize = normalizeHermesPage(page, pageSize)
	return s.dao.ListArtifacts(ctx, page, pageSize, approvedOnly)
}

func (s *HermesService) ApproveArtifact(ctx context.Context, artifactID uint64, actorID uint) error {
	if artifactID == 0 || actorID == 0 {
		return fmt.Errorf("%w: invalid artifact approval", ErrHermesInvalidInput)
	}
	return s.dao.ApproveArtifact(ctx, artifactID, actorID, s.now().UTC())
}

// RejectArtifact 把待审制品打回。只允许 pending -> rejected。
func (s *HermesService) RejectArtifact(ctx context.Context, artifactID uint64, actorID uint) error {
	if artifactID == 0 || actorID == 0 {
		return fmt.Errorf("%w: invalid artifact rejection", ErrHermesInvalidInput)
	}
	return s.dao.SetArtifactStatus(ctx, artifactID,
		localmodel.HermesArtifactPending, localmodel.HermesArtifactRejected, actorID, s.now().UTC())
}

// RevokeArtifact 撤回已批准的制品。已下发的策略仍持有该 artifact_id，
// 因此吊销只阻止后续引用与下载，不追溯改写历史策略。
func (s *HermesService) RevokeArtifact(ctx context.Context, artifactID uint64, actorID uint) error {
	if artifactID == 0 || actorID == 0 {
		return fmt.Errorf("%w: invalid artifact revocation", ErrHermesInvalidInput)
	}
	return s.dao.SetArtifactStatus(ctx, artifactID,
		localmodel.HermesArtifactApproved, localmodel.HermesArtifactRevoked, actorID, s.now().UTC())
}

// CreateHermesArtifactRequest 是制品录入载荷。字段对应 docs 第 9.2 节要求
// 「发布前必须固定：上游 tag、对应 commit、下载来源、SHA-256、审核状态」。
type CreateHermesArtifactRequest struct {
	Kind           string `json:"kind"`
	Platform       string `json:"platform"`
	Arch           string `json:"arch"`
	Version        string `json:"version"`
	UpstreamCommit string `json:"upstream_commit"`
	SourceURL      string `json:"source_url"`
	SHA256         string `json:"sha256"`
	Signature      string `json:"signature"`
}

var (
	hermesArtifactKinds          = map[string]bool{"manager": true, "hermes": true}
	hermesArtifactPlatforms      = map[string]bool{"windows": true, "darwin": true, "linux": true, "any": true}
	hermesArtifactArches         = map[string]bool{"amd64": true, "arm64": true, "any": true}
	hermesSHA256Pattern          = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hermesCommitPattern          = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
	hermesArtifactVersionPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._+-]{0,38}[A-Za-z0-9])?$`)
	// 移动引用不是不可变版本。文档 9.2：不得让终端跟随移动分支。
	hermesMovingRefs = map[string]bool{"latest": true, "main": true, "master": true, "head": true, "dev": true}
)

func (s *HermesService) CreateArtifact(ctx context.Context, req CreateHermesArtifactRequest) (*localmodel.HermesArtifact, error) {
	req.Kind = strings.ToLower(strings.TrimSpace(req.Kind))
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))
	req.Arch = strings.ToLower(strings.TrimSpace(req.Arch))
	req.Version = strings.TrimSpace(req.Version)
	req.UpstreamCommit = strings.ToLower(strings.TrimSpace(req.UpstreamCommit))
	req.SourceURL = strings.TrimSpace(req.SourceURL)
	// char(64) 会对短值右填充空格，摘要必须先规范化再校验
	req.SHA256 = strings.ToLower(strings.TrimSpace(req.SHA256))
	req.Signature = strings.TrimSpace(req.Signature)

	if !hermesArtifactKinds[req.Kind] {
		return nil, fmt.Errorf("%w: artifact kind must be manager or hermes", ErrHermesInvalidInput)
	}
	if !hermesArtifactPlatforms[req.Platform] {
		return nil, fmt.Errorf("%w: unsupported artifact platform", ErrHermesInvalidInput)
	}
	if !hermesArtifactArches[req.Arch] {
		return nil, fmt.Errorf("%w: unsupported artifact arch", ErrHermesInvalidInput)
	}
	if !hermesArtifactVersionPattern.MatchString(req.Version) {
		return nil, fmt.Errorf("%w: artifact version must be a safe 1-40 character release identifier", ErrHermesInvalidInput)
	}
	if hermesMovingRefs[strings.ToLower(req.Version)] {
		return nil, fmt.Errorf("%w: artifact version must be an immutable tag, not a moving ref", ErrHermesInvalidInput)
	}
	if !hermesSHA256Pattern.MatchString(req.SHA256) {
		return nil, fmt.Errorf("%w: sha256 must be 64 lowercase hex chars", ErrHermesInvalidInput)
	}
	if err := validateArtifactSourceURL(req.SourceURL); err != nil {
		return nil, err
	}
	// Hermes 本体必须钉住上游 commit；manager 由本仓构建，允许留空
	if req.Kind == "hermes" && !hermesCommitPattern.MatchString(req.UpstreamCommit) {
		return nil, fmt.Errorf("%w: hermes artifact requires an upstream commit sha", ErrHermesInvalidInput)
	}
	if req.UpstreamCommit != "" && !hermesCommitPattern.MatchString(req.UpstreamCommit) {
		return nil, fmt.Errorf("%w: upstream commit must be a hex sha", ErrHermesInvalidInput)
	}

	artifact := &localmodel.HermesArtifact{
		Kind:           req.Kind,
		Platform:       req.Platform,
		Arch:           req.Arch,
		Version:        req.Version,
		UpstreamCommit: req.UpstreamCommit,
		SourceURL:      req.SourceURL,
		SHA256:         req.SHA256,
		Signature:      req.Signature,
		Status:         localmodel.HermesArtifactPending,
	}
	if err := s.dao.CreateArtifact(ctx, artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

func validateArtifactSourceURL(raw string) error {
	if raw == "" || len(raw) > 1024 {
		return fmt.Errorf("%w: artifact source url is required", ErrHermesInvalidInput)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: artifact source url is malformed", ErrHermesInvalidInput)
	}
	// 只允许 https：制品会被下载到员工电脑执行，明文信道不可接受
	if parsed.Scheme != "https" {
		return fmt.Errorf("%w: artifact source url must use https", ErrHermesInvalidInput)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%w: artifact source url must include a host", ErrHermesInvalidInput)
	}
	return nil
}
