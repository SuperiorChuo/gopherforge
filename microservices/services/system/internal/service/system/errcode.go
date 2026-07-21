package system

import (
	"context"
	"errors"
	"strings"
	"time"

	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	"github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/system/internal/pkg/pagination"
	"github.com/go-admin-kit/services/system/internal/pkg/runtimeconfig"
	"gorm.io/gorm"
)

// ErrCodeService 错误码管理业务逻辑：CRUD + 写后即时刷新本进程文案缓存。
// 其他实例/服务依赖 30s TTL 缓存自然过期热生效。
type ErrCodeService struct {
	errCodeDAO  systemdao.ErrCodeDAO
	invalidator runtimeconfig.ErrorCodeInvalidator
}

// NewErrCodeServiceWithDB 用注入的数据库句柄构建 ErrCodeService。
func NewErrCodeServiceWithDB(db *gorm.DB) ErrCodeService {
	return ErrCodeService{errCodeDAO: *systemdao.NewErrCodeDAO(db)}
}

type ErrCodeListRequest struct {
	pagination.PageRequest
	Keyword string `form:"keyword" json:"keyword"`
	Scope   string `form:"scope" json:"scope"`
	Status  *int8  `form:"status" json:"status"`
}

type CreateErrCodeRequest struct {
	Code    string `json:"code" binding:"required"`
	Message string `json:"message" binding:"required"`
	Memo    string `json:"memo"`
	Scope   string `json:"scope"`
	Status  int8   `json:"status"`
}

type UpdateErrCodeRequest struct {
	Message string  `json:"message"`
	Memo    *string `json:"memo"`
	Scope   string  `json:"scope"`
	Status  *int8   `json:"status"`
}

var (
	ErrErrorCodeAlreadyExists = errors.New("error code already exists")
	ErrErrorCodeNotFound      = errors.New("error code not found")
	ErrErrorCodeCodeRequired  = errors.New("error code identifier is required")
)

const errCodeCacheRefreshTimeout = 2 * time.Second

func (s *ErrCodeService) CreateContext(ctx context.Context, req CreateErrCodeRequest) (*model.ErrorCode, error) {
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return nil, ErrErrorCodeCodeRequired
	}

	_, err := s.errCodeDAO.GetByCodeContext(ctx, code)
	if err == nil {
		return nil, ErrErrorCodeAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	errorCode := &model.ErrorCode{
		Code:    code,
		Message: req.Message,
		Memo:    req.Memo,
		Scope:   req.Scope,
		Status:  req.Status,
	}
	if errorCode.Scope == "" {
		errorCode.Scope = "global"
	}
	if errorCode.Status == 0 {
		errorCode.Status = 1
	}

	if err := s.errCodeDAO.CreateContext(ctx, errorCode); err != nil {
		return nil, err
	}

	s.refreshMessageCache(ctx)
	return errorCode, nil
}

func (s *ErrCodeService) GetByIDContext(ctx context.Context, id uint) (*model.ErrorCode, error) {
	errorCode, err := s.errCodeDAO.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrErrorCodeNotFound
		}
		return nil, err
	}
	return errorCode, nil
}

func (s *ErrCodeService) GetListContext(ctx context.Context, req ErrCodeListRequest) ([]model.ErrorCode, int64, error) {
	return s.errCodeDAO.GetListContext(ctx, req.PageRequest, req.Keyword, req.Scope, req.Status)
}

// GetAllEnabledContext 返回全量启用错误码（供服务/前端整包拉取）。
func (s *ErrCodeService) GetAllEnabledContext(ctx context.Context) ([]model.ErrorCode, error) {
	return s.errCodeDAO.GetAllEnabledContext(ctx)
}

func (s *ErrCodeService) UpdateContext(ctx context.Context, id uint, req UpdateErrCodeRequest) (*model.ErrorCode, error) {
	errorCode, err := s.errCodeDAO.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrErrorCodeNotFound
		}
		return nil, err
	}

	// code 是各服务代码引用的稳定标识，不允许改，只允许改文案/备注/来源/状态
	if req.Message != "" {
		errorCode.Message = req.Message
	}
	if req.Memo != nil {
		errorCode.Memo = *req.Memo
	}
	if req.Scope != "" {
		errorCode.Scope = req.Scope
	}
	if req.Status != nil {
		errorCode.Status = *req.Status
	}

	if err := s.errCodeDAO.UpdateContext(ctx, errorCode); err != nil {
		return nil, err
	}

	s.refreshMessageCache(ctx)
	return errorCode, nil
}

func (s *ErrCodeService) DeleteContext(ctx context.Context, id uint) error {
	if _, err := s.errCodeDAO.GetByIDContext(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrErrorCodeNotFound
		}
		return err
	}
	if err := s.errCodeDAO.DeleteContext(ctx, id); err != nil {
		return err
	}
	s.refreshMessageCache(ctx)
	return nil
}

// refreshMessageCache 写后尽力刷新本进程读取器缓存（失败不影响主流程，
// 最迟 30s TTL 到期后仍会热生效）。
func (s *ErrCodeService) refreshMessageCache(ctx context.Context) {
	invalidator := s.invalidator
	if invalidator == nil {
		invalidator = runtimeconfig.DefaultErrorCodeReader()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), errCodeCacheRefreshTimeout)
	defer cancel()
	_ = invalidator.Refresh(refreshCtx)
}
