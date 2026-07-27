package config

import "os"

type Config struct {
	Addr        string
	PGURL       string
	MetricsPort string
}

func Load() (*Config, error) {
	return &Config{
		Addr:        getenv("ADDR", ":8080"),
		PGURL:       getenv("POSTGRES_URL", "postgres://location:location@localhost:5432/location?sslmode=disable"),
		MetricsPort: getenv("METRICS_PORT", "9104"),
	}, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
