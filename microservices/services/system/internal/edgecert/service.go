package edgecert

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Service 边缘证书业务。
type Service struct {
	DB *gorm.DB
}

func (s *Service) List(ctx context.Context) ([]PublicView, error) {
	var rows []Certificate
	if err := s.DB.WithContext(ctx).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := make([]PublicView, 0, len(rows))
	for i := range rows {
		if rows[i].Status == StatusIssued && rows[i].NotAfter != nil && rows[i].NotAfter.Before(now) {
			rows[i].Status = StatusExpired
			_ = s.DB.WithContext(ctx).Model(&rows[i]).Update("status", StatusExpired).Error
		}
		out = append(out, rows[i].ToPublic())
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id uint64) (*PublicView, error) {
	var row Certificate
	if err := s.DB.WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	v := row.ToPublic()
	return &v, nil
}

type UpsertInput struct {
	Domain    string `json:"domain"`
	Email     string `json:"email"`
	IsStaging bool   `json:"is_staging"`
}

func (s *Service) UpsertDraft(ctx context.Context, in UpsertInput) (*PublicView, error) {
	domain := strings.TrimSpace(strings.ToLower(in.Domain))
	email := strings.TrimSpace(in.Email)
	if domain == "" || email == "" {
		return nil, fmt.Errorf("domain and email required")
	}
	var row Certificate
	err := s.DB.WithContext(ctx).Where("domain = ?", domain).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		row = Certificate{
			Domain: domain, Email: email, Status: StatusDraft,
			Provider: "letsencrypt", IsStaging: in.IsStaging,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else {
		row.Email = email
		row.IsStaging = in.IsStaging
		row.UpdatedAt = time.Now().UTC()
		if row.Status == StatusFailed {
			row.Status = StatusDraft
			row.LastError = ""
		}
		if err := s.DB.WithContext(ctx).Save(&row).Error; err != nil {
			return nil, err
		}
	}
	v := row.ToPublic()
	return &v, nil
}

// Issue 同步签发（可能数十秒）；调用方宜在 HTTP 层放宽超时。
func (s *Service) Issue(ctx context.Context, id uint64) (*PublicView, error) {
	var row Certificate
	if err := s.DB.WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	row.Status = StatusPending
	row.LastError = ""
	row.UpdatedAt = time.Now().UTC()
	_ = s.DB.WithContext(ctx).Save(&row).Error

	issueCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if err := Issue(issueCtx, &row); err != nil {
		row.Status = StatusFailed
		row.LastError = err.Error()
		row.UpdatedAt = time.Now().UTC()
		_ = s.DB.WithContext(ctx).Save(&row).Error
		return nil, err
	}
	if err := s.DB.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	v := row.ToPublic()
	return &v, nil
}

func (s *Service) Delete(ctx context.Context, id uint64) error {
	return s.DB.WithContext(ctx).Delete(&Certificate{}, id).Error
}

// Download 返回 fullchain + key（仅 issue 权限）。
func (s *Service) Download(ctx context.Context, id uint64) (fullchain, key string, domain string, err error) {
	var row Certificate
	if err = s.DB.WithContext(ctx).First(&row, id).Error; err != nil {
		return "", "", "", err
	}
	if row.FullchainPEM == "" || row.PrivateKeyPEM == "" {
		return "", "", "", fmt.Errorf("certificate not issued")
	}
	return row.FullchainPEM, row.PrivateKeyPEM, row.Domain, nil
}
