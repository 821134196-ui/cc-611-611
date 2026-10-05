package models

import (
	"time"

	"gorm.io/gorm"
)

// 申请 / 授权状态机：
// pending 待审批 → approved 已批准（可下载）
//
//	→ rejected 已驳回
//
// approved → revoked 已撤回（馆方操作）
// approved → expired 已过期（到期巡检或实时判定）
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
	StatusRevoked  = "revoked"
	StatusExpired  = "expired"
)

// 分辨率档位：preview 仅预览图，original 原图（同时允许预览图）。
const (
	TierPreview  = "preview"
	TierOriginal = "original"
)

const (
	RoleApplicant = "applicant" // 申请人 / 使用方
	RoleCurator   = "curator"   // 馆方审批人
)

// User 同时表示申请人与馆方账号，Role 区分权限。
type User struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Username  string         `gorm:"uniqueIndex;not null" json:"username"`
	Password  string         `json:"-"`
	Role      string         `gorm:"not null;default:applicant" json:"role"`
	Email     string         `json:"email"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Artifact 藏品及其本地原图路径；预览图由系统按原图生成。
type Artifact struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	Code         string         `gorm:"uniqueIndex;not null" json:"code"`
	Name         string         `gorm:"not null" json:"name"`
	Dynasty      string         `json:"dynasty"`
	Category     string         `json:"category"`
	Description  string         `json:"description"`
	OriginalPath string         `json:"original_path"`
	PreviewPath  string         `json:"preview_path"`
	OriginalW    int            `json:"original_w"`
	OriginalH    int            `json:"original_h"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// Application 一条图片使用授权申请；审批通过后它本身即“授权”。
// 同一藏品的不同申请各自独立管理到期时间与撤回状态。
type Application struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ArtifactID  uint      `gorm:"not null;index" json:"artifact_id"`
	Artifact    *Artifact `gorm:"foreignKey:ArtifactID" json:"artifact,omitempty"`
	ApplicantID uint      `gorm:"not null;index" json:"applicant_id"`
	Applicant   *User     `gorm:"foreignKey:ApplicantID" json:"applicant,omitempty"`

	// 申请填报
	Purpose       string    `gorm:"not null" json:"purpose"` // 用途说明
	Channel       string    `gorm:"not null" json:"channel"` // 刊物 / 网页等渠道
	RequestedTier string    `gorm:"not null;default:preview" json:"requested_tier"`
	RequestedEdit bool      `json:"requested_edit"` // 申请是否需要二次编辑
	StartAt       time.Time `gorm:"not null" json:"start_at"`
	EndAt         time.Time `gorm:"not null" json:"end_at"` // 申请人期望的授权期限

	// 审批结果
	Status         string     `gorm:"not null;default:pending;index" json:"status"`
	ResolutionTier string     `gorm:"default:preview" json:"resolution_tier"` // 馆方核定的分辨率
	AllowEdit      bool       `json:"allow_edit"`                             // 馆方核定能否二次编辑
	ExpiresAt      *time.Time `gorm:"index" json:"expires_at"`                // 授权到期时间（独立于申请期望期限）
	ReviewerID     *uint      `json:"reviewer_id"`
	Reviewer       *User      `gorm:"foreignKey:ReviewerID" json:"reviewer,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at"`
	ReviewNote     string     `json:"review_note"`
	RevokedAt      *time.Time `json:"revoked_at"`
	RevokeReason   string     `json:"revoke_reason"`

	// Version 乐观锁版本号：审批 / 撤回按“当前状态 + 版本”条件更新，
	// 并发决策只有一个条件成立，保证同一申请不会留下互相矛盾的结果。
	Version int `gorm:"not null;default:0" json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EffectiveStatus 返回考虑时间后的真实状态。
// 巡检任务可能尚未把 approved 改成 expired，但下载鉴权必须实时判定。
func (a Application) EffectiveStatus(now time.Time) string {
	if a.Status == StatusApproved && a.ExpiresAt != nil && now.After(*a.ExpiresAt) {
		return StatusExpired
	}
	return a.Status
}

// Publication 申请人发表后回填的引用位置（刊物条目或页面地址）。
// 授权撤回 / 过期后记录仍然保留，馆方据此联系使用方。
type Publication struct {
	ID            uint         `gorm:"primaryKey" json:"id"`
	ApplicationID uint         `gorm:"not null;index" json:"application_id"`
	Application   *Application `gorm:"foreignKey:ApplicationID" json:"application,omitempty"`
	ArtifactID    uint         `gorm:"not null;index" json:"artifact_id"`
	Artifact      *Artifact    `gorm:"foreignKey:ArtifactID" json:"artifact,omitempty"`
	ApplicantID   uint         `gorm:"not null;index" json:"applicant_id"`
	Title         string       `gorm:"not null" json:"title"`
	URL           string       `gorm:"not null" json:"url"`
	PublishedAt   time.Time    `json:"published_at"`
	Note          string       `json:"note"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

// DownloadLog 每次下载尝试（含被拒绝的尝试）留痕。
type DownloadLog struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ApplicationID uint      `gorm:"not null;index" json:"application_id"`
	UserID        uint      `gorm:"not null" json:"user_id"`
	Resolution    string    `gorm:"not null" json:"resolution"`
	Granted       bool      `gorm:"not null" json:"granted"`
	Reason        string    `json:"reason"`
	IP            string    `json:"ip"`
	CreatedAt     time.Time `json:"created_at"`
}

// EmailMessage 模拟邮件：不真正发信，写入本表并在前端“邮件提醒”页可查。
type EmailMessage struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ToUserID  uint      `gorm:"not null;index" json:"to_user_id"`
	ToAddress string    `json:"to_address"`
	Subject   string    `gorm:"not null" json:"subject"`
	Body      string    `gorm:"not null" json:"body"`
	Kind      string    `gorm:"not null" json:"kind"`
	CreatedAt time.Time `json:"created_at"`
}
