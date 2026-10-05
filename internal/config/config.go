package config

import "os"

// Config 存放运行期配置，全部可用环境变量覆盖。
type Config struct {
	Port       string
	DBPath     string
	StorageDir string
	JWTSecret  string
	PreviewMax string // 预览图最长边像素
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Load 读取环境变量并补默认值。
func Load() Config {
	return Config{
		Port:       getenv("PORT", "8080"),
		DBPath:     getenv("DB_PATH", "data/museum.db"),
		StorageDir: getenv("STORAGE_DIR", "data/storage"),
		JWTSecret:  getenv("JWT_SECRET", "museum-license-dev-secret"),
		PreviewMax: getenv("PREVIEW_MAX_PX", "1200"),
	}
}
