package api

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func (a *App) searchMedia(ctx context.Context, query, source, mediaType string, limit int, includeDetails bool) ([]map[string]any, string, map[string]string) {
	if strings.TrimSpace(query) == "" {
		return []map[string]any{}, "OK", nil
	}
	if kind, id, mt, ok := detectMediaID(query); ok {
		if result, found := a.mediaDetail(ctx, kind, id, mt); found {
			return []map[string]any{result}, "OK", nil
		}
	}
	results := []map[string]any{}
	sourceErrors := map[string]string{}
	if source == "all" || source == "tmdb" {
		if tmdb, err := a.searchTMDB(ctx, query, mediaType, limit); err == nil {
			results = append(results, tmdb...)
		} else {
			sourceErrors["tmdb"] = fmt.Sprintf("TMDB 搜索失败：%v", err)
		}
	}
	if source == "all" || source == "bangumi" {
		if bgm, err := a.searchBangumi(ctx, query, limit); err == nil {
			results = append(results, bgm...)
		} else {
			sourceErrors["bangumi"] = fmt.Sprintf("Bangumi 搜索失败：%v", err)
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}
	if len(sourceErrors) == 0 {
		sourceErrors = nil
	}
	return results, "OK", sourceErrors
}

func (a *App) mediaDetail(ctx context.Context, source, id, mediaType string) (map[string]any, bool) {
	source = normalizeSource(source)
	if source == "bangumi" {
		if result, err := a.getBangumi(ctx, id); err == nil {
			return result, true
		}
		return mediaResultFromFields("bangumi", id, "", firstNonEmpty(mediaType, "动画"), ""), true
	}
	if result, err := a.getTMDB(ctx, id, firstNonEmpty(mediaType, "movie")); err == nil {
		return result, true
	}
	return mediaResultFromFields("tmdb", id, "", firstNonEmpty(mediaType, "movie"), ""), true
}

func detectMediaID(query string) (source, id, mediaType string, ok bool) {
	query = strings.TrimSpace(query)
	patterns := []struct {
		re     *regexp.Regexp
		source string
	}{
		{regexp.MustCompile(`(?i)themoviedb\.org/(movie|tv)/(\d+)`), "tmdb"},
		{regexp.MustCompile(`(?i)^tmdb:(?:(movie|tv):)?(\d+)$`), "tmdb"},
		{regexp.MustCompile(`(?i)(?:bgm\.tv|bangumi\.tv)/subject/(\d+)`), "bangumi"},
		{regexp.MustCompile(`(?i)^bgm:(\d+)$`), "bangumi"},
	}
	for _, pattern := range patterns {
		m := pattern.re.FindStringSubmatch(query)
		if len(m) == 0 {
			continue
		}
		if pattern.source == "tmdb" {
			if len(m) == 3 {
				return "tmdb", m[2], firstNonEmpty(m[1], "movie"), true
			}
			return "tmdb", m[len(m)-1], "movie", true
		}
		return "bangumi", m[len(m)-1], "动画", true
	}
	return "", "", "", false
}

func mediaResultFromFields(source, id, title, mediaType, poster string) map[string]any {
	parsedID, _ := strconv.ParseInt(id, 10, 64)
	if title == "" {
		title = source + ":" + id
	}
	sourceURL := ""
	if source == "bangumi" {
		sourceURL = "https://bgm.tv/subject/" + id
	} else {
		sourceURL = "https://www.themoviedb.org/" + firstNonEmpty(mediaType, "movie") + "/" + id
	}
	return map[string]any{"id": parsedID, "title": title, "original_title": title, "media_type": mediaType, "overview": "", "release_date": "", "year": nil, "poster": poster, "poster_url": poster, "vote_average": 0, "rating": 0, "source": source, "source_url": sourceURL, "extra": map[string]any{}}
}
