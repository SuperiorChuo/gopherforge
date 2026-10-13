package api

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"gorm.io/gorm"
	"net/http"
	"time"
)

func notFoundOr(c *gin.Context, err error, msg string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Error(c, http.StatusNotFound, msg)
		return
	}
	response.Error(c, http.StatusBadRequest, err.Error())
}

func parseDate(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	if t, err := time.ParseInLocation("2006-01-02", *s, time.Local); err == nil {
		return &t
	}
	return nil
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func firstU64(vals ...uint64) uint64 {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}
