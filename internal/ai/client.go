package ai

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"jarvis/internal/config"
	"jarvis/internal/types"
)

type Client struct {
	cfg *config.Config

	httpClient *http.Client

	mu            sync.Mutex
	gigaToken     string
	gigaExpiresAt time.Time
}

func NewClient(cfg *config.Config) *Client {
	transport := &http.Transport{}

	if cfg.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: transport,
		},
	}
}

func (c *Client) Send(messages []types.Message) (string, error) {
	switch c.cfg.Provider {
	case "gigachat":
		return c.sendGigaChat(messages)

	case "nvidia", "openai":
		return c.sendOpenAICompatible(messages)

	default:
		return "", fmt.Errorf("unknown provider: %s", c.cfg.Provider)
	}
}

func (c *Client) sendOpenAICompatible(messages []types.Message) (string, error) {
	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"

	payload := map[string]interface{}{
		"model":       c.cfg.Model,
		"messages":    messages,
		"temperature": c.cfg.Temperature,
		"top_p":       c.cfg.TopP,
		"max_tokens":  c.cfg.MaxTokens,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	body, err := c.do(req)
	if err != nil {
		return "", err
	}

	return parseChatCompletionResponse(body)
}

func (c *Client) sendGigaChat(messages []types.Message) (string, error) {
	token, err := c.gigaAccessToken()
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(c.cfg.GigaBaseURL, "/") + "/chat/completions"

	payload := map[string]interface{}{
		"model":       c.cfg.Model,
		"messages":    messages,
		"temperature": c.cfg.Temperature,
		"top_p":       c.cfg.TopP,
		"max_tokens":  c.cfg.MaxTokens,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	body, err := c.do(req)
	if err != nil {
		return "", err
	}

	return parseChatCompletionResponse(body)
}

func (c *Client) gigaAccessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.gigaToken != "" && time.Now().Before(c.gigaExpiresAt.Add(-2*time.Minute)) {
		return c.gigaToken, nil
	}

	form := url.Values{}
	form.Set("scope", c.cfg.GigaScope)

	req, err := http.NewRequest(
		"POST",
		c.cfg.GigaOAuthURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Basic "+c.cfg.GigaAuthKey)
	req.Header.Set("RqUID", uuidV4())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	body, err := c.do(req)
	if err != nil {
		return "", err
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresAt   int64  `json:"expires_at"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("cannot parse GigaChat token response: %w. body=%s", err, string(body))
	}

	if parsed.AccessToken == "" {
		return "", fmt.Errorf("GigaChat token response has empty access_token: %s", string(body))
	}

	c.gigaToken = parsed.AccessToken

	if parsed.ExpiresAt > 0 {
		// GigaChat expires_at обычно приходит в milliseconds unix time.
		c.gigaExpiresAt = time.UnixMilli(parsed.ExpiresAt)
	} else {
		c.gigaExpiresAt = time.Now().Add(25 * time.Minute)
	}

	return c.gigaToken, nil
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func parseChatCompletionResponse(body []byte) (string, error) {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("cannot parse chat completion response: %w. body=%s", err, string(body))
	}

	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("no choices in model response: %s", string(body))
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("empty model response")
	}

	return content, nil
}

func uuidV4() string {
	var b [16]byte

	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}
