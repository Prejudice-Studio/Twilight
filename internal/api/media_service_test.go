package api

import "testing"

func TestInterleaveMediaResultsKeepsBothSourcesWithinLimit(t *testing.T) {
	tmdb := []map[string]any{{"source": "tmdb", "id": 1}, {"source": "tmdb", "id": 2}, {"source": "tmdb", "id": 3}}
	bangumi := []map[string]any{{"source": "bangumi", "id": 11}, {"source": "bangumi", "id": 12}}

	results := interleaveMediaResults(tmdb, bangumi, 4)
	if len(results) != 4 {
		t.Fatalf("result count=%d want=4", len(results))
	}
	wantSources := []string{"tmdb", "bangumi", "tmdb", "bangumi"}
	for index, want := range wantSources {
		if got := asString(results[index]["source"]); got != want {
			t.Fatalf("result[%d].source=%q want=%q", index, got, want)
		}
	}
}

func TestInterleaveMediaResultsUsesRemainingSource(t *testing.T) {
	tmdb := []map[string]any{{"id": 1}}
	bangumi := []map[string]any{{"id": 11}, {"id": 12}, {"id": 13}}

	results := interleaveMediaResults(tmdb, bangumi, 3)
	if len(results) != 3 || asString(results[2]["id"]) != "12" {
		t.Fatalf("unexpected interleaved results: %#v", results)
	}
	if got := interleaveMediaResults(tmdb, bangumi, 0); len(got) != 0 {
		t.Fatalf("zero limit returned %#v", got)
	}
}

func TestTMDBToMediaKeepsDetailedMetadata(t *testing.T) {
	result := tmdbToMedia(map[string]any{
		"id":               float64(123),
		"name":             "示例剧集",
		"original_name":    "Example Series",
		"first_air_date":   "2024-01-02",
		"vote_average":     8.7,
		"vote_count":       1234,
		"tagline":          "A useful tagline",
		"last_air_date":    "2024-04-01",
		"homepage":         "https://example.com/series",
		"episode_run_time": []any{float64(24)},
		"production_countries": []any{
			map[string]any{"name": "日本"},
		},
		"spoken_languages": []any{
			map[string]any{"name": "日语"},
		},
		"production_companies": []any{
			map[string]any{"name": "Example Studio"},
		},
		"created_by": []any{
			map[string]any{"name": "Example Creator"},
		},
		"credits": map[string]any{
			"cast": []any{map[string]any{"name": "Example Actor"}},
			"crew": []any{map[string]any{"name": "Example Director", "job": "Director"}},
		},
		"videos": map[string]any{
			"results": []any{map[string]any{"key": "abcDEF_1234", "site": "YouTube", "type": "Trailer"}},
		},
	}, "tv", "https://image.example")

	if got := result["rating"]; got != 8.7 {
		t.Fatalf("rating=%v want=8.7", got)
	}
	if got := result["runtime"]; got != 24 {
		t.Fatalf("runtime=%v want=24", got)
	}
	if got := result["trailer_url"]; got != "https://www.youtube.com/watch?v=abcDEF_1234" {
		t.Fatalf("trailer_url=%v", got)
	}
	for key, want := range map[string]string{"tagline": "A useful tagline", "end_date": "2024-04-01", "official_url": "https://example.com/series"} {
		if got := asString(result[key]); got != want {
			t.Fatalf("%s=%q want=%q", key, got, want)
		}
	}
	if got := result["vote_count"]; got != 1234 {
		t.Fatalf("vote_count=%v want=1234", got)
	}
	if got := result["creators"].([]string); len(got) != 2 || got[0] != "Example Creator" || got[1] != "Example Director" {
		t.Fatalf("creators=%#v", got)
	}
	if got := result["cast"].([]string); len(got) != 1 || got[0] != "Example Actor" {
		t.Fatalf("cast=%#v", got)
	}
}

func TestBangumiToMediaKeepsDetailedMetadata(t *testing.T) {
	result := bangumiToMedia(map[string]any{
		"id":       float64(243981),
		"name":     "Yagate Kimi ni Naru",
		"name_cn":  "终将成为你",
		"date":     "2018-10-05",
		"platform": "TV",
		"eps":      float64(13),
		"volumes":  float64(2),
		"rating":   map[string]any{"score": 7.8, "rank": float64(394), "total": float64(11001)},
		"tags": []any{
			map[string]any{"name": "百合"},
			map[string]any{"name": "校园"},
		},
		"infobox": []any{
			map[string]any{"key": "别名", "value": []any{map[string]any{"v": "Bloom into You"}}},
			map[string]any{"key": "播放结束", "value": "2018年12月28日"},
			map[string]any{"key": "官方网站", "value": "https://example.com"},
			map[string]any{"key": "导演", "value": "加藤誠"},
			map[string]any{"key": "动画制作", "value": "TROYCA"},
		},
	})

	if got := result["rating"]; got != 7.8 {
		t.Fatalf("rating=%v want=7.8", got)
	}
	if got := result["episodes"]; got != 13 {
		t.Fatalf("episodes=%v want=13", got)
	}
	if got := result["vote_count"]; got != 11001 {
		t.Fatalf("vote_count=%v want=11001", got)
	}
	if got := result["aliases"].([]string); len(got) != 1 || got[0] != "Bloom into You" {
		t.Fatalf("aliases=%#v", got)
	}
	if got := result["creators"].([]string); len(got) != 1 || got[0] != "加藤誠" {
		t.Fatalf("creators=%#v", got)
	}
	if got := result["studios"].([]string); len(got) != 1 || got[0] != "TROYCA" {
		t.Fatalf("studios=%#v", got)
	}
}
