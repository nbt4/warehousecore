package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"

	commonconfig "github.com/nbt4/cores-common/pkg/config"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	App      AppConfig
	CORS     CORSConfig
}

type ServerConfig struct {
	Port string
	Host string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

type AppConfig struct {
	Environment string
	LogLevel    string
}

type CORSConfig struct {
	AllowedOrigins []string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Server: ServerConfig{
			Port: commonconfig.GetEnv("PORT", "8081"),
			Host: commonconfig.GetEnv("HOST", "0.0.0.0"),
		},
		Database: DatabaseConfig{
			Host:     commonconfig.GetEnv("DB_HOST", "localhost"),
			Port:     commonconfig.GetEnv("DB_PORT", "5432"),
			Name:     commonconfig.GetEnv("DB_NAME", "rentalcore"),
			User:     commonconfig.GetEnv("DB_USER", "rentalcore"),
			Password: commonconfig.GetEnv("DB_PASS", ""),
			SSLMode:  commonconfig.GetEnv("DB_SSLMODE", "disable"),
		},
		App: AppConfig{
			Environment: commonconfig.GetEnv("APP_ENV", "development"),
			LogLevel:    commonconfig.GetEnv("LOG_LEVEL", "info"),
		},
		CORS: CORSConfig{
			AllowedOrigins: []string{commonconfig.GetEnv("CORS_ORIGIN", "*")},
		},
	}

	log.Printf("Database Config: PostgreSQL %s@%s:%s/%s",
		cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)

	return cfg, nil
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode)
}
