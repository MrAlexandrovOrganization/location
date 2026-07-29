package config

import (
	"os"
	"strconv"
)

type Config struct {
	Addr        string
	PGURL       string
	MetricsPort string
	MaxSpeedMS  float64
}

func Load() (*Config, error) {
	maxSpeed := 100.0
	if v := os.Getenv("MAX_SPEED_MS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			maxSpeed = f
		}
	}
	return &Config{
		Addr:        getenv("ADDR", ":8080"),
		PGURL:       getenv("POSTGRES_URL", "postgres://location:location@localhost:5432/location?sslmode=disable"),
		MetricsPort: getenv("METRICS_PORT", "9104"),
		MaxSpeedMS:  maxSpeed,
	}, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
