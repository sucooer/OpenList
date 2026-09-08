package planned

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
)

func init() {
	RegisterAction("webhook", "Send an HTTP request to a URL (GET/POST/PUT/DELETE)", actionWebhook)
}

type webhookParams struct {
	URL     string            `json:"url"`     // target URL
	Method  string            `json:"method"`  // GET (default) | POST | PUT | DELETE
	Body    string            `json:"body"`    // request body (raw string)
	Headers map[string]string `json:"headers"` // extra request headers
	Timeout int               `json:"timeout"` // seconds, default 30
}

func actionWebhook(_ *model.PlannedTask, params map[string]interface{}, dryRun bool) (string, error) {
	var p webhookParams
	if err := decodeParams(params, &p); err != nil {
		return "", fmt.Errorf("invalid params: %w", err)
	}
	if p.URL == "" {
		return "", fmt.Errorf("url is required")
	}
	method := strings.ToUpper(strings.TrimSpace(p.Method))
	if method == "" {
		method = http.MethodGet
	}
	if dryRun {
		return fmt.Sprintf("dry-run: would send %s %s", method, p.URL), nil
	}

	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 30
	}
	var body io.Reader
	if p.Body != "" {
		body = bytes.NewReader([]byte(p.Body))
	}
	req, err := http.NewRequest(method, p.URL, body)
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	// drain a bounded prefix of the body for logging
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	bodyPreview := strings.TrimSpace(string(respBody))
	if len(bodyPreview) > 200 {
		bodyPreview = bodyPreview[:200] + "..."
	}
	status := fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	if bodyPreview != "" {
		return fmt.Sprintf("%s %s -> %s (%s)", method, p.URL, status, bodyPreview), nil
	}
	return fmt.Sprintf("%s %s -> %s", method, p.URL, status), nil
}
