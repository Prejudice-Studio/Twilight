package api

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

func (a *App) searchBangumi(ctx context.Context, query string, limit int) ([]map[string]any, error) {
	endpoint, err := bangumiEndpoint(a.cfg.BangumiAPIURL, "/search/subjects", url.Values{
		"limit":  {fmt.Sprint(limit)},
		"offset": {"0"},
	})
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"keyword": query,
		"sort":    "match",
		"filter":  map[string]any{"type": []int{2, 6}, "nsfw": true},
	}
	var payload map[string]any
	if err := postJSON(ctx, endpoint, a.bangumiHeaders(), body, &payload); err != nil {
		return nil, err
	}
	rows, _ := payload["data"].([]any)
	results := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item, _ := row.(map[string]any)
		if item != nil {
			results = append(results, bangumiToMedia(item))
		}
	}
	return results, nil
}

func (a *App) getBangumi(ctx context.Context, id string) (map[string]any, error) {
	endpoint, err := bangumiEndpoint(a.cfg.BangumiAPIURL, "/subjects/"+id, nil)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := getJSON(ctx, endpoint, a.bangumiHeaders(), &payload); err != nil {
		return nil, err
	}
	return bangumiToMedia(payload), nil
}

func (a *App) bangumiHeaders() map[string]string {
	headers := map[string]string{"User-Agent": "Twilight/1.0", "Accept": "application/json"}
	if a.cfg.BangumiToken != "" {
		headers["Authorization"] = "Bearer " + a.cfg.BangumiToken
	}
	return headers
}

func bangumiEndpoint(base, path string, values url.Values) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "https://api.bgm.tv/v0"
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(parsed.Path, "/v0") {
		parsed.Path += "/v0"
	}
	parsed.Path += "/" + strings.TrimLeft(path, "/")
	if values != nil {
		parsed.RawQuery = values.Encode()
	}
	return parsed.String(), nil
}

func bangumiToMedia(item map[string]any) map[string]any {
	id := fmt.Sprint(item["id"])
	title := firstNonEmpty(asString(item["name_cn"]), asString(item["name"]), id)
	images, _ := item["images"].(map[string]any)
	poster := firstNonEmpty(asString(images["large"]), asString(images["common"]), asString(images["medium"]))
	result := mediaResultFromFields("bangumi", id, title, bangumiTypeName(int(numeric(item["type"]))), poster)
	result["original_title"] = firstNonEmpty(asString(item["name"]), title)
	result["overview"] = asString(item["summary"])
	result["release_date"] = asString(item["date"])
	if date := asString(item["date"]); len(date) >= 4 {
		result["year"] = date[:4]
	}
	rating, _ := item["rating"].(map[string]any)
	score := numeric(rating["score"])
	result["vote_average"] = score
	result["rating"] = score
	genres := []string{}
	if tags, ok := item["tags"].([]any); ok {
		for _, row := range tags {
			tag, _ := row.(map[string]any)
			if name := asString(tag["name"]); name != "" {
				genres = append(genres, name)
			}
			if len(genres) >= 5 {
				break
			}
		}
	}
	if len(genres) > 0 {
		result["genres"] = genres
	}
	result["extra"] = map[string]any{"rank": rating["rank"], "type_id": item["type"], "eps": item["eps"], "volumes": item["volumes"], "tags": item["tags"]}
	return result
}

func bangumiTypeName(t int) string {
	switch t {
	case 1:
		return "书籍"
	case 2:
		return "动画"
	case 3:
		return "音乐"
	case 4:
		return "游戏"
	case 6:
		return "三次元"
	default:
		return "未知"
	}
}
