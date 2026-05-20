package pagination

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PageRequest 分页请求
type PageRequest struct {
	Page     int `json:"page" form:"page"`           // 页码，从1开始
	PageSize int `json:"page_size" form:"page_size"` // 每页数量
}

// PageResponse 分页响应
type PageResponse struct {
	Page     int   `json:"page"`      // 当前页码
	PageSize int   `json:"page_size"` // 每页数量
	Total    int64 `json:"total"`     // 总记录数
	Pages    int   `json:"pages"`     // 总页数
}

// GetPageRequest 从请求中获取分页参数
func GetPageRequest(c *gin.Context) PageRequest {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	// 限制范围
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100 // 最大每页100条
	}

	return PageRequest{
		Page:     page,
		PageSize: pageSize,
	}
}

// Paginate 分页查询
func Paginate(req PageRequest) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		offset := (req.Page - 1) * req.PageSize
		return db.Offset(offset).Limit(req.PageSize)
	}
}

// CalculatePages 计算总页数
func CalculatePages(total int64, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	pages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		pages++
	}
	return pages
}

// NewPageResponse 创建分页响应
func NewPageResponse(req PageRequest, total int64) PageResponse {
	return PageResponse{
		Page:     req.Page,
		PageSize: req.PageSize,
		Total:    total,
		Pages:    CalculatePages(total, req.PageSize),
	}
}
