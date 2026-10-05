package api

import (
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"museum-license/internal/models"
)

// errReviewConflict 表示申请已不再是 pending，被并发的其他审批动作抢先处理
var errReviewConflict = errors.New("application already reviewed")

func newCSVWriter(w io.Writer) *csv.Writer {
	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	return cw
}

// expirySweeper 周期扫描：仅用于把"数据库状态"同步为 expired 并发出到期提醒。
// 访问控制不依赖它——下载校验始终实时比较 ExpireAt，
// 所以即使扫描协程尚未运行，到期后旧链接也会立刻被拒绝。
func (s *Server) expirySweeper() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.sweepExpired()
	}
}

func (s *Server) sweepExpired() {
	var apps []models.Application
	if err := s.db.Preload("Image").Preload("Applicant").
		Where("status = ? AND expire_at < ?", models.StatusApproved, time.Now()).
		Find(&apps).Error; err != nil {
		return
	}
	for i := range apps {
		a := apps[i]
		// 条件更新，避免与撤回动作竞争
		res := s.db.Model(&models.Application{}).
			Where("id = ? AND status = ?", a.ID, models.StatusApproved).
			Update("status", models.StatusExpired)
		if res.RowsAffected > 0 && a.Applicant != nil && a.Image != nil {
			s.mailer.NotifyExpired(a.Applicant.Name, a.Applicant.Email, a.Image.Title, a.ExpireAt)
		}
	}
}

// serveFrontend 生产模式托管前端构建产物；未构建时给出提示
func (s *Server) serveFrontend(r *gin.Engine) {
	dist := s.cfg.StaticDir
	if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(404, gin.H{"error": "not found"})
				return
			}
			c.String(http.StatusServiceUnavailable,
				"前端尚未构建：请先在 frontend 目录执行 npm install && npm run build，或运行 ./run.sh")
		})
		return
	}
	r.Static("/assets", filepath.Join(dist, "assets"))
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		c.File(filepath.Join(dist, "index.html")) // SPA history fallback
	})
}
