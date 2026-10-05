package models

import "time"

// 角色
const (
	RoleApplicant = "applicant" // 申请人
	RoleStaff     = "staff"     // 馆方审核人员
)

// User 系统用户：申请人或馆方人员
type User struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Username  string    `gorm:"uniqueIndex;not null" json:"username"`
	Name      string    `gorm:"not null" json:"name"`
	Email     string    `gorm:"not null" json:"email"`
	Role      string    `gorm:"not null;default:applicant" json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// 分辨率等级：数值越大权限越高
const (
	ResPreview = 1 // 预览图（缩略）
	ResWeb     = 2 // 网页用
	ResPrint   = 3 // 印刷/原图
)

// Image 馆藏图片。三级分辨率文件在入库时由系统生成。
type Image struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	AccessionNo string    `gorm:"uniqueIndex;not null" json:"accessionNo"` // 藏品编号
	Title       string    `gorm:"not null" json:"title"`
	Dynasty     string    `json:"dynasty"`
	Category    string    `json:"category"`
	PreviewPath string    `json:"-"` // 600px 预览
	WebPath     string    `json:"-"` // 1600px 网页用
	PrintPath   string    `json:"-"` // 原图
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	UploadedBy  uint      `json:"uploadedBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

// 申请 / 授权状态
const (
	StatusPending  = "pending"  // 待审批
	StatusApproved = "approved" // 已批准（可下载，未到期、未撤回）
	StatusRejected = "rejected" // 已拒绝
	StatusRevoked  = "revoked"  // 已撤回
	StatusExpired  = "expired"  // 已过期
)

// 用途渠道
const (
	ChannelPrint  = "print"  // 刊物
	ChannelWeb    = "web"    // 网页
	ChannelSocial = "social" // 新媒体
)

// Application 一次授权申请。同一图片可有多条申请，各自独立管理期限。
type Application struct {
	ID          uint   `gorm:"primarykey" json:"id"`
	ImageID     uint   `gorm:"index;not null" json:"imageId"`
	Image       *Image `gorm:"foreignKey:ImageID" json:"image,omitempty"`
	ApplicantID uint   `gorm:"index;not null" json:"applicantId"`
	Applicant   *User  `gorm:"foreignKey:ApplicantID" json:"applicant,omitempty"`

	Purpose  string    `gorm:"not null" json:"purpose"` // 用途说明
	Channel  string    `gorm:"not null" json:"channel"` // 刊物 / 网页 / 新媒体
	StartAt  time.Time `gorm:"not null" json:"startAt"`
	ExpireAt time.Time `gorm:"not null" json:"expireAt"` // 申请的期限

	// 审批结果
	Status        string     `gorm:"not null;default:pending;index" json:"status"`
	MaxResolution int        `gorm:"not null;default:1" json:"maxResolution"` // 批准的最高分辨率等级
	CanEdit       bool       `json:"canEdit"`                                 // 能否二次编辑
	ReviewerID    *uint      `json:"reviewerId"`
	ReviewedAt    *time.Time `json:"reviewedAt"`
	ReviewNote    string     `json:"reviewNote"`
	RevokedAt     *time.Time `json:"revokedAt"`
	RevokeReason  string     `json:"revokeReason"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// EffectiveState 返回当前实时状态（过期判断不依赖后台扫描）
func (a *Application) EffectiveState(now time.Time) string {
	switch a.Status {
	case StatusApproved:
		if now.After(a.ExpireAt) {
			return StatusExpired
		}
		return StatusApproved
	default:
		return a.Status
	}
}

// CanDownload 判断在给定时刻是否允许下载指定分辨率等级
func (a *Application) CanDownload(level int, now time.Time) bool {
	return a.EffectiveState(now) == StatusApproved && level <= a.MaxResolution
}

// Publication 申请人发表后登记的引用位置。授权失效后记录仍保留，供馆方查找。
type Publication struct {
	ID            uint         `gorm:"primarykey" json:"id"`
	ApplicationID uint         `gorm:"index;not null" json:"applicationId"`
	Application   *Application `gorm:"foreignKey:ApplicationID" json:"application,omitempty"`
	Title         string       `gorm:"not null" json:"title"` // 刊物名 / 页面名
	Publisher     string       `json:"publisher"`             // 出版方 / 站点
	URL           string       `gorm:"not null" json:"url"`   // 页面地址
	Channel       string       `json:"channel"`
	PublishedAt   time.Time    `gorm:"not null" json:"publishedAt"`
	Note          string       `json:"note"`
	CreatedBy     uint         `json:"createdBy"`
	CreatedAt     time.Time    `json:"createdAt"`
}

// DownloadRecord 每次成功/失败的原图取件尝试，便于审计
type DownloadRecord struct {
	ID            uint      `gorm:"primarykey" json:"id"`
	ApplicationID uint      `gorm:"index" json:"applicationId"`
	UserID        uint      `json:"userId"`
	Level         int       `json:"level"`
	Allowed       bool      `json:"allowed"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"createdAt"`
}

// MailMessage 模拟邮件：不真正发送，落库并可在页面查看
type MailMessage struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	ToEmail   string    `gorm:"not null" json:"toEmail"`
	Subject   string    `gorm:"not null" json:"subject"`
	Body      string    `gorm:"not null" json:"body"`
	Kind      string    `gorm:"not null" json:"kind"` // approved/rejected/expiring/revoked/publication
	CreatedAt time.Time `json:"createdAt"`
}
