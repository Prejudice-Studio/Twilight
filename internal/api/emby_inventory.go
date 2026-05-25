package api

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func (a *App) embySearchItems(ctx context.Context, searchTerm string, includeTypes []string, year int, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 20
	}
	params := map[string]string{
		"SearchTerm": searchTerm,
		"Limit":      strconv.Itoa(clamp(limit, 1, 100)),
		"Recursive":  "true",
		"Fields":     "ProviderIds,Overview,OriginalTitle,PremiereDate,ProductionYear,SortName",
	}
	if len(includeTypes) > 0 {
		params["IncludeItemTypes"] = strings.Join(includeTypes, ",")
	}
	if year > 0 {
		params["Years"] = strconv.Itoa(year)
	}
	var payload map[string]any
	if err := a.embyGet(ctx, "/Items"+embyItemQuery(params), &payload); err != nil {
		return nil, err
	}
	rows, _ := payload["Items"].([]any)
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if item, ok := row.(map[string]any); ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func (a *App) embyFindByProviderID(ctx context.Context, provider, id, mediaType string) (map[string]any, bool, error) {
	if id == "" {
		return nil, false, nil
	}
	includeType := "Series"
	if normalizeInventoryMediaType(mediaType) == "movie" {
		includeType = "Movie"
	}
	params := map[string]string{
		"AnyProviderIdEquals": provider + "." + id,
		"IncludeItemTypes":    includeType,
		"Recursive":           "true",
		"Fields":              "ProviderIds,Overview,OriginalTitle,PremiereDate,ProductionYear,SortName",
		"Limit":               "1",
	}
	var payload map[string]any
	if err := a.embyGet(ctx, "/Items"+embyItemQuery(params), &payload); err != nil {
		return nil, false, err
	}
	rows, _ := payload["Items"].([]any)
	if len(rows) == 0 {
		return nil, false, nil
	}
	item, _ := rows[0].(map[string]any)
	return item, item != nil, nil
}

func (a *App) embySeriesSeasons(ctx context.Context, seriesID string) ([]int, error) {
	params := map[string]string{
		"ParentId":         seriesID,
		"IncludeItemTypes": "Season",
		"Recursive":        "false",
		"Fields":           "ProviderIds,Overview,PremiereDate,ProductionYear",
		"SortBy":           "SortName",
		"SortOrder":        "Ascending",
	}
	var payload map[string]any
	if err := a.embyGet(ctx, "/Items"+embyItemQuery(params), &payload); err != nil {
		return nil, err
	}
	rows, _ := payload["Items"].([]any)
	seen := map[int]bool{}
	seasons := []int{}
	for _, row := range rows {
		item, _ := row.(map[string]any)
		n := seasonNumber(item)
		if n > 0 && !seen[n] {
			seen[n] = true
			seasons = append(seasons, n)
		}
	}
	sort.Ints(seasons)
	return seasons, nil
}

func (a *App) embyCheckInventory(ctx context.Context, payload map[string]any) map[string]any {
	season := intValue(payload, "season", 0)
	source := normalizeSource(stringValue(payload, "source"))
	mediaID := firstNonEmpty(stringValue(payload, "media_id"), stringValue(payload, "id"), stringValue(payload, "tmdb_id"))
	title := firstNonEmpty(stringValue(payload, "title"), stringValue(payload, "name"))
	originalTitle := stringValue(payload, "original_title")
	mediaType := normalizeInventoryMediaType(firstNonEmpty(stringValue(payload, "media_type"), stringValue(payload, "type"), "movie"))
	year := intValue(payload, "year", 0)
	if title == "" && mediaID != "" {
		if detail, found := a.mediaDetail(ctx, source, mediaID, mediaType); found {
			title = asString(detail["title"])
			originalTitle = firstNonEmpty(originalTitle, asString(detail["original_title"]))
			if year == 0 {
				year = int(numeric(detail["year"]))
			}
			mediaType = normalizeInventoryMediaType(firstNonEmpty(asString(detail["media_type"]), mediaType))
		}
	}
	if a.cfg().EmbyURL == "" {
		return inventoryResult(false, "库存检查不可用：Emby 未配置", nil, nil, season)
	}
	var item map[string]any
	var found bool
	var err error
	if source == "tmdb" && mediaID != "" {
		item, found, err = a.embyFindByProviderID(ctx, "Tmdb", mediaID, mediaType)
		if err != nil {
			return inventoryResult(false, "库存检查失败："+err.Error(), nil, nil, season)
		}
	}
	if !found && title != "" {
		item, found, err = a.embyFindByTitle(ctx, title, originalTitle, mediaType, year)
		if err != nil {
			return inventoryResult(false, "库存检查失败："+err.Error(), nil, nil, season)
		}
	}
	if !found || item == nil {
		if mediaType == "movie" {
			return inventoryResult(false, "库中暂无此电影", nil, nil, season)
		}
		return inventoryResult(false, "库中暂无此剧集", nil, nil, season)
	}
	if mediaType == "movie" {
		name := firstNonEmpty(asString(item["Name"]), title)
		msg := "库中已有：" + name
		if itemYear := int(numeric(item["ProductionYear"])); itemYear > 0 {
			msg += fmt.Sprintf(" (%d)", itemYear)
		}
		return inventoryResult(true, msg, item, nil, season)
	}
	seasons, err := a.embySeriesSeasons(ctx, asString(item["Id"]))
	if err != nil {
		return inventoryResult(false, "库存检查失败："+err.Error(), item, nil, season)
	}
	name := firstNonEmpty(asString(item["Name"]), title)
	if season > 0 {
		if intSliceContains(seasons, season) {
			return inventoryResult(true, fmt.Sprintf("库中已有：%s 第 %d 季", name, season), item, seasons, season)
		}
		return inventoryResult(false, fmt.Sprintf("库中有 %s，但缺少第 %d 季\n已有季度：%s", name, season, seasonListText(seasons)), item, seasons, season)
	}
	return inventoryResult(true, fmt.Sprintf("库中已有：%s\n已有季度：%s", name, seasonListText(seasons)), item, seasons, season)
}

