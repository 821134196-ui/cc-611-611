// Package services 承载授权状态机、下载鉴权与发表登记的核心业务逻辑。
package services

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"museumlicense/internal/email"
	"museumlicense/internal/models"
)

// 业务错误，API 层据此映射状态码（冲突 / 无权 / 未找到）。
var (
	ErrNotFound         = errors.New("记录不存在")
	ErrConflict         = errors.New("申请状态已被其他操作改变")
	ErrNotPending       = errors.New("该申请已被处理，不能重复审批")
	ErrNotApproved      = errors.New("授权当前无效，无法下载")
	ErrTierNotGranted   = errors.New("本次授权未获准下载该分辨率")
	ErrForbidden        = errors.New("无权操作该申请")
	ErrInvalidInput     = errors.New("参数不合法")
	ErrAlreadyPublished = errors.New("该网址已登记过")
)

type LicenseService struct {
	db     *gorm.DB
	mailer *email.Mailer
	Now    func() time.Time // 可在测试中替换为固定时钟
}

func NewLicenseService(db *gorm.DB, mailer *email.Mailer) *LicenseService {
	return &LicenseService{db: db, mailer: mailer, Now: time.Now}
}

// CreateApplicationInput 申请人提交的内容。
type CreateApplicationInput struct {
	ArtifactID    uint
	ApplicantID   uint
	Purpose       string
	Channel       string
	RequestedTier string
	RequestedEdit bool
	StartAt       time.Time
	EndAt         time.Time
}

