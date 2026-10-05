// Package email 提供模拟邮件接口：不接入真实 SMTP，邮件落库并可在页面查看。
package email

import (
	"fmt"

	"gorm.io/gorm"

	"museumlicense/internal/models"
)

type Mailer struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Mailer {
	return &Mailer{db: db}
}

// Send 把一封“邮件”写入 email_messages 表。
func (m *Mailer) Send(to *models.User, kind, subject, body string) error {
	msg := models.EmailMessage{
		ToUserID:  to.ID,
		ToAddress: to.Email,
		Subject:   subject,
		Body:      body,
		Kind:      kind,
	}
	return m.db.Create(&msg).Error
}

// Convenience 构造器 ------------------------------------------------

func (m *Mailer) NotifyApproved(to *models.User, a *models.Application) {
	subject := fmt.Sprintf("【授权通过】藏品 #%d 使用申请已批准", a.ArtifactID)
	body := fmt.Sprintf(
		"您关于藏品 #%d 的申请（用途：%s，渠道：%s）已获批准。\n"+
			"允许下载分辨率：%s；是否可二次编辑：%t。\n"+
			"授权到期时间：%s。\n"+
			"请在实际发表后及时回填引用位置。",
		a.ArtifactID, a.Purpose, a.Channel,
		a.ResolutionTier, a.AllowEdit,
		a.ExpiresAt.Format("2006-01-02 15:04:05"))
	_ = m.Send(to, "approved", subject, body)
}

func (m *Mailer) NotifyRejected(to *models.User, a *models.Application) {
	subject := fmt.Sprintf("【授权驳回】藏品 #%d 使用申请", a.ArtifactID)
	body := fmt.Sprintf("您关于藏品 #%d 的申请已被驳回。原因：%s", a.ArtifactID, a.ReviewNote)
	_ = m.Send(to, "rejected", subject, body)
}

func (m *Mailer) NotifyRevoked(to *models.User, a *models.Application) {
	subject := fmt.Sprintf("【授权撤回】藏品 #%d 的下载授权已停止", a.ArtifactID)
	body := fmt.Sprintf(
		"您关于藏品 #%d 的授权已被撤回，原图下载链接即刻失效。\n"+
			"撤回原因：%s\n"+
			"已发表的引用位置仍需按协议处理，馆方将依据您登记的位置与您联系。",
		a.ArtifactID, a.RevokeReason)
	_ = m.Send(to, "revoked", subject, body)
}

func (m *Mailer) NotifyExpired(to *models.User, a *models.Application) {
	subject := fmt.Sprintf("【授权到期】藏品 #%d 的下载授权已结束", a.ArtifactID)
	body := fmt.Sprintf("您关于藏品 #%d 的授权已于 %s 到期，原图下载链接已停止提供。",
		a.ArtifactID, a.ExpiresAt.Format("2006-01-02 15:04:05"))
	_ = m.Send(to, "expired", subject, body)
}
