package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

const maxEmbyItemImageBytes = int64(10 << 20)

type embyItemMetadata struct {
	ID         string `json:"Id"`
	Name       string `json:"Name"`
	Type       string `json:"Type"`
	SeriesID   string `json:"SeriesId"`
	SeriesName string `json:"SeriesName"`
}

func embyItemImageURL(itemID string) string {
	itemID = strings.TrimSpace(itemID)
	if !validEmbyItemID(itemID) {
		return ""
	}
	return "/api/v1/emby/items/" + url.PathEscape(itemID) + "/image"
}

func validEmbyItemID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func (a *App) handleEmbyItemImage(w http.ResponseWriter, r *http.Request, params Params) {
	if !a.embyConfigured() {
		failWithCode(w, http.StatusBadGateway, ErrEmbyNotConfigured, "Emby not configured")
		return
	}
	itemID := strings.TrimSpace(params["item_id"])
	if !validEmbyItemID(itemID) {
		failWithCode(w, http.StatusNotFound, ErrNotFound, "image not found")
		return
	}
	endpoint, err := a.validatedEmbyEndpoint("/Items/" + url.PathEscape(itemID) + "/Images/Primary?maxWidth=512&quality=90")
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "failed to resolve Emby image endpoint")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "failed to create Emby image request")
		return
	}
	for key, value := range a.embyHeaders() {
		req.Header.Set(key, value)
	}
	req.Header.Set("Accept", "image/avif,image/webp,image/*")
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "failed to fetch Emby image")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		failWithCode(w, http.StatusNotFound, ErrNotFound, "image not found")
		return
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "Emby image request failed")
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxEmbyItemImageBytes+1))
	if err != nil || len(data) == 0 || int64(len(data)) > maxEmbyItemImageBytes {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "invalid Emby image response")
		return
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	detectedType := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]))
	if isEmbyItemImageContentType(detectedType) {
		contentType = detectedType
	} else if !isEmbyItemImageContentType(contentType) {
		failWithCode(w, http.StatusBadGateway, ErrInternal, "invalid Emby image content type")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600, stale-while-revalidate=86400")
	if etag := strings.TrimSpace(resp.Header.Get("ETag")); etag != "" && !strings.ContainsAny(etag, "\r\n") {
		w.Header().Set("ETag", etag)
	}
	_, _ = w.Write(data)
}

func isEmbyItemImageContentType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image/jpeg", "image/png", "image/webp", "image/gif", "image/avif", "image/bmp":
		return true
	default:
		return false
	}
}

func (a *App) embyItemMetadata(ctx context.Context, ids []string) map[string]embyItemMetadata {
	cleanIDs := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		cleanIDs = append(cleanIDs, id)
	}
	result := make(map[string]embyItemMetadata, len(cleanIDs))
	const batchSize = 100
	for start := 0; start < len(cleanIDs); start += batchSize {
		end := min(start+batchSize, len(cleanIDs))
		var payload struct {
			Items []embyItemMetadata `json:"Items"`
		}
		query := embyItemQuery(map[string]string{
			"Ids":       strings.Join(cleanIDs[start:end], ","),
			"Recursive": "true",
			"Fields":    "SeriesId,SeriesName",
		})
		if err := a.embyGet(ctx, "/Items"+query, &payload); err != nil {
			zap.L().Warn("failed to batch read Emby item metadata", zap.Error(err))
			continue
		}
		for _, item := range payload.Items {
			result[item.ID] = item
		}
	}
	return result
}
