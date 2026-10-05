package config

import (
	"os"
	"path/filepath"
)

// Config 运行配置，均可通过环境变量覆盖
type Config struct {
	Port      string
	DataDir   string // SQLite 数据库与图片根目录
	StaticDir string // 前端构建产物目录
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() Config {
	cwd, _ := os.Getwd()
	dataDir := getenv("DATA_DIR", filepath.Join(cwd, "data"))
	staticDir := getenv("STATIC_DIR", filepath.Join(cwd, "..", "frontend", "dist"))
	_ = os.MkdirAll(filepath.Join(dataDir, "images"), 0o755)
	return Config{
		Port:      getenv("PORT", "8080"),
		DataDir:   dataDir,
		StaticDir: staticDir,
	}
}

func (c Config) DBPath() string   { return filepath.Join(c.DataDir, "museum.db") }
func (c Config) ImageDir() string { return filepath.Join(c.DataDir, "images") }
