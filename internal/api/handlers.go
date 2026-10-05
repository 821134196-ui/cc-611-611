package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"museumlicense/internal/auth"
	"museumlicense/internal/email"
	"museumlicense/internal/imaging"
	"museumlicense/internal/models"
	"museumlicense/internal/services"
)

type Handlers struct {
	DB      *gorm.DB
	Lic     *services.LicenseService
	Pub     *services.PublicationService
	Mailer  *email.Mailer
	Auth    *auth.Manager
	Storage string
}

// 错误统一映射：业务冲突返回 409，便于前端提示“已被他人处理”。
func mapServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrConflict), errors.Is(err, services.ErrNotPending):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrNotApproved):
		c.JSON(http.StatusGone, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrTierNotGranted):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrInvalidInput), errors.Is(err, services.ErrAlreadyPublished):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
	}
}

// ---- 认证 ----

type registerRequest struct {
	Username string `json:"username" binding:"required,min=2,max=40"`
	Password string `json:"password" binding:"required,min=4"`
	Email    string `json:"email"`
}

func (h *Handlers) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写用户名与至少 4 位密码"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
		return
	}
	user := models.User{Username: req.Username, Password: hash, Email: req.Email, Role: models.RoleApplicant}
	if err := h.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "用户名已存在"})
		return
	}
	token, _ := h.Auth.IssueToken(&user)
	c.JSON(http.StatusOK, gin.H{"token": token, "user": sanitizeUser(&user)})
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *Handlers) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入用户名和密码"})
		return
	}
	var user models.User
	if err := h.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	if !auth.CheckPassword(user.Password, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	token, _ := h.Auth.IssueToken(&user)
	c.JSON(http.StatusOK, gin.H{"token": token, "user": sanitizeUser(&user)})
}

func (h *Handlers) Me(c *gin.Context) {
	claims := auth.CurrentUser(c)
	var user models.User
	if err := h.DB.First(&user, claims.UserID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": sanitizeUser(&user)})
}

func sanitizeUser(u *models.User) gin.H {
	return gin.H{"id": u.ID, "username": u.Username, "role": u.Role, "email": u.Email}
}

// ---- 藏品 ----

func (h *Handlers) ListArtifacts(c *gin.Context) {
	var arts []models.Artifact
	q := h.DB.Order("id ASC")
	if kw := c.Query("keyword"); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("name LIKE ? OR code LIKE ? OR dynasty LIKE ?", like, like, like)
	}
	if err := q.Find(&arts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": arts})
}

func (h *Handlers) GetArtifact(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var art models.Artifact
	if err := h.DB.First(&art, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "藏品不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": art})
}

// createArtifactRequest 馆方录入藏品（multipart/form-data）：
// file 为本地图片，系统保存原图并生成预览图。
func (h *Handlers) CreateArtifact(c *gin.Context) {
	var req struct {
		Code        string `form:"code" binding:"required"`
		Name        string `form:"name" binding:"required"`
		Dynasty     string `form:"dynasty"`
		Category    string `form:"category"`
		Description string `form:"description"`
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "编号和名称必填"})
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传图片文件"})
		return
	}
	ext := strings.ToLower(extOf(fh.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		ext = ".jpg"
	}
	origDir := h.Storage + "/originals"
	previewDir := h.Storage + "/previews"
	if err := mkdirAll(origDir, previewDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储目录不可用"})
		return
	}
	origPath := origDir + "/" + req.Code + ext
	if err := c.SaveUploadedFile(fh, origPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败"})
		return
	}
	w, hgt, derr := imaging.Dimensions(origPath)
	if derr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "图片无法解析，请上传 JPG/PNG"})
		return
	}
	previewPath, perr := imaging.MakePreview(origPath, previewDir, 1200)
	if perr != nil {
		previewPath = ""
	}
	art := models.Artifact{
		Code: req.Code, Name: req.Name, Dynasty: req.Dynasty,
		Category: req.Category, Description: req.Description,
		OriginalPath: origPath, PreviewPath: previewPath,
		OriginalW: w, OriginalH: hgt,
	}
	if err := h.DB.Create(&art).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "藏品编号已存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": art})
}

// ArtifactImage 内联返回藏品预览图（任何登录用户可看目录缩略图）；
// 支持 ?token= 以便 <img> 标签引用；original 类型仍须走授权下载接口。
func (h *Handlers) ArtifactImage(c *gin.Context) {
	if auth.CurrentUser(c) == nil {
		tok := c.Query("token")
		claims, err := h.Auth.ParseToken(tok)
		if err != nil || claims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		c.Set("current_user", claims)
	}
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var art models.Artifact
	if err := h.DB.First(&art, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "藏品不存在"})
		return
	}
	path := art.PreviewPath
	if path == "" {
		path = art.OriginalPath
	}
	c.Header("Content-Disposition", "inline")
	c.File(path)
}

