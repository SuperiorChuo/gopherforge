package api

// 客户列表导出（路线图第 11 项在 CRM 域的接入）：CSV + UTF-8 BOM（Excel
// 双击打开中文不乱码），复用列表的筛选与数据权限硬隔离；业务域功能，
// 不同步下游脚手架。CSV 走标准库零新依赖（xlsx 组件属 shared，业务服务
// 构建上下文不含 shared，见 docker-compose 上下文约定）。

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/crm/internal/store/crm"
)

const customerExportCap = 10000

// ExportCustomers handles GET /api/v1/crm/customers/export —
// 按当前筛选条件导出客户 CSV（分页循环拉取，上限 customerExportCap）。
func (s *Server) ExportCustomers(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	owner, _ := strconv.ParseUint(c.Query("owner_user_id"), 10, 64)
	base := store.CustomerFilter{
		Keyword: c.Query("keyword"), Level: c.Query("level"), Owner: owner,
		Scope: c.Query("scope"), Me: u.UserID, Access: s.scopeOf(c, u),
	}

	filename := fmt.Sprintf("crm_customers_%s.csv", time.Now().Format("20060102150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(c.Writer)
	defer w.Flush()
	_ = w.Write([]string{"ID", "客户名称", "行业", "级别", "电话", "地址", "来源",
		"负责人ID", "最近跟进", "创建时间", "备注"})

	exported, page := 0, 1
	for exported < customerExportCap {
		f := base
		f.Page = store.Page{Page: page, PageSize: 500}
		list, total, err := s.Store.ListCustomers(u.TenantID, f)
		if err != nil {
			// 响应流已开写，无法再回错误 envelope（与日志 CSV 导出同限制）
			return
		}
		for i := range list {
			cu := &list[i]
			follow := ""
			if cu.LastFollowAt != nil {
				follow = cu.LastFollowAt.Format("2006-01-02 15:04:05")
			}
			_ = w.Write([]string{
				strconv.FormatUint(cu.ID, 10), cu.Name, cu.Industry, cu.Level,
				cu.Phone, cu.Address, cu.Source,
				strconv.FormatUint(cu.OwnerUserID, 10), follow,
				cu.CreatedAt.Format("2006-01-02 15:04:05"), cu.Note,
			})
			exported++
			if exported >= customerExportCap {
				_ = w.Write([]string{fmt.Sprintf("…已达导出上限 %d 行，请缩小筛选范围分批导出", customerExportCap)})
				break
			}
		}
		if int64(exported) >= total || len(list) == 0 {
			break
		}
		page++
	}
}
