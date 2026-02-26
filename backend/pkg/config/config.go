package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	APIGatewayPort string
	WSProxyPort    string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	// Kubernetes
	KubeconfigPath string
	Namespace      string

	// Docker Registry
	DockerRegistry string
	DockerImage    string
	SidecarImage   string

	// Auth
	JWTSecret string

	// Redis
	RedisURL string

	// Logging
	LogLevel  string
	LogFormat string
}

func Load() (*Config, error) {
	// Load .env file if exists
	_ = godotenv.Load()

	return &Config{
		// Server
		APIGatewayPort: getEnv("API_GATEWAY_PORT", "8080"),
		WSProxyPort:    getEnv("WS_PROXY_PORT", "8081"),

		// Database
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "sac"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "sac"),

		// Kubernetes
		KubeconfigPath: getEnv("KUBECONFIG_PATH", "../kubeconfig.yaml"),
		Namespace:      getEnv("K8S_NAMESPACE", "sac"),

		// Docker Registry
		DockerRegistry: getEnv("DOCKER_REGISTRY", ""),
		DockerImage:    getEnv("DOCKER_IMAGE", ""),
		SidecarImage:   getEnv("SIDECAR_IMAGE", ""),

		// Auth
		JWTSecret: getEnv("JWT_SECRET", ""),

		// Redis
		RedisURL: getEnv("REDIS_URL", ""),

		// Logging
		LogLevel:  getEnv("LOG_LEVEL", "info"),
		LogFormat: getEnv("LOG_FORMAT", "json"),
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}