// ---- 申请 / 授权 ----

type createApplicationRequest struct {
	ArtifactID    uint   `json:"artifact_id" binding:"required"`
	Purpose       string `json:"purpose" binding:"required"`
	Channel       string `json:"channel" binding:"required"`
	RequestedTier string `json:"requested_tier"`
	RequestedEdit bool   `json:"requested_edit"`
	StartAt       string `json:"start_at"`
	EndAt         string `json:"end_at" binding:"required"`
}

func parseDate(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02"}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func (h *Handlers) CreateApplication(c *gin.Context) {
	claims := auth.CurrentUser(c)
	var req createApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请完整填写用途、渠道和期限"})
		return
	}
	end, ok := parseDate(req.EndAt)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "到期时间格式不正确"})
		return
	}
	start := time.Now()
	if req.StartAt != "" {
		if t, ok2 := parseDate(req.StartAt); ok2 {
			start = t
		}
	}
	app, err := h.Lic.CreateApplication(services.CreateApplicationInput{
		ArtifactID:    req.ArtifactID,
		ApplicantID:   claims.UserID,
		Purpose:       req.Purpose,
		Channel:       req.Channel,
		RequestedTier: req.RequestedTier,
		RequestedEdit: req.RequestedEdit,
		StartAt:       start,
		EndAt:         end,
	})
	if err != nil {
		mapServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": app})
}

func (h *Handlers) ListApplications(c *gin.Context) {
	claims := auth.CurrentUser(c)
	status := c.Query("status")
	var applicantID uint
	var artifactID uint
	if claims.Role == models.RoleApplicant {
		applicantID = claims.UserID // 申请人只能看到自己的
	} else if uid := c.Query("applicant_id"); uid != "" {
		v, _ := strconv.ParseUint(uid, 10, 64)
		applicantID = uint(v)
	}
	if aid := c.Query("artifact_id"); aid != "" {
		v, _ := strconv.ParseUint(aid, 10, 64)
		artifactID = uint(v)
	}
	apps, err := h.Lic.ListApplications(applicantID, status, artifactID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	now := time.Now()
	type row struct {
		models.Application
		EffectiveStatus string `json:"effective_status"`
	}
	items := make([]row, 0, len(apps))
	for _, a := range apps {
		items = append(items, row{Application: a, EffectiveStatus: a.EffectiveStatus(now)})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handlers) GetApplication(c *gin.Context) {
	claims := auth.CurrentUser(c)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	app, err := h.Lic.GetApplication(uint(id))
	if err != nil {
		mapServiceError(c, err)
		return
	}
	if claims.Role == models.RoleApplicant && app.ApplicantID != claims.UserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权查看该申请"})
		return
	}
	pubs, _ := h.Pub.ListByApplication(app.ID, claims.UserID, claims.Role)
	c.JSON(http.StatusOK, gin.H{
		"item":             app,
		"effective_status": app.EffectiveStatus(time.Now()),
		"publications":     pubs,
	})
}

type decisionRequest struct {
	Approve        bool   `json:"approve"`
	ResolutionTier string `json:"resolution_tier"`
	AllowEdit      bool   `json:"allow_edit"`
	ExpiresAt      string `json:"expires_at"`
	Note           string `json:"note"`
}

func (h *Handlers) Decide(c *gin.Context) {
	claims := auth.CurrentUser(c)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req decisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "审批参数不完整"})
		return
	}
	in := services.DecisionInput{
		Approve:        req.Approve,
		ResolutionTier: req.ResolutionTier,
		AllowEdit:      req.AllowEdit,
		Note:           req.Note,
		ReviewerID:     claims.UserID,
	}
	if req.Approve {
		if in.ResolutionTier == "" {
			in.ResolutionTier = models.TierPreview
		}
		if in.ResolutionTier != models.TierPreview && in.ResolutionTier != models.TierOriginal {
			c.JSON(http.StatusBadRequest, gin.H{"error": "分辨率档位非法"})
			return
		}
		exp, ok := parseDate(req.ExpiresAt)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请选择授权到期时间"})
			return
		}
		in.ExpiresAt = exp
	}
	app, err := h.Lic.Decide(uint(id), in)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": app})
}

type revokeRequest struct {
	Reason string `json:"reason"`
}

func (h *Handlers) Revoke(c *gin.Context) {
	claims := auth.CurrentUser(c)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req revokeRequest
	_ = c.ShouldBindJSON(&req)
	app, err := h.Lic.Revoke(uint(id), claims.UserID, req.Reason)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": app})
}

