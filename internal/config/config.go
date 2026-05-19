package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Provider string `json:"provider,omitempty"`

	// Common OpenAI-compatible API
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`

	// GigaChat
	GigaBaseURL        string `json:"giga_base_url,omitempty"`
	GigaOAuthURL       string `json:"giga_oauth_url,omitempty"`
	GigaAuthURL        string `json:"giga_auth_url,omitempty"`
	GigaAuthKey        string `json:"giga_auth_key,omitempty"`
	GigaScope          string `json:"giga_scope,omitempty"`
	GigaAccessToken    string `json:"giga_access_token,omitempty"`
	GigaRefreshToken   string `json:"giga_refresh_token,omitempty"`
	GigaTokenExpiresAt string `json:"giga_token_expires_at,omitempty"`

	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
	TopP        float64 `json:"top_p"`

	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty"`

	// Vision model, OpenAI-compatible endpoint
	VisionEnabled     bool    `json:"vision_enabled,omitempty"`
	VisionAPIKey      string  `json:"vision_api_key,omitempty"`
	VisionBaseURL     string  `json:"vision_base_url,omitempty"`
	VisionModel       string  `json:"vision_model,omitempty"`
	VisionMaxTokens   int     `json:"vision_max_tokens,omitempty"`
	VisionTemperature float64 `json:"vision_temperature,omitempty"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.Provider == "" {
		cfg.Provider = "gigachat"
	}

	if cfg.GigaBaseURL == "" {
		cfg.GigaBaseURL = "https://gigachat.devices.sberbank.ru/api/v1"
	}

	if cfg.GigaOAuthURL == "" {
		cfg.GigaOAuthURL = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
	}

	// Alias на случай, если где-то в коде используется старое имя.
	if cfg.GigaAuthURL == "" {
		cfg.GigaAuthURL = cfg.GigaOAuthURL
	}

	if cfg.GigaOAuthURL == "" {
		cfg.GigaOAuthURL = cfg.GigaAuthURL
	}

	if cfg.GigaScope == "" {
		cfg.GigaScope = "GIGACHAT_API_PERS"
	}

	if cfg.Model == "" {
		cfg.Model = "GigaChat"
	}

	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 700
	}

	if cfg.TopP == 0 {
		cfg.TopP = 0.7
	}

	if cfg.VisionMaxTokens <= 0 {
		cfg.VisionMaxTokens = 1200
	}

	if cfg.VisionTemperature == 0 {
		cfg.VisionTemperature = 0.1
	}

	return &cfg, nil
}
