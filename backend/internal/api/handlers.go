package api

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"museum-license/internal/auth"
	"museum-license/internal/media"
	"museum-license/internal/models"
)

// ---------- 认证 ----------

func (s *Server) login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请输入用户名"})
		return
	}
	u, token, err := s.auth.Login(strings.TrimSpace(req.Username))
	if err != nil {
		c.JSON(401, gin.H{"error": "用户不存在（演示账号：curator / alice / bob）"})
		return
	}
	c.SetCookie(auth.CookieName, token, 8*3600, "/", "", false, true)
	c.JSON(200, gin.H{"user": u})
}

func (s *Server) logout(c *gin.Context) {
	if t, err := c.Cookie(auth.CookieName); err == nil {
		s.auth.Logout(t)
	}
	c.SetCookie(auth.CookieName, "", -1, "/", "", false, true)
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) me(c *gin.Context) {
	u := auth.CurrentUser(c)
	if u == nil {
		c.JSON(200, gin.H{"user": nil})
		return
	}
	c.JSON(200, gin.H{"user": u})
}

// ---------- 图片 ----------

func (s *Server) listImages(c *gin.Context) {
	var imgs []models.Image
	q := s.db.Model(&models.Image{})
	if kw := c.Query("q"); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("title LIKE ? OR accession_no LIKE ? OR dynasty LIKE ?", like, like, like)
	}
	if err := q.Order("accession_no asc").Find(&imgs).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	out := make([]gin.H, 0, len(imgs))
	for i := range imgs {
		img := imgs[i]
		out = append(out, s.imageJSON(&img))
	}
	c.JSON(200, gin.H{"items": out})
}

func (s *Server) imageJSON(img *models.Image) gin.H {
	return gin.H{
		"id": img.ID, "accessionNo": img.AccessionNo, "title": img.Title,
		"dynasty": img.Dynasty, "category": img.Category,
		"width": img.Width, "height": img.Height,
		"previewUrl": fmt.Sprintf("/api/images/%d/file/preview", img.ID),
		"createdAt":  img.CreatedAt,
	}
}

// previewFile 预览图对所有登录用户开放（低分辨率），原图永远不走此路由
func (s *Server) previewFile(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var img models.Image
	if err := s.db.First(&img, id).Error; err != nil {
		c.Status(404)
		return
	}
	c.File(filepath.Join(s.cfg.DataDir, img.PreviewPath))
}

func (s *Server) uploadImage(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "请选择图片文件"})
		return
	}
	title := c.PostForm("title")
	accession := c.PostForm("accessionNo")
	if title == "" || accession == "" {
		c.JSON(400, gin.H{"error": "藏品编号与名称必填"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(400, gin.H{"error": "无法读取上传文件"})
		return
	}
	defer f.Close()
	src, err := media.DecodeReader(f)
	if err != nil {
		c.JSON(400, gin.H{"error": "仅支持 JPG/PNG 图片"})
		return
	}
	u := auth.CurrentUser(c)
	base := fmt.Sprintf("upload_%d_%d", time.Now().UnixNano(), u.ID)
	prev, web, print, w, h, err := media.Encode(src, s.cfg.ImageDir(), base)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	rel := func(p string) string { p, _ = filepath.Rel(s.cfg.DataDir, p); return p }
	img := models.Image{
		AccessionNo: accession, Title: title,
		Dynasty: c.PostForm("dynasty"), Category: c.PostForm("category"),
		PreviewPath: rel(prev), WebPath: rel(web), PrintPath: rel(print),
		Width: w, Height: h, UploadedBy: u.ID,
	}
	if err := s.db.Create(&img).Error; err != nil {
		c.JSON(409, gin.H{"error": "藏品编号已存在或保存失败"})
		return
	}
	c.JSON(201, gin.H{"item": s.imageJSON(&img)})
}

// ---------- 申请 ----------

type applicationJSON struct {
	models.Application
	EffectiveStatus string `json:"effectiveStatus"`
	Image           struct {
		ID          uint   `json:"id"`
		AccessionNo string `json:"accessionNo"`
		Title       string `json:"title"`
		Dynasty     string `json:"dynasty"`
	} `json:"image" gorm:"-"`
	Applicant struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Email    string `json:"email"`
	} `json:"applicant" gorm:"-"`
	Publications []models.Publication `json:"publications" gorm:"-"`
}

