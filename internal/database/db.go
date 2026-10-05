package database

import (
	"log"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"museumlicense/internal/models"
)

// Open 打开 / 创建 SQLite 数据库，启用 WAL 与忙等待以支持并发读写，
// 并执行自动迁移。
func Open(path string) (*gorm.DB, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// 连接池保留少量并发连接；写冲突由 busy_timeout 排队，
	// 业务层的条件更新保证最终状态一致。
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(4)

	if err := autoMigrate(db); err != nil {
		return nil, err
	}
	return db, nil
}

func autoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&models.User{},
		&models.Artifact{},
		&models.Application{},
		&models.Publication{},
		&models.DownloadLog{},
		&models.EmailMessage{},
	); err != nil {
		log.Printf("auto migrate failed: %v", err)
		return err
	}
	return nil
}
