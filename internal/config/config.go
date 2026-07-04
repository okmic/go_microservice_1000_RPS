package config

import (
    "fmt"
    "log"
    "os"
    "strconv"
    "time"

    "github.com/joho/godotenv"
)

type Config struct {
    AppPort string
    AppEnv  string

    DBHost            string
    DBPort            string
    DBUser            string
    DBPassword        string
    DBName            string
    DBSSLMode         string
    DBMaxOpenConns    int
    DBMaxIdleConns    int
    DBConnMaxLifetime time.Duration

    RateLimitPerSecond int
    RateLimitBurst     int
}

func Load() *Config {
    if err := godotenv.Load("config.env"); err != nil {
        log.Printf("Warning: config.env not found, using environment variables")
    }

    return &Config{
        AppPort: getEnv("APP_PORT", "9999"),
        AppEnv:  getEnv("APP_ENV", "development"),

        DBHost:            getEnv("DB_HOST", "localhost"),
        DBPort:            getEnv("DB_PORT", "5432"),
        DBUser:            getEnv("DB_USER", "postgres"),
        DBPassword:        getEnv("DB_PASSWORD", "postgres"),
        DBName:            getEnv("DB_NAME", "walletdb"),
        DBSSLMode:         getEnv("DB_SSL_MODE", "disable"),
        DBMaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 100),
        DBMaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 10),
        DBConnMaxLifetime: time.Duration(getEnvAsInt("DB_CONN_MAX_LIFETIME", 300)) * time.Second,

        RateLimitPerSecond: getEnvAsInt("RATE_LIMIT_PER_SECOND", 1000),
        RateLimitBurst:     getEnvAsInt("RATE_LIMIT_BURST", 2000),
    }
}

func (c *Config) GetDBDSN() string {
    return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
        c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode)
}

func getEnv(key, defaultValue string) string {
    if value, exists := os.LookupEnv(key); exists {
        return value
    }
    return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
    if value, exists := os.LookupEnv(key); exists {
        if intVal, err := strconv.Atoi(value); err == nil {
            return intVal
        }
    }
    return defaultValue
}
