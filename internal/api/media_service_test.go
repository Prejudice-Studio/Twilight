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