// Download 每次请求都实时鉴权：
// 撤回 / 过期 / 未批 / 档位不足都会立刻拒绝，不存在“旧链接仍可取原图”的窗口。
func (h *Handlers) Download(c *gin.Context) {
	claims := auth.CurrentUser(c)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	resolution := c.DefaultQuery("resolution", models.TierPreview)
	check, err := h.Lic.AuthorizeDownload(uint(id), claims.UserID, claims.Role, resolution)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename="+check.FileName)
	c.File(check.FilePath)
}

// ---- 发表登记 ----

type publicationRequest struct {
	Title       string `json:"title" binding:"required"`
	URL         string `json:"url" binding:"required"`
	PublishedAt string `json:"published_at"`
	Note        string `json:"note"`
}

func (h *Handlers) RegisterPublication(c *gin.Context) {
	claims := auth.CurrentUser(c)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req publicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写引用标题和页面地址"})
		return
	}
	in := services.RegisterPublicationInput{
		ApplicationID: uint(id),
		UserID:        claims.UserID,
		Title:         req.Title,
		URL:           req.URL,
		Note:          req.Note,
	}
	if req.PublishedAt != "" {
		if t, ok := parseDate(req.PublishedAt); ok {
			in.PublishedAt = t
		}
	}
	pub, err := h.Pub.Register(in)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": pub})
}

func (h *Handlers) ListApplicationPublications(c *gin.Context) {
	claims := auth.CurrentUser(c)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	pubs, err := h.Pub.ListByApplication(uint(id), claims.UserID, claims.Role)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": pubs})
}

// SearchPublications 检索已发表引用位置。
// 馆方可按条件检索全部（默认包含已撤回/过期授权的记录）；
// 申请人只能检索自己申请下的位置。
func (h *Handlers) SearchPublications(c *gin.Context) {
	claims := auth.CurrentUser(c)
	f := services.PublicationFilter{
		Applicant:    c.Query("applicant"),
		Keyword:      c.Query("keyword"),
		ArtifactCode: c.Query("artifact_code"),
		OnlyActive:   c.Query("only_active") == "1" || c.Query("only_active") == "true",
	}
	if claims.Role == models.RoleApplicant {
		f = services.PublicationFilter{Keyword: c.Query("keyword"), ArtifactCode: c.Query("artifact_code")}
	}
	if aid := c.Query("artifact_id"); aid != "" {
		v, _ := strconv.ParseUint(aid, 10, 64)
		f.ArtifactID = uint(v)
	}
	pubs, err := h.Pub.SearchForApplicant(claims.UserID, claims.Role, f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	now := time.Now()
	type row struct {
		models.Publication
		EffectiveStatus string `json:"effective_status"`
	}
	items := make([]row, 0, len(pubs))
	for _, p := range pubs {
		st := ""
		if p.Application != nil {
			st = p.Application.EffectiveStatus(now)
		}
		items = append(items, row{Publication: p, EffectiveStatus: st})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// ExportPublications 导出 CSV，供馆方联系使用方。
func (h *Handlers) ExportPublications(c *gin.Context) {
	f := services.PublicationFilter{
		Applicant:    c.Query("applicant"),
		Keyword:      c.Query("keyword"),
		ArtifactCode: c.Query("artifact_code"),
	}
	if aid := c.Query("artifact_id"); aid != "" {
		v, _ := strconv.ParseUint(aid, 10, 64)
		f.ArtifactID = uint(v)
	}
	pubs, err := h.Pub.Search(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "导出失败"})
		return
	}
	// 加 UTF-8 BOM，避免 Excel 打开中文乱码。
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=publications.csv")
	c.Writer.WriteString("\xEF\xBB\xBF")
	c.Writer.WriteString(services.CSVHeader)
	for _, p := range pubs {
		c.Writer.WriteString(services.CSVRow(p))
	}
}

// ---- 模拟邮件 ----

func (h *Handlers) ListEmails(c *gin.Context) {
	claims := auth.CurrentUser(c)
	q := h.DB.Order("id DESC")
	if claims.Role != models.RoleCurator {
		q = q.Where("to_user_id = ?", claims.UserID)
	}
	var msgs []models.EmailMessage
	if err := q.Limit(200).Find(&msgs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": msgs})
}

// ---- 下载日志（馆方审计） ----

func (h *Handlers) ListDownloadLogs(c *gin.Context) {
	var logs []models.DownloadLog
	q := h.DB.Order("id DESC").Limit(200)
	if aid := c.Query("application_id"); aid != "" {
		q = q.Where("application_id = ?", aid)
	}
	if err := q.Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": logs})
}

// ---- 小工具 ----

func extOf(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[i:]
		}
	}
	return ""
}

func mkdirAll(dirs ...string) error {
	for _, d := range dirs {
		if err := mkdir(d); err != nil {
			return err
		}
	}
	return nil
}
