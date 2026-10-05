package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"museum-license/internal/auth"
	"museum-license/internal/config"
	"museum-license/internal/mail"
	"museum-license/internal/models"
)

type Server struct {
	db     *gorm.DB
	cfg    config.Config
	auth   *auth.Manager
	mailer *mail.Service
	secret []byte
}

func NewServer(db *gorm.DB, cfg config.Config, am *auth.Manager, ms *mail.Service) *Server {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	s := &Server{db: db, cfg: cfg, auth: am, mailer: ms, secret: secret}
	go s.expirySweeper()
	return s
}

func (s *Server) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	a := r.Group("/api")
	a.POST("/auth/login", s.login)
	a.POST("/auth/logout", s.logout)
	a.GET("/me", s.auth.Middleware(false), s.me)

	authed := a.Group("", s.auth.Middleware(true))
	{
		authed.GET("/mails", s.listMails)

		authed.GET("/images", s.listImages)
		authed.POST("/images", auth.RequireStaff(), s.uploadImage)
		authed.GET("/images/:id/file/preview", s.previewFile)

		authed.GET("/applications", s.listApplications)
		authed.POST("/applications", s.createApplication)
		authed.GET("/applications/:id", s.getApplication)
		authed.POST("/applications/:id/review", auth.RequireStaff(), s.reviewApplication)
		authed.POST("/applications/:id/revoke", auth.RequireStaff(), s.revokeApplication)
		authed.GET("/applications/:id/download-token", s.mintDownloadToken)

		authed.GET("/publications", s.listPublications)
		authed.POST("/applications/:id/publications", s.createPublication)
		authed.GET("/publications/export.csv", s.exportPublicationsCSV)
	}

	// 取件链接：令牌 + 实时授权校验（撤回/到期立即失效），不走静态目录
	r.GET("/api/download", s.download)

	s.serveFrontend(r)
	return r
}

// ---------- 下载令牌 ----------

const tokenTTL = 10 * time.Minute

// 令牌内容：appID|level|userID|expireUnix，附 HMAC-SHA256
func (s *Server) signToken(payload string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}

func (s *Server) verifyToken(tok string) (appID, level, userID uint, exp int64, err error) {
	parts := strings.SplitN(tok, ".", 2)
	if len(parts) != 2 {
		err = errors.New("malformed token")
		return
	}
	payloadBytes, perr := base64.RawURLEncoding.DecodeString(parts[0])
	if perr != nil {
		err = perr
		return
	}
	payload := string(payloadBytes)
	want := s.signToken(payload)
	if !hmac.Equal([]byte(want), []byte(tok)) {
		err = errors.New("bad signature")
		return
	}
	fields := strings.Split(payload, "|")
	if len(fields) != 4 {
		err = errors.New("bad payload")
		return
	}
	appID64, _ := strconv.ParseUint(fields[0], 10, 64)
	lv, _ := strconv.ParseUint(fields[1], 10, 64)
	uid, _ := strconv.ParseUint(fields[2], 10, 64)
	exp, _ = strconv.ParseInt(fields[3], 10, 64)
	appID, level, userID = uint(appID64), uint(lv), uint(uid)
	if time.Now().Unix() > exp {
		err = errors.New("token expired")
	}
	return
}

// checkDownload 是取件权限的唯一入口：任何时候都以数据库中的当前状态为准。
// 撤回或到期后，即使申请人手里仍有旧链接，也无法取得原图。
func (s *Server) checkDownload(appID uint, level int) (*models.Application, *models.Image, string) {
	var app models.Application
	if err := s.db.First(&app, appID).Error; err != nil {
		return nil, nil, "授权记录不存在"
	}
	now := time.Now()
	switch app.EffectiveState(now) {
	case models.StatusRevoked:
		return &app, nil, "授权已撤回，下载链接已停止服务"
	case models.StatusExpired:
		return &app, nil, "授权已到期，下载链接已停止服务"
	case models.StatusRejected:
		return &app, nil, "授权申请未获批准"
	case models.StatusPending:
		return &app, nil, "申请尚在审批中"
	}
	if level > app.MaxResolution {
		return &app, nil, fmt.Sprintf("当前授权最高仅允许 %d 级分辨率", app.MaxResolution)
	}
	var img models.Image
	if err := s.db.First(&img, app.ImageID).Error; err != nil {
		return &app, nil, "藏品图片不存在"
	}
	return &app, &img, ""
}

func levelPath(img *models.Image, level int) string {
	switch level {
	case models.ResPreview:
		return img.PreviewPath
	case models.ResWeb:
		return img.WebPath
	default:
		return img.PrintPath
	}
}