// CreateApplication 建立一条待审批申请。
func (s *LicenseService) CreateApplication(in CreateApplicationInput) (*models.Application, error) {
	if in.ArtifactID == 0 || in.Purpose == "" || in.Channel == "" {
		return nil, ErrInvalidInput
	}
	if in.RequestedTier != models.TierPreview && in.RequestedTier != models.TierOriginal {
		in.RequestedTier = models.TierPreview
	}
	if in.EndAt.Before(in.StartAt) || in.EndAt.Before(s.Now()) {
		return nil, ErrInvalidInput
	}
	var art models.Artifact
	if err := s.db.First(&art, in.ArtifactID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	app := models.Application{
		ArtifactID:    in.ArtifactID,
		ApplicantID:   in.ApplicantID,
		Purpose:       in.Purpose,
		Channel:       in.Channel,
		RequestedTier: in.RequestedTier,
		RequestedEdit: in.RequestedEdit,
		StartAt:       in.StartAt,
		EndAt:         in.EndAt,
		Status:        models.StatusPending,
	}
	if err := s.db.Create(&app).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

// GetApplication 读取申请（含关联）。
func (s *LicenseService) GetApplication(id uint) (*models.Application, error) {
	var app models.Application
	err := s.db.Preload("Artifact").Preload("Applicant").Preload("Reviewer").First(&app, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &app, err
}

// DecisionInput 馆方审批结果。
type DecisionInput struct {
	Approve        bool
	ResolutionTier string // 核定允许的最高分辨率：preview / original
	AllowEdit      bool
	ExpiresAt      time.Time
	Note           string
	ReviewerID     uint
}

// Decide 审批一条申请。
//
// 并发一致性：审批结果通过单条“带状态条件的 UPDATE”落库——
//
//	UPDATE applications SET status=?, ... WHERE id=? AND status='pending'
//
// SQLite 的写事务串行化，两个审批人同时操作同一申请时，只有一个
// UPDATE 能匹配到 pending 行（RowsAffected=1）；另一个匹配 0 行，
// 收到 ErrConflict，页面提示“已被处理”。因此不可能同时留下批准与驳回
// 两条互相矛盾的结果。
func (s *LicenseService) Decide(appID uint, in DecisionInput) (*models.Application, error) {
	if in.Approve {
		if in.ResolutionTier != models.TierPreview && in.ResolutionTier != models.TierOriginal {
			return nil, ErrInvalidInput
		}
		if in.ExpiresAt.Before(s.Now()) {
			return nil, ErrInvalidInput
		}
	}

	var app models.Application
	if err := s.db.First(&app, appID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if app.Status != models.StatusPending {
		// 进入条件更新前已被他人抢先处理，直接给出冲突。
		return nil, ErrNotPending
	}

	now := s.Now()
	target := models.StatusApproved
	if !in.Approve {
		target = models.StatusRejected
	}

	updates := map[string]interface{}{
		"status":      target,
		"reviewer_id": in.ReviewerID,
		"reviewed_at": now,
		"review_note": in.Note,
		"version":     gorm.Expr("version + 1"),
	}
	if in.Approve {
		updates["resolution_tier"] = in.ResolutionTier
		updates["allow_edit"] = in.AllowEdit
		updates["expires_at"] = in.ExpiresAt
	}

	res := s.db.Model(&models.Application{}).
		Where("id = ? AND status = ?", appID, models.StatusPending).
		Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		// 并发审批：条件未匹配，说明另一个审批已经生效。
		return nil, ErrConflict
	}

	final, err := s.GetApplication(appID)
	if err != nil {
		return nil, err
	}

	var applicant models.User
	_ = s.db.First(&applicant, final.ApplicantID).Error
	if in.Approve {
		s.mailer.NotifyApproved(&applicant, final)
	} else {
		s.mailer.NotifyRejected(&applicant, final)
	}
	return final, nil
}

// Revoke 馆方撤回一条已生效授权。
// 同样使用状态条件 UPDATE：approved → revoked 是一次原子跃迁，
// 撤回后下载鉴权立即失败，且重复撤回 / 撤回过期授权都会得到冲突。
func (s *LicenseService) Revoke(appID uint, reviewerID uint, reason string) (*models.Application, error) {
	now := s.Now()
	res := s.db.Model(&models.Application{}).
		Where("id = ? AND status = ?", appID, models.StatusApproved).
		Updates(map[string]interface{}{
			"status":        models.StatusRevoked,
			"revoked_at":    now,
			"revoke_reason": reason,
			"reviewer_id":   reviewerID,
			"version":       gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		// 可能：不存在 / 已撤回 / 已过期 / 尚未批准。
		var app models.Application
		if err := s.db.First(&app, appID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if app.Status == models.StatusPending {
			return nil, ErrNotPending
		}
		return nil, ErrConflict
	}

	final, err := s.GetApplication(appID)
	if err != nil {
		return nil, err
	}
	var applicant models.User
	_ = s.db.First(&applicant, final.ApplicantID).Error
	s.mailer.NotifyRevoked(&applicant, final)
	return final, nil
}

// SweepExpired 把已到 ExpiresAt 但仍为 approved 的授权批量标记为 expired，
// 并给申请人发模拟到期提醒。返回新过期的申请列表。
func (s *LicenseService) SweepExpired() ([]models.Application, error) {
	now := s.Now()
	var candidates []models.Application
	if err := s.db.Where("status = ? AND expires_at < ?", models.StatusApproved, now).
		Find(&candidates).Error; err != nil {
		return nil, err
	}
	var newly []models.Application
	for i := range candidates {
		app := &candidates[i]
		res := s.db.Model(&models.Application{}).
			Where("id = ? AND status = ? AND expires_at < ?", app.ID, models.StatusApproved, now).
			Updates(map[string]interface{}{
				"status":  models.StatusExpired,
				"version": gorm.Expr("version + 1"),
			})
		if res.Error != nil {
			return nil, res.Error
		}
		if res.RowsAffected == 0 {
			continue // 并发撤回 / 其他巡检实例抢先处理
		}
		var applicant models.User
		_ = s.db.First(&applicant, app.ApplicantID).Error
		s.mailer.NotifyExpired(&applicant, app)
		newly = append(newly, *app)
	}
	return newly, nil
}

// DownloadCheck 下载鉴权结果。鉴权每次实时执行，不存在长期有效的旧链接——
// 撤回或过期后的下一次请求即刻被拒。
type DownloadCheck struct {
	Application *models.Application
	FilePath    string
	FileName    string
}

// AuthorizeDownload 校验某申请能否下载指定分辨率。
// resolution: preview（预览图）或 original（原图）。
func (s *LicenseService) AuthorizeDownload(appID, userID uint, role, resolution string) (*DownloadCheck, error) {
	if resolution != models.TierPreview && resolution != models.TierOriginal {
		return nil, ErrInvalidInput
	}
	app, err := s.GetApplication(appID)
	if err != nil {
		return nil, err
	}
	if role != models.RoleCurator && app.ApplicantID != userID {
		return nil, ErrForbidden
	}

	switch app.EffectiveStatus(s.Now()) {
	case models.StatusPending, models.StatusRejected:
		s.logDownload(appID, userID, resolution, false, "授权尚未生效")
		return nil, ErrNotApproved
	case models.StatusRevoked:
		s.logDownload(appID, userID, resolution, false, "授权已撤回")
		return nil, ErrNotApproved
	case models.StatusExpired:
		// 巡检可能还没跑，这里顺手把状态持久化（失败不影响本次拒绝）。
		_ = s.db.Model(&models.Application{}).
			Where("id = ? AND status = ? AND expires_at < ?", appID, models.StatusApproved, s.Now()).
			Update("status", models.StatusExpired).Error
		s.logDownload(appID, userID, resolution, false, "授权已过期")
		return nil, ErrNotApproved
	}

	// 状态为 approved 且未到期。
	if resolution == models.TierOriginal && app.ResolutionTier != models.TierOriginal {
		s.logDownload(appID, userID, resolution, false, "未获准原图分辨率")
		return nil, ErrTierNotGranted
	}

	var path, name string
	if resolution == models.TierOriginal {
		path, name = app.Artifact.OriginalPath, "original_"+app.Artifact.Code
	} else {
		path, name = app.Artifact.PreviewPath, "preview_"+app.Artifact.Code+".jpg"
		if path == "" {
			path = app.Artifact.OriginalPath // 未生成预览时降级（种子数据通常已生成）
		}
	}
	s.logDownload(appID, userID, resolution, true, "")
	return &DownloadCheck{Application: app, FilePath: path, FileName: name}, nil
}

func (s *LicenseService) logDownload(appID, userID uint, resolution string, granted bool, reason string) {
	_ = s.db.Create(&models.DownloadLog{
		ApplicationID: appID,
		UserID:        userID,
		Resolution:    resolution,
		Granted:       granted,
		Reason:        reason,
	}).Error
}

// ListApplications 按归属、状态、藏品过滤。
// applicantID > 0 时只看该申请人；status / artifactID 为空 / 0 表示不过滤。
func (s *LicenseService) ListApplications(applicantID uint, status string, artifactID uint) ([]models.Application, error) {
	q := s.db.Preload("Artifact").Preload("Applicant").Order("applications.id DESC")
	if applicantID > 0 {
		q = q.Where("applicant_id = ?", applicantID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if artifactID > 0 {
		q = q.Where("artifact_id = ?", artifactID)
	}
	var apps []models.Application
	err := q.Find(&apps).Error
	return apps, err
}
