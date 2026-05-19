package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func (c *Client) AnalyzeImage(ctx context.Context, prompt string, imagePath string) (string, error) {
	if c == nil || c.cfg == nil {
		return "", fmt.Errorf("vision client is nil")
	}

	if !c.cfg.VisionEnabled {
		return "", fmt.Errorf("vision is disabled in config")
	}

	baseURL := strings.TrimRight(c.cfg.VisionBaseURL, "/")
	apiKey := strings.TrimSpace(c.cfg.VisionAPIKey)
	model := strings.TrimSpace(c.cfg.VisionModel)

	if baseURL == "" {
		baseURL = strings.TrimRight(c.cfg.BaseURL, "/")
	}

	if apiKey == "" {
		apiKey = strings.TrimSpace(c.cfg.APIKey)
	}

	if model == "" {
		model = strings.TrimSpace(c.cfg.Model)
	}

	if baseURL == "" {
		return "", fmt.Errorf("vision_base_url is empty")
	}

	if apiKey == "" {
		return "", fmt.Errorf("vision_api_key is empty")
	}

	if model == "" {
		return "", fmt.Errorf("vision_model is empty")
	}

	imageBytes, err := os.ReadFile(imagePath)
	if err != nil {
		return "", err
	}

	encoded := base64.StdEncoding.EncodeToString(imageBytes)
	dataURL := "data:image/png;base64," + encoded

	payload := map[string]interface{}{
		"model":       model,
		"temperature": c.cfg.VisionTemperature,
		"max_tokens":  c.cfg.VisionMaxTokens,
		"messages": []map[string]interface{}{
			{
				"role": "system",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": "You are a computer vision sensor inside a Windows desktop AI runtime. Return only valid JSON. Do not invent invisible details.",
					},
				},
			},
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": prompt,
					},
					{
						"type": "image_url",
						"image_url": map[string]interface{}{
							"url": dataURL,
						},
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 80 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("vision request failed: status=%d body=%s", resp.StatusCode, string(rawBody))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content interface{} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return "", fmt.Errorf("cannot parse vision response: %w body=%s", err, string(rawBody))
	}

	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("vision response has no choices")
	}

	return contentToString(parsed.Choices[0].Message.Content), nil
}

func contentToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)

	case []interface{}:
		var b strings.Builder

		for _, item := range v {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			if text, ok := m["text"].(string); ok {
				b.WriteString(text)
			}
		}

		return strings.TrimSpace(b.String())

	default:
		raw, _ := json.Marshal(value)
		return strings.TrimSpace(string(raw))
	}
}
