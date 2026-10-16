package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ---------- SERP rank checks ----------

type createSerpCheckReq struct {
	KeywordID uint64 `json:"keyword_id"` // 0 = 全部 tracking 关键词
}

// CreateSerpCheck 创建排名检查任务并立即返回；同租户已有 running 任务时返回该任务且 reused=true。
func (s *Server) CreateSerpCheck(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req createSerpCheckReq
	if !bindOptionalJSON(c, &req) {
		return
	}
	task, reused, err := s.Runner.SubmitSerpTask(u.TenantID, u.UserID, req.KeywordID, "manual")
	if err != nil {
		writeFail(c, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(c, gin.H{"task": task, "reused": reused})
}

// GetSerpTask 轮询任务进度。
func (s *Server) GetSerpTask(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	task, err := s.Store.GetSerpTask(u.TenantID, id)
	if err != nil {
		writeFail(c, http.StatusNotFound, "not found")
		return
	}
	writeOK(c, gin.H{"task": task})
}

func (s *Server) ListSerpTasks(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	list, total, err := s.Store.ListSerpTasks(u.TenantID, pageOf(c))
	if err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, gin.H{"list": list, "total": total})
}

// KeywordHistory 返回某关键词的排名历史（趋势数据）。
func (s *Server) KeywordHistory(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	days := 30
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}
	list, err := s.Store.ListRankSnapshots(u.TenantID, id, c.Query("engine"), days)
	if err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, gin.H{"list": list})
}
