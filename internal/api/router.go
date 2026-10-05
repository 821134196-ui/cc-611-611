package api

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"museumlicense/internal/auth"
	"museumlicense/internal/models"
)

// Router 装配全部路由。webFS 为嵌入的前端构建产物（测试中可为 nil）。
func (h *Handlers) Router(webFS fs.FS) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.GET("/api/health", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	r.POST("/api/auth/register", h.Register)
	r.POST("/api/auth/login", h.Login)

	authed := r.Group("/")
	authed.Use(h.Auth.RequireAuth())
	{
		authed.GET("/api/auth/me", h.Me)

		authed.GET("/api/artifacts", h.ListArtifacts)
		authed.GET("/api/artifacts/:id", h.GetArtifact)
		authed.GET("/api/artifacts/:id/image", h.ArtifactImage)

		authed.POST("/api/applications", h.CreateApplication)
		authed.GET("/api/applications", h.ListApplications)
		authed.GET("/api/applications/:id", h.GetApplication)
		authed.POST("/api/applications/:id/decide", auth.RequireRole(models.RoleCurator), h.Decide)
		authed.POST("/api/applications/:id/revoke", auth.RequireRole(models.RoleCurator), h.Revoke)
		authed.GET("/api/applications/:id/download", h.Download)

		authed.POST("/api/applications/:id/publications", h.RegisterPublication)
		authed.GET("/api/applications/:id/publications", h.ListApplicationPublications)
		authed.GET("/api/publications", h.SearchPublications)

		authed.GET("/api/emails", h.ListEmails)

		curator := authed.Group("/")
		curator.Use(auth.RequireRole(models.RoleCurator))
		{
			curator.POST("/api/artifacts", h.CreateArtifact)
			curator.GET("/api/publications/export", h.ExportPublications)
			curator.GET("/api/download-logs", h.ListDownloadLogs)
		}
	}

	if webFS != nil {
		h.mountFrontend(r, webFS)
	}
	return r
}

// mountFrontend 托管嵌入的 Vue 构建产物；未知非 /api 路径回退到 index.html
// 以支持前端路由。
func (h *Handlers) mountFrontend(r *gin.Engine, webFS fs.FS) {
	// webFS 是 web 包 //go:embed dist 的产物，根下第一层就是 dist 目录。
	dist, err := fs.Sub(webFS, "dist")
	if err != nil {
		return
	}
	fileServer := http.FileServer(http.FS(dist))
	r.NoRoute(func(c *gin.Context) {
		p := strings.TrimPrefix(c.Request.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(dist, p); err != nil {
			// SPA 回退
			c.Request.URL.Path = "/"
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}

// StartExpirySweeper 定时把到期授权标记为 expired 并发送模拟提醒。
// 下载接口本身也实时判期，因此巡检延迟不影响安全，只影响列表状态与提醒。
func StartExpirySweeper(h *Handlers, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	_ = h
	sweep := func() {
		if _, err := h.Lic.SweepExpired(); err != nil {
			// 巡检失败不致命，下个周期重试。
			_ = err
		}
	}
	sweep()
	for {
		select {
		case <-ticker.C:
			sweep()
		case <-stop:
			return
		}
	}
}
