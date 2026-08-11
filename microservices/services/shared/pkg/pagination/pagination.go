// Package pagination provides pagination query parsing and GORM pagination application.

package pagination

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PageRequest contains pagination query parameters.
type PageRequest struct {
	Page     int `json:"page" form:"page"`
	PageSize int `json:"page_size" form:"page_size"`
}

// GetPageRequest reads pagination parameters from a request.
func GetPageRequest(c *gin.Context) PageRequest {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	return PageRequest{
		Page:     page,
		PageSize: pageSize,
	}
}

// Paginate applies offset and limit to a GORM query.
func Paginate(req PageRequest) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		offset := (req.Page - 1) * req.PageSize
		return db.Offset(offset).Limit(req.PageSize)
	}
}

