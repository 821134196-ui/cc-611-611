// Package mail 模拟邮件服务：不接入 SMTP，所有“邮件”落库，前端“邮件记录”页可查。
package mail

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"museum-license/internal/models"
)

type Service struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) Send(to, kind, subject, body string) {
	msg := models.MailMessage{ToEmail: to, Kind: kind, Subject: subject, Body: body, CreatedAt: time.Now()}
	// 邮件失败不应阻断主流程
	_ = s.db.Create(&msg).Error
}

func (s *Service) NotifyApproved(name, email, imageTitle string, maxRes int, canEdit bool, expireAt time.Time) {
	level := map[int]string{1: "预览图（600px）", 2: "网页版（1600px）", 3: "原图/印刷版"}[maxRes]
	body := fmt.Sprintf("%s 您好：\n\n您申请使用的藏品图片《%s》已获批准。\n允许下载的最高分辨率：%s\n是否允许二次编辑：%t\n授权截止：%s\n\n请注意：获准下载不等于已获发表许可，正式发表后请在系统中登记引用位置。\n\n——博物馆授权管理系统",
		name, imageTitle, level, canEdit, expireAt.Format("2006-01-02"))
	s.Send(email, "approved", "您的图片授权申请已批准", body)
}

func (s *Service) NotifyRejected(name, email, imageTitle, note string) {
	body := fmt.Sprintf("%s 您好：\n\n您申请使用的藏品图片《%s》未获批准。\n审核说明：%s\n\n——博物馆授权管理系统", name, imageTitle, note)
	s.Send(email, "rejected", "您的图片授权申请未获批准", body)
}

func (s *Service) NotifyRevoked(name, email, imageTitle, reason string) {
	body := fmt.Sprintf("%s 您好：\n\n您对藏品图片《%s》的使用授权已被撤回。%s\n请立即停止使用并撤下已发布内容中超出合理引用范围的原图；此前历史申请与发表登记记录将依法保留备查。\n\n——博物馆授权管理系统",
		name, imageTitle, reason)
	s.Send(email, "revoked", "图片使用授权撤回通知", body)
}

func (s *Service) NotifyExpired(name, email, imageTitle string, expireAt time.Time) {
	body := fmt.Sprintf("%s 您好：\n\n您对藏品图片《%s》的使用授权已于 %s 到期，此后旧下载链接将不再提供原图。如需继续使用请重新提交申请。\n\n——博物馆授权管理系统",
		name, imageTitle, expireAt.Format("2006-01-02"))
	s.Send(email, "expiring", "图片使用授权到期通知", body)
}

func (s *Service) NotifyPublication(staffEmails []string, applicantName, imageTitle, url string) {
	body := fmt.Sprintf("申请人 %s 登记了藏品图片《%s》的发表位置：%s\n可在“发表位置”页面导出并据此联系使用方。",
		applicantName, imageTitle, url)
	for _, e := range staffEmails {
		s.Send(e, "publication", "新的图片引用位置已登记", body)
	}
}