func (a *App) embyFindByTitle(ctx context.Context, title, originalTitle, mediaType string, year int) (map[string]any, bool, error) {
	searchTitles := []string{strings.TrimSpace(title)}
	if strings.TrimSpace(originalTitle) != "" && !strings.EqualFold(originalTitle, title) {
		searchTitles = append(searchTitles, strings.TrimSpace(originalTitle))
	}
	includeTypes := []string{"Series"}
	if mediaType == "movie" {
		includeTypes = []string{"Movie"}
	}
	for _, searchTitle := range searchTitles {
		if searchTitle == "" {
			continue
		}
		items, err := a.embySearchItems(ctx, searchTitle, includeTypes, year, 10)
		if err != nil {
			return nil, false, err
		}
		for _, item := range items {
			if !embyTitleMatches(searchTitle, item) {
				continue
			}
			if year > 0 {
				itemYear := int(numeric(item["ProductionYear"]))
				if itemYear != 0 && absInt(itemYear-year) > 1 {
					continue
				}
			}
			return item, true, nil
		}
	}
	return nil, false, nil
}

func inventoryResult(exists bool, message string, item map[string]any, seasons []int, season int) map[string]any {
	result := map[string]any{"exists": exists, "message": message, "seasons_available": seasons, "season_requested": nil}
	if season > 0 {
		result["season_requested"] = season
	}
	if item != nil {
		dto := embyItemDTO(item)
		result["item"] = dto
		result["media_item"] = dto
	}
	if seasons == nil {
		result["seasons_available"] = []int{}
	}
	return result
}

func embyItemDTO(item map[string]any) map[string]any {
	return map[string]any{
		"id":             asString(item["Id"]),
		"name":           asString(item["Name"]),
		"type":           asString(item["Type"]),
		"overview":       asString(item["Overview"]),
		"year":           zeroNil(int64(numeric(item["ProductionYear"]))),
		"series_name":    asString(item["SeriesName"]),
		"original_title": asString(item["OriginalTitle"]),
		"premiere_date":  asString(item["PremiereDate"]),
		"tmdb_id":        providerID(item, "Tmdb"),
		"imdb_id":        providerID(item, "Imdb"),
	}
}

func providerID(item map[string]any, key string) string {
	providers, _ := item["ProviderIds"].(map[string]any)
	return asString(providers[key])
}

func normalizeInventoryMediaType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "movie", "电影":
		return "movie"
	default:
		return "tv"
	}
}

func embyTitleMatches(title string, item map[string]any) bool {
	title = strings.ToLower(strings.TrimSpace(title))
	for _, candidate := range []string{asString(item["Name"]), asString(item["OriginalTitle"]), asString(item["SortName"])} {
		if strings.ToLower(strings.TrimSpace(candidate)) == title {
			return true
		}
	}
	return false
}

var seasonNumberRE = regexp.MustCompile(`(?i)(?:Season|第)\s*(\d+)`)

func seasonNumber(item map[string]any) int {
	if n := int(numeric(item["IndexNumber"])); n > 0 {
		return n
	}
	match := seasonNumberRE.FindStringSubmatch(asString(item["Name"]))
	if len(match) == 2 {
		n, _ := strconv.Atoi(match[1])
		return n
	}
	return 0
}

func seasonListText(seasons []int) string {
	if len(seasons) == 0 {
		return "无"
	}
	parts := make([]string, 0, len(seasons))
	for _, season := range seasons {
		parts = append(parts, strconv.Itoa(season))
	}
	return strings.Join(parts, ", ")
}

func intSliceContains(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func embyItemQuery(params map[string]string) string {
	q := url.Values{}
	for key, value := range params {
		if value != "" {
			q.Set(key, value)
		}
	}
	encoded := q.Encode()
	if encoded == "" {
		return ""
	}
	return "?" + encoded
}
