package config

import "os"

type Config struct {
	Port          string
	APIToken      string
	ESLHost       string
	ESLPort       string
	ESLPassword   string
	DBDSN         string
	WebhookURL    string
	WebhookSecret string
}

func Load() Config {
	return Config{
		Port:          getenv("CC_API_PORT", "8090"),
		APIToken:      getenv("CC_API_TOKEN", "dev-cc-token-change-me"),
		ESLHost:       getenv("CC_ESL_HOST", "127.0.0.1"),
		ESLPort:       getenv("CC_ESL_PORT", "8021"),
		ESLPassword:   getenv("CC_ESL_PASSWORD", "ClueCon"),
		DBDSN:         getenv("CC_DB_DSN", "postgres://fscc:fscc123@127.0.0.1:5434/fs_cc?sslmode=disable"),
		WebhookURL:    getenv("CC_WEBHOOK_URL", ""),
		WebhookSecret: getenv("CC_WEBHOOK_SECRET", "dev-webhook-secret-change-me"),
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
