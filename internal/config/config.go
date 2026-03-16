package config

import (
	"fmt"
	"os"
)

type Config struct {
	APIKey        string
	APIURL        string
	SandboxBankID string
	RedisHost     string
	BankLoginID   string
	BankPassword  string
}

func Load() (*Config, error) {
	apiKey := os.Getenv("BASIQ_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("BASIQ_API_KEY is required")
	}

	apiURL := os.Getenv("BASIQ_API_URL")
	if apiURL == "" {
		apiURL = "https://au-api.basiq.io"
	}

	sandboxBankID := os.Getenv("SANDBOX_BANK_ID")
	if sandboxBankID == "" {
		sandboxBankID = "AU00000"
	}

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost:6379"
	}

	bankLoginID := os.Getenv("BANK_LOGIN_ID")
	if bankLoginID == "" {
		return nil, fmt.Errorf("BANK_LOGIN_ID is required")
	}

	bankPassword := os.Getenv("BANK_PASSWORD")
	if bankPassword == "" {
		return nil, fmt.Errorf("BANK_PASSWORD is required")
	}

	return &Config{
		APIKey:        apiKey,
		APIURL:        apiURL,
		SandboxBankID: sandboxBankID,
		RedisHost:     redisHost,
		BankLoginID:   bankLoginID,
		BankPassword:  bankPassword,
	}, nil
}
