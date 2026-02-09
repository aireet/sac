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

	// Auth
	JWTSecret string
}

func Load() (*Config, error) {
	// Load .env file if exists
	_ = godotenv.Load()

	return &Config{
		// Server
		APIGatewayPort: getEnv("API_GATEWAY_PORT", "8080"),
		WSProxyPort:    getEnv("WS_PROXY_PORT", "8081"),

		// Database
		DBHost:     getEnv("DB_HOST", "pgm-uf68x0dfyoth4u5g.pg.rds.aliyuncs.com"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "sandbox"),
		DBPassword: getEnv("DB_PASSWORD", "4SOZfo6t6Oyj9A=="),
		DBName:     getEnv("DB_NAME", "sandbox"),

		// Kubernetes
		KubeconfigPath: getEnv("KUBECONFIG_PATH", "../kubeconfig.yaml"),
		Namespace:      getEnv("K8S_NAMESPACE", "sac"),

		// Docker Registry
		DockerRegistry: getEnv("DOCKER_REGISTRY", "docker-register-registry-vpc.cn-shanghai.cr.aliyuncs.com"),
		DockerImage:    getEnv("DOCKER_IMAGE", "prod/sac/cc:0.0.3"),

		// Auth
		JWTSecret: getEnv("JWT_SECRET", "sac-dev-jwt-secret-change-in-production"),
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