func (s *Server) appJSON(a models.Application, includePubs bool) gin.H {
	now := time.Now()
	j := gin.H{
		"id": a.ID, "imageId": a.ImageID, "applicantId": a.ApplicantID,
		"purpose": a.Purpose, "channel": a.Channel,
		"startAt": a.StartAt, "expireAt": a.ExpireAt,
		"status": a.Status, "effectiveStatus": a.EffectiveState(now),
		"maxResolution": a.MaxResolution, "canEdit": a.CanEdit,
		"reviewNote": a.ReviewNote, "revokeReason": a.RevokeReason,
		"createdAt": a.CreatedAt, "reviewedAt": a.ReviewedAt, "revokedAt": a.RevokedAt,
	}
	if a.Image != nil {
		j["image"] = s.imageJSON(a.Image)
	}
	if a.Applicant != nil {
		j["applicant"] = gin.H{"id": a.Applicant.ID, "username": a.Applicant.Username, "name": a.Applicant.Name, "email": a.Applicant.Email}
	}
	if includePubs {
		var pubs []models.Publication
		s.db.Where("application_id = ?", a.ID).Order("published_at desc").Find(&pubs)
		j["publications"] = pubs
	} else {
		j["publications"] = []models.Publication{}
	}
	return j
}

func (s *Server) listApplications(c *gin.Context) {
	u := auth.CurrentUser(c)
	q := s.db.Model(&models.Application{}).Preload("Image").Preload("Applicant")
	switch c.Query("scope") {
	case "pending":
		q = q.Where("status = ?", models.StatusPending)
	case "mine":
		q = q.Where("applicant_id = ?", u.ID)
	default:
		if u.Role != models.RoleStaff {
			q = q.Where("applicant_id = ?", u.ID)
		}
	}
	if imgID := c.Query("imageId"); imgID != "" {
		q = q.Where("image_id = ?", imgID)
	}
	var apps []models.Application
	if err := q.Order("created_at desc").Find(&apps).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	items := make([]gin.H, 0, len(apps))
	for i := range apps {
		items = append(items, s.appJSON(apps[i], false))
	}
	c.JSON(200, gin.H{"items": items})
}

func (s *Server) getApplication(c *gin.Context) {
	app, ok := s.loadApplication(c)
	if !ok {
		return
	}
	u := auth.CurrentUser(c)
	if u.Role != models.RoleStaff && app.ApplicantID != u.ID {
		c.JSON(403, gin.H{"error": "无权查看该申请"})
		return
	}
	c.JSON(200, gin.H{"item": s.appJSON(*app, true)})
}

func (s *Server) loadApplication(c *gin.Context) (*models.Application, bool) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var app models.Application
	if err := s.db.Preload("Image").Preload("Applicant").First(&app, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "申请不存在"})
		return nil, false
	}
	return &app, true
}

