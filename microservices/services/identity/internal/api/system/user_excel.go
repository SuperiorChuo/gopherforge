package system

// 用户 Excel 导出 / 导入模板 / 批量导入（路线图第 11 项）。
// xlsx 编解码走 shared/pkg/excel；导出复用列表权限与数据范围，
// 导入复用创建权限与 CreateUserContext 的全部校验（配额/重名/越权部门）。

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/identity/internal/pkg/authz"
	"github.com/go-admin-kit/services/identity/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/excel"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

const userImportMaxFileSize = 5 << 20 // 5MB

var (
	userExportHeaders = []string{"ID", "用户名", "昵称", "邮箱", "手机号", "部门", "状态", "创建时间"}
	userExportWidths  = []float64{8, 18, 18, 26, 16, 18, 8, 20}
	userImportHeaders = []string{"用户名*", "昵称", "初始密码", "邮箱", "手机号", "部门名称", "状态"}
	userImportWidths  = []float64{18, 18, 18, 26, 16, 18, 10}
)

// ExportUsers 按当前筛选条件导出用户 xlsx（GET /users/export）。
func (a *UserManagementAPI) ExportUsers(c *gin.Context) {
	var req system.UserListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}
	if statusStr := c.Query("status"); statusStr != "" {
		if status, err := strconv.ParseInt(statusStr, 10, 8); err == nil {
			statusInt8 := int8(status)
			req.Status = &statusInt8
		}
	}
	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		internalServerError(c, "failed to resolve user data scope", err)
		return
	}
	req.DataScope = dataScope

	users, truncated, err := a.userService.ExportUsersContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to export users", err)
		return
	}
	deptNames, err := a.userService.DepartmentNameMapContext(c.Request.Context())
	if err != nil {
		internalServerError(c, "failed to load departments", err)
		return
	}

	sheet, err := excel.NewSheet("用户", userExportHeaders, userExportWidths)
	if err != nil {
		internalServerError(c, "failed to build excel", err)
		return
	}
	for i := range users {
		u := &users[i]
		statusText := "启用"
		if u.Status != 1 {
			statusText = "禁用"
		}
		if err := sheet.AppendRow(u.ID, u.Username, u.Nickname, u.Email, u.Phone,
			deptNames[u.DepartmentID], statusText,
			u.CreatedAt.Format("2006-01-02 15:04:05")); err != nil {
			internalServerError(c, "failed to build excel", err)
			return
		}
	}
	if truncated {
		_ = sheet.AppendRow(fmt.Sprintf("…已达导出上限 %d 行，请缩小筛选范围分批导出", system.UserExportCap))
	}
	filename := fmt.Sprintf("users_%s.xlsx", time.Now().Format("20060102150405"))
	// 写响应后无法再回错误 envelope，失败只能中断连接（与 CSV 导出同限制）
	_ = sheet.WriteHTTP(c.Writer, filename)
}

// DownloadUserImportTemplate 下载导入模板（GET /users/import-template）。
func (a *UserManagementAPI) DownloadUserImportTemplate(c *gin.Context) {
	sheet, err := excel.NewSheet("用户导入", userImportHeaders, userImportWidths)
	if err != nil {
		internalServerError(c, "failed to build excel", err)
		return
	}
	_ = sheet.AppendRow("zhangsan", "张三", "", "zhangsan@example.com", "13800000000", "技术部", "启用")
	_ = sheet.AppendRow(fmt.Sprintf(
		"说明：用户名必填；密码留空用默认 %s；部门须为已存在的部门名称（留空不挂部门）；状态填 启用/禁用（默认启用）。导入前请删除示例行与本行。",
		system.UserImportDefaultPassword))
	_ = sheet.WriteHTTP(c.Writer, "user_import_template.xlsx")
}

// ImportUsers 批量导入用户（POST /users/import，multipart 字段 file）。
// 部分成功语义：单行失败不中断其余行，逐行错误明细返回给前端展示。
func (a *UserManagementAPI) ImportUsers(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "缺少上传文件（form 字段 file）")
		return
	}
	if fileHeader.Size > userImportMaxFileSize {
		response.BadRequest(c, "文件超过 5MB 上限")
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		response.BadRequest(c, "无法读取上传文件")
		return
	}
	defer func() { _ = f.Close() }()

	rows, err := excel.ReadFirstSheet(f, system.UserImportMaxRows)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if len(rows) < 2 {
		response.BadRequest(c, "文件中没有数据行")
		return
	}
	if !strings.Contains(excel.Cell(rows[0], 0), "用户名") {
		response.BadRequest(c, "表头不符，请使用「下载模板」生成的文件")
		return
	}

	deptNames, err := a.userService.DepartmentNameMapContext(c.Request.Context())
	if err != nil {
		internalServerError(c, "failed to load departments", err)
		return
	}
	deptByName := make(map[string]uint, len(deptNames))
	dupNames := map[string]bool{}
	for id, name := range deptNames {
		if _, exists := deptByName[name]; exists {
			dupNames[name] = true
			continue
		}
		deptByName[name] = id
	}

	var (
		toCreate []system.ImportUserRow
		failures []system.ImportRowError
		total    int
	)
	for i := 1; i < len(rows); i++ {
		rowNum := i + 1 // Excel 行号（1 为表头）
		username := strings.TrimSpace(excel.Cell(rows[i], 0))
		nickname := strings.TrimSpace(excel.Cell(rows[i], 1))
		password := strings.TrimSpace(excel.Cell(rows[i], 2))
		email := strings.TrimSpace(excel.Cell(rows[i], 3))
		phone := strings.TrimSpace(excel.Cell(rows[i], 4))
		deptName := strings.TrimSpace(excel.Cell(rows[i], 5))
		statusText := strings.TrimSpace(excel.Cell(rows[i], 6))

		// 整行空 / 模板说明行未删：静默跳过
		if username == "" && nickname == "" && email == "" && phone == "" && deptName == "" {
			continue
		}
		if strings.HasPrefix(username, "说明：") {
			continue
		}
		total++
		fail := func(reason string) {
			failures = append(failures, system.ImportRowError{Row: rowNum, Username: username, Reason: reason})
		}
		if username == "" {
			fail("用户名不能为空")
			continue
		}
		if password == "" {
			password = system.UserImportDefaultPassword
		}
		if len(password) < 6 {
			fail("初始密码至少 6 位")
			continue
		}
		var deptID uint
		if deptName != "" {
			if dupNames[deptName] {
				fail("部门名称「" + deptName + "」在租户内重名，请手工创建该用户")
				continue
			}
			id, hit := deptByName[deptName]
			if !hit {
				fail("部门「" + deptName + "」不存在")
				continue
			}
			deptID = id
		}
		status := int8(1)
		switch statusText {
		case "", "启用", "1":
			status = 1
		case "禁用", "0":
			status = 0
		default:
			fail("状态仅支持 启用/禁用")
			continue
		}
		toCreate = append(toCreate, system.ImportUserRow{
			Row: rowNum,
			Req: system.CreateUserRequest{
				Username: username, Password: password, Nickname: nickname,
				Email: email, Phone: phone, DepartmentID: deptID, Status: status,
			},
		})
	}

	success, svcFailures := a.userService.ImportUsersContext(c.Request.Context(), toCreate)
	failures = append(failures, svcFailures...)
	sort.Slice(failures, func(i, j int) bool { return failures[i].Row < failures[j].Row })

	response.Success(c, gin.H{
		"total":   total,
		"success": success,
		"failed":  len(failures),
		"errors":  failures,
	})
}
