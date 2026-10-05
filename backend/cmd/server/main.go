package main

import (
	"log"

	"museum-license/internal/api"
	"museum-license/internal/auth"
	"museum-license/internal/config"
	"museum-license/internal/mail"
	"museum-license/internal/store"
)

func main() {
	cfg := config.Load()
	db, err := store.Open(cfg)
	if err != nil {
		log.Fatalf("初始化失败: %v", err)
	}
	authMgr := auth.New(db)
	mailer := mail.New(db)
	srv := api.NewServer(db, cfg, authMgr, mailer)

	log.Printf("博物馆图片授权系统已启动： http://localhost:%s", cfg.Port)
	log.Printf("演示账号： curator(馆方) / alice、bob(申请人)")
	if err := srv.Router().Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
