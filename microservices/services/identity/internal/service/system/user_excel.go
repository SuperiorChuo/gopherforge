package system

// 用户导出 / 批量导入的服务层（路线图第 11 项「通用 Excel 导入导出」的
// 首个接入点）。xlsx 编解码在 API 层经 shared/pkg/excel 完成，本层只管
// 数据拉取与逐行落库。

import (
	"context"
	"errors"

	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
)

const (
	userExportPageSize = 500
	// UserExportCap 导出行数上限（防御大表拖垮服务；超限截断并告知）。
	UserExportCap = 10000
	// UserImportMaxRows 单次导入行数上限。
	UserImportMaxRows = 1000
	// UserImportDefaultPassword 导入未填密码时的初始密码（模板中注明）。
	UserImportDefaultPassword = "Init#12345"
)

// ExportUsersContext 按列表条件分页循环全量拉取（上限 UserExportCap）。
// truncated=true 表示命中上限被截断。
func (s *UserService) ExportUsersContext(ctx context.Context, req UserListRequest) (users []model.User, truncated bool, err error) {
	page := 1
	for {
		pr := pagination.PageRequest{Page: page, PageSize: userExportPageSize}
		batch, total, err := s.userDAO.GetUserListContext(ctx, pr, req.Keyword, req.Status, req.DataScope)
		if err != nil {
			return nil, false, err
		}
		users = append(users, batch...)
		if len(users) >= UserExportCap {
			return users[:UserExportCap], int64(UserExportCap) < total, nil
		}
		if int64(len(users)) >= total || len(batch) == 0 {
			return users, false, nil
		}
		page++
	}
}

// DepartmentNameMapContext 当前租户的部门 id→名称映射（导出列 / 导入反解共用）。
func (s *UserService) DepartmentNameMapContext(ctx context.Context) (map[uint]string, error) {
	if s.deptDAO == nil {
		return map[uint]string{}, nil
	}
	depts, err := s.deptDAO.GetAllContext(ctx, nil)
	if err != nil {
		return nil, err
	}
	m := make(map[uint]string, len(depts))
	for _, d := range depts {
		m[d.ID] = d.Name
	}
	return m, nil
}

// ImportUserRow 导入的一行（Row 为 Excel 中的行号，从 2 起：1 是表头）。
type ImportUserRow struct {
	Row int
	Req CreateUserRequest
}

// ImportRowError 单行导入失败明细（前端逐行展示）。
type ImportRowError struct {
	Row      int    `json:"row"`
	Username string `json:"username"`
	Reason   string `json:"reason"`
}

// ImportUsersContext 逐行创建用户；单行失败不中断其余行（部分成功语义，
// 与 yudao 导入行为一致），失败明细逐行返回。
func (s *UserService) ImportUsersContext(ctx context.Context, rows []ImportUserRow) (success int, failures []ImportRowError) {
	for _, r := range rows {
		if _, err := s.CreateUserContext(ctx, r.Req); err != nil {
			failures = append(failures, ImportRowError{
				Row:      r.Row,
				Username: r.Req.Username,
				Reason:   importErrorText(err),
			})
			continue
		}
		success++
	}
	return success, failures
}

// importErrorText 把服务层错误翻译成导入明细可读文案。
func importErrorText(err error) string {
	switch {
	case errors.Is(err, ErrUsernameAlreadyExists):
		return "用户名已存在"
	case errors.Is(err, ErrEmailAlreadyExists):
		return "邮箱已被使用"
	case errors.Is(err, ErrDepartmentNotInTenant):
		return "部门不存在或不属于当前租户"
	case errors.Is(err, ErrTenantUserQuota):
		return "租户用户配额已满"
	default:
		return err.Error()
	}
}
