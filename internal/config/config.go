package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server     ServerConfig
	ClickHouse ClickhouseConfig
	Postgres   PostgresConfig
	Binance    BinanceConfig
}

type ServerConfig struct {
	Port         string
	Environment  string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type PostgresConfig struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	SSLMode  string
}

type ClickhouseConfig struct {
	Port     int
	Host     string
	Database string
	Username string
	Password string
	Debug    bool
}

type BinanceConfig struct {
	WSBaseURL string
	Symbols   []string
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:         getEnv("SERVER_PORT", ":8080"),
			Environment:  getEnv("ENVIRONMENT", "development"),
			ReadTimeout:  getDurationEnv("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getDurationEnv("SERVER_WRITE_TIMEOUT", 30*time.Second),
		},
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getIntEnv("POSTGRES_PORT", 5432),
			Database: getEnv("POSTGRES_DATABASE", "crypto"),
			Username: getEnv("POSTGRES_USERNAME", "crypto"),
			Password: getEnv("POSTGRES_PASSWORD", "crypto"),
			SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
		},
		ClickHouse: ClickhouseConfig{
			Host:     getEnv("CLICKHOUSE_HOST", "localhost"),
			Port:     getIntEnv("CLICKHOUSE_PORT", 9000),
			Database: getEnv("CLICKHOUSE_DATABASE", "crypto"),
			Username: getEnv("CLICKHOUSE_USERNAME", "default"),
			Password: getEnv("CLICKHOUSE_PASSWORD", ""),
			Debug:    getBoolEnv("CLICKHOUSE_DEBUG", false),
		},
		Binance: BinanceConfig{
			WSBaseURL: getEnv("BINANCE_WS_URL", "wss://stream.binance.com:9443"),
			Symbols:   []string{"btcusdt", "ethusdt", "adausdt", "bnbusdt", "xrpusdt", "solusdt", "dotusdt", "linkusdt", "ltcusdt", "bchusdt"},
		},
	}

	return cfg, nil
}

func (c *ClickhouseConfig) ConnectionString() string {
	return fmt.Sprintf("tcp://%s:%d?database=%s&username=%s&password=%s&debug=%t",
		c.Host, c.Port, c.Database, c.Username, c.Password, c.Debug)
}

func (p *PostgresConfig) ConnectionString() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.Host, p.Port, p.Username, p.Password, p.Database, p.SSLMode)
}

// Helper function to get environment variables
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
