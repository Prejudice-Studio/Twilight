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
	// 先按「调用方是否明确指定了媒体类型」选择端点：指定 movie/tv 走
	// /search/movie 或 /search/tv，未指定走 /search/multi（名称搜索不限定类型）。
	// 注意不能先 normalizeTMDBMediaType：空串会被归一成 "movie"，导致名称搜索
	// 永远落到 /search/movie，剧集永远搜不到。
	endpoint := base + "/search/multi"
	if specific := tmdbSearchMediaType(mediaType); specific != "" {
		endpoint = base + "/search/" + specific
	}
	mediaType = normalizeTMDBMediaType(mediaType)
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
		// /search/multi 会混入 person 结果；/search/movie 与 /search/tv 结果里
		// 通常没有 media_type 字段。先按原始 media_type 过滤 person，再落到
		// 搜索时指定的媒体类型（多源搜索时 mediaType 为空，默认 movie）。
		rawMT := strings.ToLower(strings.TrimSpace(asString(item["media_type"])))
		if rawMT == "person" {
			continue
		}
		mt := normalizeTMDBMediaType(firstNonEmpty(rawMT, mediaType, "movie"))
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
	q := url.Values{
		"api_key":                {a.cfg().TMDBAPIKey},
		"language":               {"zh-CN"},
		"append_to_response":     {"credits,videos,images"},
		"include_image_language": {"zh,ja,en"},
	}
	var payload map[string]any
	if err := getJSON(ctx, endpoint+"?"+q.Encode(), nil, &payload); err != nil {
		return nil, err
	}
	return tmdbToMedia(payload, mediaType, a.cfg().TMDBImageURL), nil
}

func tmdbToMedia(item map[string]any, mediaType, imageBase string) map[string]any {
	id := asString(item["id"])
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
	if logoURL, logoLanguage := tmdbLogoURL(item["images"], imageBase); logoURL != "" {
		result["logo"] = logoURL
		result["logo_url"] = logoURL
		result["logo_language"] = logoLanguage
	}
	result["original_title"] = original
	result["overview"] = asString(item["overview"])
	result["release_date"] = release
	if len(release) >= 4 {
		result["year"] = release[:4]
	}
	rating := mediaFloat(item["vote_average"])
	result["vote_average"] = rating
	result["rating"] = rating
	if voteCount := int(numeric(item["vote_count"])); voteCount > 0 {
		result["vote_count"] = voteCount
	}
	if tagline := strings.TrimSpace(asString(item["tagline"])); tagline != "" {
		result["tagline"] = truncateString(tagline, 300)
	}
	if endDate := strings.TrimSpace(asString(item["last_air_date"])); endDate != "" {
		result["end_date"] = endDate
	}
	if homepage := strings.TrimSpace(asString(item["homepage"])); homepage != "" {
		result["official_url"] = homepage
	}
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
	runtime := int(numeric(item["runtime"]))
	if runtime <= 0 {
		if runtimes, ok := item["episode_run_time"].([]any); ok && len(runtimes) > 0 {
			runtime = int(numeric(runtimes[0]))
		}
	}
	if runtime > 0 {
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
	countries := mediaStringList(item["production_countries"], []string{"name", "iso_3166_1"}, 5)
	if len(countries) == 0 {
		countries = mediaStringList(item["origin_country"], nil, 5)
	}
	if len(countries) > 0 {
		result["countries"] = countries
	}
	languages := mediaStringList(item["spoken_languages"], []string{"name", "english_name", "iso_639_1"}, 5)
	if len(languages) == 0 {
		languages = mediaStringList(item["original_language"], nil, 1)
	}
	if len(languages) > 0 {
		result["languages"] = languages
	}
	studios := mergeMediaStringLists(8,
		mediaStringList(item["production_companies"], []string{"name"}, 8),
		mediaStringList(item["networks"], []string{"name"}, 8),
	)
	if len(studios) > 0 {
		result["studios"] = studios
	}
	credits, _ := item["credits"].(map[string]any)
	creators := mergeMediaStringLists(8,
		mediaStringList(item["created_by"], []string{"name"}, 8),
		tmdbCrewNames(credits["crew"], 8),
	)
	if len(creators) > 0 {
		result["creators"] = creators
	}
	if cast := mediaStringList(credits["cast"], []string{"name"}, 8); len(cast) > 0 {
		result["cast"] = cast
	}
	if trailerURL := tmdbTrailerURL(item["videos"]); trailerURL != "" {
		result["trailer_url"] = trailerURL
	}
	result["extra"] = map[string]any{"vote_count": result["vote_count"], "original_language": item["original_language"], "popularity": item["popularity"], "genres": genres, "runtime": result["runtime"], "number_of_seasons": result["seasons"], "number_of_episodes": result["episodes"]}
	return result
}

func tmdbCrewNames(value any, limit int) []string {
	rows, _ := value.([]any)
	filtered := make([]any, 0, min(limit, len(rows)))
	for _, row := range rows {
		crew, _ := row.(map[string]any)
		if crew == nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(asString(crew["job"]))) {
		case "director", "creator", "screenplay", "writer", "series director":
			filtered = append(filtered, crew)
		}
		if len(filtered) >= limit {
			break
		}
	}
	return mediaStringList(filtered, []string{"name"}, limit)
}

func tmdbTrailerURL(value any) string {
	videos, _ := value.(map[string]any)
	rows, _ := videos["results"].([]any)
	fallback := ""
	for _, row := range rows {
		video, _ := row.(map[string]any)
		if video == nil || !strings.EqualFold(strings.TrimSpace(asString(video["site"])), "YouTube") {
			continue
		}
		key := strings.TrimSpace(asString(video["key"]))
		if !safeYouTubeVideoKey(key) {
			continue
		}
		candidate := "https://www.youtube.com/watch?v=" + key
		if fallback == "" {
			fallback = candidate
		}
		if strings.EqualFold(strings.TrimSpace(asString(video["type"])), "Trailer") {
			return candidate
		}
	}
	return fallback
}

func tmdbLogoURL(value any, imageBase string) (string, string) {
	images, _ := value.(map[string]any)
	logos, _ := images["logos"].([]any)
	for _, language := range []string{"zh", "ja", "en"} {
		for _, row := range logos {
			logo, _ := row.(map[string]any)
			if logo == nil || strings.ToLower(strings.TrimSpace(asString(logo["iso_639_1"]))) != language {
				continue
			}
			path := strings.TrimSpace(asString(logo["file_path"]))
			if path == "" {
				continue
			}
			return strings.TrimRight(imageBase, "/") + "/w500" + path, language
		}
	}
	return "", ""
}

func safeYouTubeVideoKey(value string) bool {
	if len(value) < 6 || len(value) > 32 {
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