func (s *Server) createApplication(c *gin.Context) {
	u := auth.CurrentUser(c)
	var req struct {
		ImageID  uint   `json:"imageId"`
		Purpose  string `json:"purpose"`
		Channel  string `json:"channel"`
		StartAt  string `json:"startAt"`
		ExpireAt string `json:"expireAt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请完整填写申请信息"})
		return
	}
	if req.ImageID == 0 || strings.TrimSpace(req.Purpose) == "" {
		c.JSON(400, gin.H{"error": "请选择藏品并填写用途说明"})
		return
	}
	var img models.Image
	if err := s.db.First(&img, req.ImageID).Error; err != nil {
		c.JSON(404, gin.H{"error": "藏品不存在"})
		return
	}
	startAt, err1 := time.Parse("2006-01-02", req.StartAt)
	expireAt, err2 := time.Parse("2006-01-02", req.ExpireAt)
	if err1 != nil || err2 != nil {
		c.JSON(400, gin.H{"error": "日期格式应为 YYYY-MM-DD"})
		return
	}
	if !expireAt.After(startAt) {
		c.JSON(400, gin.H{"error": "授权截止日期必须晚于开始日期"})
		return
	}
	// 同一图片、同一申请人可以分别提交多次授权，各自独立管理期限
	app := models.Application{
		ImageID: req.ImageID, ApplicantID: u.ID,
		Purpose: strings.TrimSpace(req.Purpose), Channel: req.Channel,
		StartAt: startAt, ExpireAt: expireAt.Add(24*time.Hour - time.Second),
		Status: models.StatusPending, MaxResolution: models.ResPreview,
	}
	if err := s.db.Create(&app).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"item": s.appJSON(app, false)})
}

// reviewRequest 审批入参
type reviewRequest struct {
	Approve       bool   `json:"approve"`
	MaxResolution int    `json:"maxResolution"` // 1 预览 / 2 网页 / 3 原图
	CanEdit       bool   `json:"canEdit"`
	ExpireAt      string `json:"expireAt"` // 馆方可调整批准的期限
	Note          string `json:"note"`
}

// reviewApplication 审批。关键并发控制：
// UPDATE ... WHERE id=? AND status='pending' 在一个事务内条件更新，
// 影响行数为 0 即说明该申请已被其他审批动作处理，直接返回冲突。
// 因此无论多少个审批请求并发到达，最终状态只可能有一个一致结果。
func (s *Server) reviewApplication(c *gin.Context) {
	var req reviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "审批参数不完整"})
		return
	}
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	u := auth.CurrentUser(c)
	now := time.Now()

	res := req.MaxResolution
	if req.Approve && (res < models.ResPreview || res > models.ResPrint) {
		c.JSON(400, gin.H{"error": "分辨率等级必须为 1/2/3"})
		return
	}
	expire := (*time.Time)(nil)
	if req.Approve && req.ExpireAt != "" {
		t, err := time.Parse("2006-01-02", req.ExpireAt)
		if err != nil {
			c.JSON(400, gin.H{"error": "日期格式应为 YYYY-MM-DD"})
			return
		}
		t = t.Add(24*time.Hour - time.Second)
		expire = &t
	}

	var finalApp models.Application
	var applicant models.User
	var img models.Image

	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// 条件更新：只有仍处于 pending 的行会被改动
		q := tx.Model(&models.Application{}).Where("id = ? AND status = ?", id, models.StatusPending)
		var updates map[string]interface{}
		if req.Approve {
			updates = map[string]interface{}{
				"status":         models.StatusApproved,
				"max_resolution": res,
				"can_edit":       req.CanEdit,
				"reviewer_id":    u.ID,
				"reviewed_at":    now,
				"review_note":    req.Note,
			}
			if expire != nil {
				updates["expire_at"] = *expire
			}
		} else {
			updates = map[string]interface{}{
				"status": models.StatusRejected, "reviewer_id": u.ID,
				"reviewed_at": now, "review_note": req.Note,
			}
		}
		result := q.Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errReviewConflict
		}
		if err := tx.Preload("Image").Preload("Applicant").First(&finalApp, id).Error; err != nil {
			return err
		}
		if finalApp.Applicant != nil {
			applicant = *finalApp.Applicant
		}
		if finalApp.Image != nil {
			img = *finalApp.Image
		}
		return nil
	})

	if txErr == errReviewConflict {
		var current models.Application
		s.db.First(&current, id)
		c.JSON(409, gin.H{"error": "该申请已被其他审批动作处理（当前状态：" + current.Status + "）", "currentStatus": current.Status})
		return
	}
	if txErr != nil {
		c.JSON(500, gin.H{"error": txErr.Error()})
		return
	}

	if req.Approve {
		s.mailer.NotifyApproved(applicant.Name, applicant.Email, img.Title, res, req.CanEdit, finalApp.ExpireAt)
	} else {
		s.mailer.NotifyRejected(applicant.Name, applicant.Email, img.Title, req.Note)
	}
	c.JSON(200, gin.H{"item": s.appJSON(finalApp, false)})
}

// revokeApplication 馆方撤回已批准的授权。撤回立即生效：
// 状态写库后，所有旧下载链接在下一次取件时即被拒绝。
// 历史申请与发表记录不删除。
func (s *Server) revokeApplication(c *gin.Context) {
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	app, ok := s.loadApplication(c)
	if !ok {
		return
	}
	if app.Status != models.StatusApproved {
		c.JSON(409, gin.H{"error": "仅已批准、尚未失效的授权可以撤回（当前状态：" + app.Status + "）"})
		return
	}
	now := time.Now()
	if err := s.db.Model(&models.Application{}).Where("id = ? AND status = ?", app.ID, models.StatusApproved).
		Updates(map[string]interface{}{"status": models.StatusRevoked, "revoked_at": now, "revoke_reason": req.Reason}).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if app.Applicant != nil && app.Image != nil {
		s.mailer.NotifyRevoked(app.Applicant.Name, app.Applicant.Email, app.Image.Title, req.Reason)
	}
	c.JSON(200, gin.H{"ok": true})
}

// ---------- 下载 ----------

func (s *Server) mintDownloadToken(c *gin.Context) {
	u := auth.CurrentUser(c)
	app, ok := s.loadApplication(c)
	if !ok {
		return
	}
	if u.Role != models.RoleStaff && app.ApplicantID != u.ID {
		c.JSON(403, gin.H{"error": "无权下载该授权"})
		return
	}
	level, err := strconv.Atoi(c.DefaultQuery("level", "1"))
	if err != nil || level < models.ResPreview || level > models.ResPrint {
		c.JSON(400, gin.H{"error": "level 必须为 1/2/3"})
		return
	}
	// 换发链接时先做一次实时校验，撤回/过期/超分辨率直接拒绝，不签发链接
	if _, _, reason := s.checkDownload(app.ID, level); reason != "" {
		c.JSON(403, gin.H{"error": reason})
		return
	}
	exp := time.Now().Add(tokenTTL).Unix()
	payload := fmt.Sprintf("%d|%d|%d|%d", app.ID, level, u.ID, exp)
	tok := s.signToken(payload)
	c.JSON(200, gin.H{
		"url":       fmt.Sprintf("/api/download?token=%s", url.QueryEscape(tok)),
		"expiresIn": int(tokenTTL.Seconds()),
	})
}

func (s *Server) download(c *gin.Context) {
	tok := c.Query("token")
	appID, level, _, exp, err := s.verifyToken(tok)
	if err != nil {
		c.JSON(403, gin.H{"error": "下载链接无效"})
		return
	}
	_ = exp
	app, img, reason := s.checkDownload(appID, int(level))
	allowed := reason == ""
	uid := uint(0)
	if app != nil {
		uid = app.ApplicantID
	}
	record := models.DownloadRecord{ApplicationID: appID, UserID: uid, Level: int(level), Allowed: allowed, Reason: reason}
	_ = s.db.Create(&record).Error

	if !allowed {
		// 撤回 / 到期 / 越权：明确拒绝，绝不返回原图字节
		c.JSON(403, gin.H{"error": reason})
		return
	}
	path := filepath.Join(s.cfg.DataDir, levelPath(img, int(level)))
	if _, err := os.Stat(path); err != nil {
		c.JSON(404, gin.H{"error": "文件缺失"})
		return
	}
	filename := fmt.Sprintf("%s_%s_level%d.jpg", img.AccessionNo, img.Title, level)
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename))
	c.File(path)
}

// ---------- 发表登记 ----------

func (s *Server) createPublication(c *gin.Context) {
	u := auth.CurrentUser(c)
	app, ok := s.loadApplication(c)
	if !ok {
		return
	}
	if app.ApplicantID != u.ID && u.Role != models.RoleStaff {
		c.JSON(403, gin.H{"error": "只有申请人可以登记发表位置"})
		return
	}
	var req struct {
		Title       string `json:"title"`
		Publisher   string `json:"publisher"`
		URL         string `json:"url"`
		Channel     string `json:"channel"`
		PublishedAt string `json:"publishedAt"`
		Note        string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		c.JSON(400, gin.H{"error": "请至少填写页面地址"})
		return
	}
	pubAt := time.Now()
	if req.PublishedAt != "" {
		if t, err := time.Parse("2006-01-02", req.PublishedAt); err == nil {
			pubAt = t
		}
	}
	pub := models.Publication{
		ApplicationID: app.ID, Title: req.Title, Publisher: req.Publisher,
		URL: req.URL, Channel: req.Channel, PublishedAt: pubAt, Note: req.Note, CreatedBy: u.ID,
	}
	if err := s.db.Create(&pub).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	// 通知馆方（模拟邮件）
	var staff []models.User
	s.db.Where("role = ?", models.RoleStaff).Find(&staff)
	emails := make([]string, 0, len(staff))
	for _, st := range staff {
		emails = append(emails, st.Email)
	}
	if app.Image != nil {
		s.mailer.NotifyPublication(emails, u.Name, app.Image.Title, req.URL)
	}
	c.JSON(201, gin.H{"item": pub})
}

func (s *Server) listPublications(c *gin.Context) {
	u := auth.CurrentUser(c)
	var pubs []models.Publication
	q := s.db.Model(&models.Publication{}).
		Preload("Application").Preload("Application.Image").Preload("Application.Applicant")
	if u.Role != models.RoleStaff {
		// 申请人只能看自己的引用登记
		q = q.Joins("JOIN applications ON applications.id = publications.application_id").
			Where("applications.applicant_id = ?", u.ID)
	}
	if kw := c.Query("q"); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("publications.url LIKE ? OR publications.title LIKE ? OR publications.publisher LIKE ?", like, like, like)
	}
	if err := q.Order("published_at desc").Find(&pubs).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	items := make([]gin.H, 0, len(pubs))
	for i := range pubs {
		items = append(items, s.pubJSON(&pubs[i]))
	}
	c.JSON(200, gin.H{"items": items})
}

func (s *Server) pubJSON(p *models.Publication) gin.H {
	j := gin.H{
		"id": p.ID, "applicationId": p.ApplicationID,
		"title": p.Title, "publisher": p.Publisher, "url": p.URL,
		"channel": p.Channel, "publishedAt": p.PublishedAt, "note": p.Note, "createdAt": p.CreatedAt,
	}
	if p.Application != nil {
		aj := s.appJSON(*p.Application, false)
		j["application"] = aj
	}
	return j
}

func (s *Server) exportPublicationsCSV(c *gin.Context) {
	u := auth.CurrentUser(c)
	var pubs []models.Publication
	q := s.db.Model(&models.Publication{}).
		Preload("Application").Preload("Application.Image").Preload("Application.Applicant")
	if u.Role != models.RoleStaff {
		q = q.Joins("JOIN applications ON applications.id = publications.application_id").
			Where("applications.applicant_id = ?", u.ID)
	}
	if err := q.Order("published_at desc").Find(&pubs).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape("馆藏图片引用位置.csv"))
	// UTF-8 BOM，Excel 直接打开不乱码
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	w := newCSVWriter(c.Writer)
	_ = w.Write([]string{"登记时间", "发表时间", "藏品编号", "藏品名称", "申请人", "联系邮箱",
		"刊物/页面名称", "出版方/站点", "渠道", "引用地址", "授权当前状态", "备注"})
	for i := range pubs {
		p := &pubs[i]
		status := ""
		if p.Application != nil {
			status = p.Application.EffectiveState(time.Now())
		}
		row := []string{
			p.CreatedAt.Format("2006-01-02"),
			p.PublishedAt.Format("2006-01-02"),
			imageField(p, func(im *models.Image) string { return im.AccessionNo }),
			imageField(p, func(im *models.Image) string { return im.Title }),
			applicantField(p, func(us *models.User) string { return us.Name }),
			applicantField(p, func(us *models.User) string { return us.Email }),
			p.Title, p.Publisher, channelLabel(p.Channel), p.URL, status, p.Note,
		}
		_ = w.Write(row)
	}
	w.Flush()
}

func imageField(p *models.Publication, f func(*models.Image) string) string {
	if p.Application != nil && p.Application.Image != nil {
		return f(p.Application.Image)
	}
	return ""
}
func applicantField(p *models.Publication, f func(*models.User) string) string {
	if p.Application != nil && p.Application.Applicant != nil {
		return f(p.Application.Applicant)
	}
	return ""
}

func channelLabel(ch string) string {
	switch ch {
	case models.ChannelPrint:
		return "刊物"
	case models.ChannelWeb:
		return "网页"
	case models.ChannelSocial:
		return "新媒体"
	}
	return ch
}

// ---------- 模拟邮件 ----------

func (s *Server) listMails(c *gin.Context) {
	u := auth.CurrentUser(c)
	q := s.db.Model(&models.MailMessage{})
	if u.Role != models.RoleStaff {
		q = q.Where("to_email = ?", u.Email)
	}
	var msgs []models.MailMessage
	if err := q.Order("created_at desc").Limit(100).Find(&msgs).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"items": msgs})
}
