package api

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// tmdbBase 校验并返回 TMDB API base URL（含 trim 右斜杠）。与 Emby /
// Bangumi / Telegram 共享 SSRF 否决：拒绝 link-local / 云元数据 IP / 非
// http(s) scheme / query+fragment。配置面被入侵或 admin 误填时，可信的
// TMDB API Key 不会被发到攻击者控制的内部目标。
func (a *App) tmdbBase() (string, error) {
	raw := strings.TrimSpace(a.cfg().TMDBAPIURL)
	if raw == "" {
		return "", fmt.Errorf("TMDB API URL 未配置")
	}
	probeBase := raw
	if pb, err := url.Parse(raw); err == nil {
		clean := *pb
		clean.RawQuery = ""
		clean.Fragment = ""
		probeBase = clean.String()
	}
	cleaned, err := validateOutboundBaseURL(probeBase, "TMDB")
	if err != nil {
		return "", err
	}
	return cleaned, nil
}

func (a *App) searchTMDB(ctx context.Context, query, mediaType string, limit int) ([]map[string]any, error) {
	if a.cfg().TMDBAPIKey == "" {
		return nil, fmt.Errorf("TMDB API Key 未配置")
	}
	base, err := a.tmdbBase()
	if err != nil {
		return nil, err
	}
	mediaType = normalizeTMDBMediaType(mediaType)
	endpoint := base + "/search/multi"
	if mediaType == "movie" || mediaType == "tv" {
		endpoint = base + "/search/" + mediaType
	}
	q := url.Values{"api_key": {a.cfg().TMDBAPIKey}, "language": {"zh-CN"}, "query": {query}}
	var payload map[string]any
	if err := getJSON(ctx, endpoint+"?"+q.Encode(), nil, &payload); err != nil {
		return nil, err
	}
	rows, _ := payload["results"].([]any)
	results := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item, _ := row.(map[string]any)
		if item == nil {
			continue
		}
		mt := firstNonEmpty(fmt.Sprint(item["media_type"]), mediaType, "movie")
		if mt == "person" {
			continue
		}
		results = append(results, tmdbToMedia(item, mt, a.cfg().TMDBImageURL))
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (a *App) getTMDB(ctx context.Context, id, mediaType string) (map[string]any, error) {
	if !isPositiveNumericID(id) {
		return nil, fmt.Errorf("invalid TMDB media id")
	}
	mediaType = normalizeTMDBMediaType(mediaType)
	if a.cfg().TMDBAPIKey == "" {
		return nil, fmt.Errorf("TMDB API Key 未配置")
	}
	base, err := a.tmdbBase()
	if err != nil {
		return nil, err
	}
	endpoint := base + "/" + mediaType + "/" + id
	q := url.Values{"api_key": {a.cfg().TMDBAPIKey}, "language": {"zh-CN"}, "append_to_response": {"credits,genres,videos,images,seasons"}}
	var payload map[string]any
	if err := getJSON(ctx, endpoint+"?"+q.Encode(), nil, &payload); err != nil {
		return nil, err
	}
	return tmdbToMedia(payload, mediaType, a.cfg().TMDBImageURL), nil
}

func tmdbToMedia(item map[string]any, mediaType, imageBase string) map[string]any {
	id := fmt.Sprint(item["id"])
	title := firstNonEmpty(asString(item["title"]), asString(item["name"]), id)
	original := firstNonEmpty(asString(item["original_title"]), asString(item["original_name"]), title)
	release := firstNonEmpty(asString(item["release_date"]), asString(item["first_air_date"]))
	poster := ""
	if path := asString(item["poster_path"]); path != "" {
		poster = strings.TrimRight(imageBase, "/") + "/w500" + path
	}
	result := mediaResultFromFields("tmdb", id, title, mediaType, poster)
	if path := asString(item["backdrop_path"]); path != "" {
		result["backdrop"] = strings.TrimRight(imageBase, "/") + "/w780" + path
		result["backdrop_url"] = result["backdrop"]
	}
	result["original_title"] = original
	result["overview"] = asString(item["overview"])
	result["release_date"] = release
	if len(release) >= 4 {
		result["year"] = release[:4]
	}
	rating := numeric(item["vote_average"])
	result["vote_average"] = rating
	result["rating"] = rating
	genres := []string{}
	if rows, ok := item["genres"].([]any); ok {
		for _, row := range rows {
			genre, _ := row.(map[string]any)
			if name := asString(genre["name"]); name != "" {
				genres = append(genres, name)
			}
		}
	}
	if len(genres) > 0 {
		result["genres"] = genres
	}
	if runtime := int(numeric(item["runtime"])); runtime > 0 {
		result["runtime"] = runtime
	}
	if seasons := int(numeric(item["number_of_seasons"])); seasons > 0 {
		result["seasons"] = seasons
	}
	if episodes := int(numeric(item["number_of_episodes"])); episodes > 0 {
		result["episodes"] = episodes
	}
	if status := asString(item["status"]); status != "" {
		result["status"] = status
	}
	result["extra"] = map[string]any{"vote_count": item["vote_count"], "original_language": item["original_language"], "popularity": item["popularity"], "genres": genres, "runtime": result["runtime"], "number_of_seasons": result["seasons"], "number_of_episodes": result["episodes"]}
	return result
}
