package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port   int
	DBPath string
}

func Load() (*Config, error) {
	return &Config{
		Port:   getEnvInt("PORT", 3847),
		DBPath: getEnvStr("DB_PATH", "2L1nk.db"),
	}, nil
}

func getEnvStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var out int
		_, err := fmt.Sscanf(v, "%d", &out)
		if err == nil {
			return out
		}
	}
	return def
}
