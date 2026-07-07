package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func (a *App) searchBangumi(ctx context.Context, query string, limit int) ([]map[string]any, error) {
	endpoint, err := bangumiEndpoint(a.cfg().BangumiAPIURL, "/search/subjects", url.Values{
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
	if !isPositiveNumericID(id) {
		return nil, fmt.Errorf("invalid Bangumi subject id")
	}
	endpoint, err := bangumiEndpoint(a.cfg().BangumiAPIURL, "/subjects/"+id, nil)
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
	if a.cfg().BangumiToken != "" {
		headers["Authorization"] = "Bearer " + a.cfg().BangumiToken
	}
	return headers
}

func bangumiEndpoint(base, path string, values url.Values) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "https://api.bgm.tv/v0"
	}
	// 与 Emby/Telegram/TMDB 共享 SSRF 否决：拒绝 link-local / 云元数据 IP /
	// 非 http(s) scheme / query+fragment。这里 base 可能本身已经带了 /v0
	// 路径后缀（兼容老配置），所以校验前用一份去掉 path 的"纯 base"喂给
	// validateOutboundBaseURL（它的语义是"裸 base URL 不应带 query/fragment"）。
	cleanedBase := base
	if cb, err := url.Parse(base); err == nil {
		probe := *cb
		probe.Path = ""
		probe.RawPath = ""
		cleanedBase = probe.String()
	}
	if _, err := validateOutboundBaseURL(cleanedBase, "Bangumi"); err != nil {
		return "", err
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

func (a *App) getBangumiMe(ctx context.Context, token string) (map[string]any, bool, error) {
	endpoint, err := bangumiEndpoint(a.cfg().BangumiAPIURL, "/me", nil)
	if err != nil {
		return nil, false, err
	}
	headers := map[string]string{
		"User-Agent":    "Twilight/1.0",
		"Accept":        "application/json",
		"Authorization": "Bearer " + token,
	}
	var payload map[string]any
	err = getJSON(ctx, endpoint, headers, &payload)
	if err != nil {
		if strings.Contains(err.Error(), "remote status 401") {
			return nil, true, nil
		}
		return nil, false, err
	}
	return payload, false, nil
}

func (a *App) getBangumiUserCollections(ctx context.Context, username string, token string, collectType int, limit int, offset int) ([]map[string]any, int, error) {
	if limit <= 0 {
		limit = 8
	}
	values := url.Values{
		"subject_type": {"2"},                       // 2 for anime
		"type":         {strconv.Itoa(collectType)}, // 1:想看, 2:看过, 3:在看
		"limit":        {strconv.Itoa(limit)},
		"offset":       {strconv.Itoa(offset)},
	}
	endpoint, err := bangumiEndpoint(a.cfg().BangumiAPIURL, "/users/"+username+"/collections", values)
	if err != nil {
		return nil, 0, err
	}
	headers := map[string]string{
		"User-Agent":    "Twilight/1.0",
		"Accept":        "application/json",
		"Authorization": "Bearer " + token,
	}
	var payload map[string]any
	if err := getJSON(ctx, endpoint, headers, &payload); err != nil {
		return nil, 0, err
	}
	rows, _ := payload["data"].([]any)
	results := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item, _ := row.(map[string]any)
		if item != nil {
			results = append(results, item)
		}
	}
	total := int(numeric(payload["total"]))
	return results, total, nil
}

func (a *App) bangumiUserHeaders(token string) map[string]string {
	return map[string]string{
		"User-Agent":    "Twilight/1.0",
		"Accept":        "application/json",
		"Authorization": "Bearer " + token,
	}
}

func (a *App) updateBangumiCollection(ctx context.Context, subjectID string, token string, collectType int, rate int, hasRate bool) error {
	endpoint, err := bangumiEndpoint(a.cfg().BangumiAPIURL, "/users/-/collections/"+subjectID, nil)
	if err != nil {
		return err
	}
	body := map[string]any{
		"type": collectType,
	}
	if hasRate {
		body["rate"] = rate
	}
	headers := a.bangumiUserHeaders(token)
	var result map[string]any
	// 先尝试 PATCH（修改已有收藏），若返回 404 则回退 POST（新建收藏）
	if err := patchJSON(ctx, endpoint, headers, body, &result); err != nil {
		if strings.Contains(err.Error(), "remote status 404") {
			if err2 := postJSON(ctx, endpoint, headers, body, &result); err2 != nil {
				return err2
			}
			return nil
		}
		return err
	}
	return nil
}

func (a *App) updateBangumiEpisodeProgress(ctx context.Context, subjectID string, token string, epStatus int) error {
	episodes, err := a.bangumiEpisodes(ctx, subjectID, token)
	if err != nil {
		return err
	}
	watchedIDs := bangumiEpisodeIDsThrough(episodes, epStatus)
	if epStatus > 0 && len(watchedIDs) == 0 {
		return fmt.Errorf("未找到 Bangumi 第 %d 话以内的本篇章节", epStatus)
	}
	if len(watchedIDs) > 0 {
		if err := a.patchBangumiEpisodes(ctx, subjectID, token, watchedIDs, 2); err != nil {
			return err
		}
	}
	currentDone, err := a.bangumiDoneEpisodeIDSet(ctx, subjectID, token)
	if err != nil {
		return err
	}
	clearIDs := make([]int, 0)
	for _, episode := range episodes {
		if episode.Number <= epStatus || !currentDone[episode.ID] {
			continue
		}
		clearIDs = append(clearIDs, episode.ID)
	}
	if len(clearIDs) > 0 {
		if err := a.patchBangumiEpisodes(ctx, subjectID, token, clearIDs, 0); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) markBangumiEpisodesThrough(ctx context.Context, subjectID string, token string, epStatus int) error {
	if epStatus <= 0 {
		return nil
	}
	episodes, err := a.bangumiEpisodes(ctx, subjectID, token)
	if err != nil {
		return err
	}
	ids := bangumiEpisodeIDsThrough(episodes, epStatus)
	if len(ids) == 0 {
		return fmt.Errorf("未找到 Bangumi 第 %d 话以内的本篇章节", epStatus)
	}
	return a.patchBangumiEpisodes(ctx, subjectID, token, ids, 2)
}

func (a *App) bangumiSubjectMainEpisodeCount(ctx context.Context, subjectID string, token string) (int, error) {
	episodes, err := a.bangumiEpisodes(ctx, subjectID, token)
	if err != nil {
		return 0, err
	}
	maxEpisode := 0
	for _, episode := range episodes {
		if episode.Number > maxEpisode {
			maxEpisode = episode.Number
		}
	}
	return maxEpisode, nil
}

func (a *App) patchBangumiEpisodes(ctx context.Context, subjectID string, token string, ids []int, collectType int) error {
	endpoint, err := bangumiEndpoint(a.cfg().BangumiAPIURL, "/users/-/collections/"+subjectID+"/episodes", nil)
	if err != nil {
		return err
	}
	body := map[string]any{
		"episode_id": ids,
		"type":       collectType,
	}
	return patchJSON(ctx, endpoint, a.bangumiUserHeaders(token), body, nil)
}

type bangumiEpisodeRef struct {
	ID     int
	Number int
}

func (a *App) bangumiEpisodes(ctx context.Context, subjectID string, token string) ([]bangumiEpisodeRef, error) {
	const pageLimit = 200
	episodes := make([]bangumiEpisodeRef, 0)
	for offset := 0; ; offset += pageLimit {
		values := url.Values{
			"subject_id": {subjectID},
			"type":       {"0"},
			"limit":      {strconv.Itoa(pageLimit)},
			"offset":     {strconv.Itoa(offset)},
		}
		endpoint, err := bangumiEndpoint(a.cfg().BangumiAPIURL, "/episodes", values)
		if err != nil {
			return nil, err
		}
		var payload map[string]any
		if err := getJSON(ctx, endpoint, a.bangumiUserHeaders(token), &payload); err != nil {
			return nil, err
		}
		rows, _ := payload["data"].([]any)
		for _, row := range rows {
			item, _ := row.(map[string]any)
			if item == nil {
				continue
			}
			episodeNumber := int(numeric(item["ep"]))
			if episodeNumber <= 0 {
				episodeNumber = int(numeric(item["sort"]))
			}
			id := int(numeric(item["id"]))
			if id > 0 && episodeNumber > 0 {
				episodes = append(episodes, bangumiEpisodeRef{ID: id, Number: episodeNumber})
			}
		}
		total := int(numeric(payload["total"]))
		if len(rows) < pageLimit || (total > 0 && offset+len(rows) >= total) {
			break
		}
	}
	return episodes, nil
}

func bangumiEpisodeIDsThrough(episodes []bangumiEpisodeRef, epStatus int) []int {
	ids := make([]int, 0, min(epStatus, len(episodes)))
	for _, episode := range episodes {
		if episode.Number > 0 && episode.Number <= epStatus {
			ids = append(ids, episode.ID)
		}
	}
	return ids
}

func (a *App) bangumiDoneEpisodeIDSet(ctx context.Context, subjectID string, token string) (map[int]bool, error) {
	values := url.Values{
		"episode_type": {"0"},
		"limit":        {"1000"},
		"offset":       {"0"},
	}
	endpoint, err := bangumiEndpoint(a.cfg().BangumiAPIURL, "/users/-/collections/"+subjectID+"/episodes", values)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := getJSON(ctx, endpoint, a.bangumiUserHeaders(token), &payload); err != nil {
		return nil, err
	}
	rows, _ := payload["data"].([]any)
	out := make(map[int]bool, len(rows))
	for _, row := range rows {
		item, _ := row.(map[string]any)
		if item == nil || int(numeric(item["type"])) != 2 {
			continue
		}
		episode, _ := item["episode"].(map[string]any)
		id := int(numeric(episode["id"]))
		if id > 0 {
			out[id] = true
		}
	}
	return out, nil
}
