package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port int
}

func Load() (*Config, error) {
	return &Config{
		Port: getEnvInt("PORT", 8080),
	}, nil
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
