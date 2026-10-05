package services

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"museumlicense/internal/models"
)

// PublicationService 管理“已发表引用位置”的登记、检索与导出。
// 关键点：发表记录独立于授权状态存在，撤回 / 过期 / 驳回都不删除，
// 馆方随时能据此找到已经发出的引用并联系使用方。
type PublicationService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewPublicationService(db *gorm.DB) *PublicationService {
	return &PublicationService{db: db, now: time.Now}
}

type RegisterPublicationInput struct {
	ApplicationID uint
	UserID        uint
	Title         string
	URL           string
	PublishedAt   time.Time
	Note          string
}

// Register 申请人在实际发表后回填引用位置。
// 获准下载不等于已发表，所以发表登记是独立动作；
// 即使授权随后被撤回 / 过期，已登记记录也不会被清除。
func (s *PublicationService) Register(in RegisterPublicationInput) (*models.Publication, error) {
	if in.ApplicationID == 0 || in.Title == "" || in.URL == "" {
		return nil, ErrInvalidInput
	}
	var app models.Application
	if err := s.db.First(&app, in.ApplicationID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if app.ApplicantID != in.UserID {
		return nil, ErrForbidden
	}

	// 同一申请同一网址不允许重复登记。
	var n int64
	if err := s.db.Model(&models.Publication{}).
		Where("application_id = ? AND url = ?", in.ApplicationID, in.URL).
		Count(&n).Error; err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrAlreadyPublished
	}

	if in.PublishedAt.IsZero() {
		in.PublishedAt = s.now()
	}
	pub := models.Publication{
		ApplicationID: in.ApplicationID,
		ArtifactID:    app.ArtifactID,
		ApplicantID:   app.ApplicantID,
		Title:         in.Title,
		URL:           in.URL,
		PublishedAt:   in.PublishedAt,
		Note:          in.Note,
	}
	if err := s.db.Create(&pub).Error; err != nil {
		return nil, err
	}
	return &pub, nil
}

// PublicationFilter 馆方检索条件，全部可选。
type PublicationFilter struct {
	ArtifactID   uint
	ArtifactCode string // 按藏品编号过滤
	Applicant    string // 用户名模糊匹配
	Keyword      string // 标题 / 网址关键字
	// IncludeInactive 默认包含已撤回 / 过期授权下的发表位置——这是核心需求。
	OnlyActive bool
}

// SearchForApplicant 在 Search 基础上做角色限定：申请人只能查自己的。
func (s *PublicationService) SearchForApplicant(userID uint, role string, f PublicationFilter) ([]models.Publication, error) {
	if role == models.RoleApplicant {
		f.Applicant = ""
		f.OnlyActive = false
	}
	q := s.buildQuery(f)
	if role == models.RoleApplicant {
		q = q.Where("publications.applicant_id = ?", userID)
	}
	var pubs []models.Publication
	err := q.Find(&pubs).Error
	return pubs, err
}

// Search 检索发表位置，带申请与藏品信息，便于馆方核对授权状态。
func (s *PublicationService) Search(f PublicationFilter) ([]models.Publication, error) {
	var pubs []models.Publication
	err := s.buildQuery(f).Find(&pubs).Error
	return pubs, err
}

func (s *PublicationService) buildQuery(f PublicationFilter) *gorm.DB {
	q := s.db.Model(&models.Publication{}).
		Preload("Application").
		Preload("Application.Artifact").
		Preload("Application.Applicant").
		Preload("Artifact").
		Order("publications.id DESC")
	if f.ArtifactID > 0 {
		q = q.Where("publications.artifact_id = ?", f.ArtifactID)
	}
	if f.ArtifactCode != "" {
		q = q.Joins("JOIN artifacts ON artifacts.id = publications.artifact_id").
			Where("artifacts.code = ?", f.ArtifactCode)
	}
	if f.Applicant != "" {
		q = q.Joins("JOIN users ON users.id = publications.applicant_id").
			Where("users.username LIKE ?", "%"+f.Applicant+"%")
	}
	if f.Keyword != "" {
		like := "%" + f.Keyword + "%"
		q = q.Where("publications.title LIKE ? OR publications.url LIKE ?", like, like)
	}
	if f.OnlyActive {
		q = q.Joins("JOIN applications ON applications.id = publications.application_id").
			Where("applications.status = ? AND (applications.expires_at IS NULL OR applications.expires_at > ?)",
				models.StatusApproved, s.now())
	}
	return q
}

// ListByApplication 申请人查看自己某条申请下登记的位置（任何授权状态都可查）。
func (s *PublicationService) ListByApplication(appID, userID uint, role string) ([]models.Publication, error) {
	if role != models.RoleCurator {
		var app models.Application
		if err := s.db.First(&app, appID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if app.ApplicantID != userID {
			return nil, ErrForbidden
		}
	}
	var pubs []models.Publication
	err := s.db.Where("application_id = ?", appID).Order("id DESC").Find(&pubs).Error
	return pubs, err
}

// CSVHeader / Row 用于导出，字段用中文表头方便馆方直接使用。
const CSVHeader = "藏品编号,藏品名称,申请人,授权状态,用途,渠道,引用标题,发表位置URL,发表时间,登记时间,撤回原因\n"

// CSVRow 输出一行 CSV；授权状态字段让馆方一眼看出哪些引用是在撤回/过期后仍挂在网上的。
func CSVRow(p models.Publication) string {
	app := p.Application
	code, name := "", ""
	status, purpose, channel, revokeReason := "", "", "", ""
	if app != nil {
		purpose = app.Purpose
		channel = app.Channel
		revokeReason = app.RevokeReason
		if app.Artifact != nil {
			code = app.Artifact.Code
			name = app.Artifact.Name
		}
		if app.ID != 0 {
			status = app.EffectiveStatus(time.Now())
		}
	}
	return fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n",
		csvEscape(code), csvEscape(name), csvEscape(applicantName(app)),
		csvEscape(status), csvEscape(purpose), csvEscape(channel),
		csvEscape(p.Title), csvEscape(p.URL),
		p.PublishedAt.Format("2006-01-02"),
		p.CreatedAt.Format("2006-01-02 15:04"),
		csvEscape(revokeReason))
}

func applicantName(app *models.Application) string {
	if app != nil && app.Applicant != nil {
		return app.Applicant.Username
	}
	return ""
}

func csvEscape(s string) string {
	needsQuote := false
	for _, r := range s {
		if r == ',' || r == '"' || r == '\n' || r == '\r' {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return s
	}
	out := make([]rune, 0, len(s)+2)
	out = append(out, '"')
	for _, r := range s {
		if r == '"' {
			out = append(out, '"', '"')
		} else {
			out = append(out, r)
		}
	}
	out = append(out, '"')
	return string(out)
}
