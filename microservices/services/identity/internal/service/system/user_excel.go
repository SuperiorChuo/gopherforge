package system

// 用户导出 / 批量导入的服务层（路线图第 11 项「通用 Excel 导入导出」的
// 首个接入点）。xlsx 编解码在 API 层经 shared/pkg/excel 完成，本层只管
// 数据拉取与逐行落库。

import (
	"context"
	"errors"

	systemdao "github.com/go-admin-kit/services/identity/internal/dao/system"
	localmodel "github.com/go-admin-kit/services/identity/internal/model"
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

// StreamExportUsersContext 按列表条件游标翻页，每页回调一次（上限
// UserExportCap 行）。truncated=true 表示命中上限被截断。
//
// 三处与旧实现不同：
//   - Keyset cursor 取代 LIMIT/OFFSET 循环，第 20 页与第 1 页同价；
//   - 去掉每页一次的全表 COUNT，靠"短页即末页"判终止；
//   - 去掉 Preload("Roles")/Preload("Posts")（导出列里没有角色/岗位，见
//     api/system/user_excel.go 的表头）。
//
// 逐页回调而不是一次返回一万行：调用方把每页直接写进 excelize StreamWriter，
// 内存里最多只留一页。
func (s *UserService) StreamExportUsersContext(
	ctx context.Context,
	req UserListRequest,
	emit func([]localmodel.User) error,
) (truncated bool, err error) {
	var (
		cursor  systemdao.ExportUserCursor
		emitted int
	)
	for {
		// On the page that would reach the cap, ask for one row past it: that
		// probe is what distinguishes "exactly UserExportCap rows exist"
		// (nothing truncated) from "more rows remain" (truncated).
		limit := userExportPageSize
		remaining := UserExportCap - emitted
		if remaining <= limit {
			limit = remaining + 1
		}

		batch, err := s.userDAO.ExportUsersPageContext(ctx, cursor, limit, req.Keyword, req.Status, req.DataScope)
		if err != nil {
			return false, err
		}
		if len(batch) == 0 {
			return false, nil
		}

		over := emitted + len(batch) - UserExportCap
		if over > 0 {
			if err := emit(batch[:len(batch)-over]); err != nil {
				return false, err
			}
			return true, nil
		}
		if err := emit(batch); err != nil {
			return false, err
		}
		emitted += len(batch)

		if len(batch) < limit {
			// A short page is the last page — no more rows exist.
			return false, nil
		}
		if emitted == UserExportCap {
			// Filled the cap on a full page; the probe above proved nothing
			// follows, so this is a complete export.
			return false, nil
		}

		last := batch[len(batch)-1]
		cursor = systemdao.ExportUserCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
}

// ExportUsersContext 收集全部导出行到内存（保留给需要整份切片的调用方；
// HTTP 导出走 StreamExportUsersContext）。
func (s *UserService) ExportUsersContext(ctx context.Context, req UserListRequest) (users []localmodel.User, truncated bool, err error) {
	truncated, err = s.StreamExportUsersContext(ctx, req, func(batch []localmodel.User) error {
		users = append(users, batch...)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return users, truncated, nil
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
