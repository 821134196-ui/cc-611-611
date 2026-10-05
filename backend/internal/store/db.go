package store

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"museum-license/internal/config"
	"museum-license/internal/media"
	"museum-license/internal/models"
)

// Open 打开/创建 SQLite 数据库并执行迁移
func Open(cfg config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(cfg.DBPath()+"?_journal_mode=WAL&_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// SQLite 同一时刻只允许一个写事务；单连接写串行化，
	// 配合审批语句的 WHERE status='pending' 条件更新，并发审批结果确定无矛盾。
	if sqlDB, e := db.DB(); e == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Image{},
		&models.Application{},
		&models.Publication{},
		&models.DownloadRecord{},
		&models.MailMessage{},
	); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := seed(db, cfg); err != nil {
		return nil, fmt.Errorf("seed: %w", err)
	}
	return db, nil
}

// seed 首次启动写入演示账号、藏品图片与若干授权样例
func seed(db *gorm.DB, cfg config.Config) error {
	var userCount int64
	db.Model(&models.User{}).Count(&userCount)
	if userCount > 0 {
		return nil
	}

	users := []models.User{
		{Username: "curator", Name: "王馆长", Email: "curator@museum.example", Role: models.RoleStaff},
		{Username: "alice", Name: "李清照（编辑）", Email: "alice@press.example", Role: models.RoleApplicant},
		{Username: "bob", Name: "陈博文（自媒体）", Email: "bob@web.example", Role: models.RoleApplicant},
	}
	if err := db.Create(&users).Error; err != nil {
		return err
	}
	staffID, aliceID, bobID := users[0].ID, users[1].ID, users[2].ID

	type spec struct {
		AccessionNo, Title, Dynasty, Category string
		Width, Height                         int
	}
	specs := []spec{
		{"GU-0001", "青花缠枝莲纹罐", "明·永乐", "陶瓷器", 2400, 3000},
		{"GU-0002", "鎏金铁芯铜龙", "唐", "金属器", 2800, 1800},
		{"GU-0003", "玉琮", "新石器时代·良渚", "玉器", 2000, 2600},
		{"GU-0004", "三彩骆驼载乐俑", "唐", "陶俑", 2200, 2800},
		{"GU-0005", "霁蓝釉白龙纹梅瓶", "元", "陶瓷器", 2000, 3200},
		{"GU-0006", "越王州句剑", "春秋晚期", "兵器", 3000, 900},
	}

	now := time.Now()
	for i, s := range specs {
		base := fmt.Sprintf("img_%d_%d", i+1, now.UnixMilli())
		src := media.Placeholder(i+1, s.Title, s.Width, s.Height)
		prev, web, print, w, h, err := media.Encode(src, cfg.ImageDir(), base)
		if err != nil {
			return err
		}
		rel := func(p string) string { p, _ = filepath.Rel(cfg.DataDir, p); return p }
		img := models.Image{
			AccessionNo: s.AccessionNo, Title: s.Title, Dynasty: s.Dynasty, Category: s.Category,
			PreviewPath: rel(prev), WebPath: rel(web), PrintPath: rel(print),
			Width: w, Height: h, UploadedBy: staffID,
		}
		if err := db.Create(&img).Error; err != nil {
			return err
		}

		// 每个藏品挂两条不同期限/状态的申请，便于直接看到三个分区
		apps := []models.Application{
			{
				ImageID: img.ID, ApplicantID: aliceID,
				Purpose: "《馆藏精粹》年度图录跨页配图", Channel: models.ChannelPrint,
				StartAt: now.AddDate(0, -1, 0), ExpireAt: now.AddDate(0, 6, 0),
				Status: models.StatusApproved, MaxResolution: models.ResPrint, CanEdit: false,
				ReviewerID: &staffID, ReviewedAt: ptrTime(now.AddDate(0, -1, 1)),
				ReviewNote: "限图录一次印刷，不得转授",
			},
			{
				ImageID: img.ID, ApplicantID: bobID,
				Purpose: "博物馆公众号推文插图", Channel: models.ChannelSocial,
				StartAt: now.AddDate(0, 0, -7), ExpireAt: now.AddDate(0, 0, 14),
				Status: models.StatusPending, MaxResolution: models.ResPreview,
			},
		}
		if err := db.Create(&apps).Error; err != nil {
			return err
		}

		// 给第一张图的已批准申请补一条已登记发表位置
		if i == 0 {
			pub := models.Publication{
				ApplicationID: apps[0].ID,
				Title:         "《馆藏精粹（2026）》第 42 页",
				Publisher:     "艺文出版社",
				URL:           "https://press.example.com/catalog/2026/p42",
				Channel:       models.ChannelPrint,
				PublishedAt:   now.AddDate(0, 0, -20),
				Note:          "首版第一次印刷 3000 册",
				CreatedBy:     aliceID,
			}
			if err := db.Create(&pub).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func ptrTime(t time.Time) *time.Time { return &t }
