package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func getJSON(ctx context.Context, endpoint string, headers map[string]string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return doJSONRequest(req, dst)
}

func postJSON(ctx context.Context, endpoint string, headers map[string]string, body any, dst any) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return doJSONRequest(req, dst)
}

func doJSONRequest(req *http.Request, dst any) error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		detail := strings.TrimSpace(string(data))
		if detail != "" {
			return fmt.Errorf("remote status %d: %s", resp.StatusCode, truncateString(detail, 300))
		}
		return fmt.Errorf("remote status %d", resp.StatusCode)
	}
	if dst == nil {
		return nil
	}
	return json.Unmarshal(data, dst)
}
