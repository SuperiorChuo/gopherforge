package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	URL    string
	Secret string
	HTTP   *http.Client
}

func New(url, secret string) *Client {
	return &Client{
		URL:    url,
		Secret: secret,
		HTTP:   &http.Client{Timeout: 8 * time.Second},
	}
}

func (c *Client) Enabled() bool { return c != nil && c.URL != "" }

func (c *Client) Send(event string, payload any) error {
	if !c.Enabled() {
		return fmt.Errorf("webhook url empty")
	}
	body := map[string]any{
		"event":     event,
		"payload":   payload,
		"sent_at":   time.Now().UTC().Format(time.RFC3339),
		"source":    "go-freeswitch-cc",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.URL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CC-Event", event)
	if c.Secret != "" {
		mac := hmac.New(sha256.New, []byte(c.Secret))
		_, _ = mac.Write(raw)
		req.Header.Set("X-CC-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}
