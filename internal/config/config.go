package config

import (
	"encoding/hex"
	"errors"
	"os"
	"strconv"
)

type Config struct {
	Addr            string
	PGURL           string
	MetricsPort     string
	MaxSpeedMS      float64
	MobileTokenHash []byte
}

func Load() (*Config, error) {
	var mobileTokenHash []byte
	if v := os.Getenv("MOBILE_TOKEN_SHA256"); v != "" {
		var err error
		mobileTokenHash, err = hex.DecodeString(v)
		if err != nil || len(mobileTokenHash) != 32 {
			return nil, errors.New("MOBILE_TOKEN_SHA256 must contain a SHA-256 hex digest")
		}
	}
	maxSpeed := 100.0
	if v := os.Getenv("MAX_SPEED_MS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			maxSpeed = f
		}
	}
	return &Config{
		Addr:            getenv("ADDR", ":8080"),
		PGURL:           getenv("POSTGRES_URL", "postgres://location:location@localhost:5432/location?sslmode=disable"),
		MetricsPort:     getenv("METRICS_PORT", "9104"),
		MaxSpeedMS:      maxSpeed,
		MobileTokenHash: mobileTokenHash,
	}, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
