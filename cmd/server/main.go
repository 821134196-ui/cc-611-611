package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"museumlicense/internal/api"
	"museumlicense/internal/auth"
	"museumlicense/internal/config"
	"museumlicense/internal/database"
	"museumlicense/internal/email"
	"museumlicense/internal/seed"
	"museumlicense/internal/services"
	museumweb "museumlicense/web"
)

func main() {
	cfg := config.Load()

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}
	if err := os.MkdirAll(cfg.StorageDir, 0o755); err != nil {
		log.Fatalf("创建存储目录失败: %v", err)
	}

	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	if err := seed.Run(db, cfg.StorageDir); err != nil {
		log.Fatalf("种子数据初始化失败: %v", err)
	}

	mailer := email.New(db)
	licSvc := services.NewLicenseService(db, mailer)
	pubSvc := services.NewPublicationService(db)
	authMgr := auth.NewManager(cfg.JWTSecret)

	h := &api.Handlers{
		DB:      db,
		Lic:     licSvc,
		Pub:     pubSvc,
		Mailer:  mailer,
		Auth:    authMgr,
		Storage: cfg.StorageDir,
	}
	router := h.Router(museumweb.Dist)

	stop := make(chan struct{})
	go api.StartExpirySweeper(h, time.Minute, stop)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("博物馆藏品图片授权系统已启动: http://localhost:%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	close(stop)
	log.Println("正在关闭服务...")
}
