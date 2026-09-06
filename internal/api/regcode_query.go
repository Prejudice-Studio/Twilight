package api

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

// regcodeListFilter is shared by the V1 compatibility resource, the V2 resource,
// and select-all batch deletion. Keeping the match rules in one place prevents a
// destructive operation from targeting a different set than the visible list.
type regcodeListFilter struct {
	typeValue string
	status    string
	source    string
	search    string
}

func regcodeListFilterFromQuery(query url.Values) regcodeListFilter {
	return regcodeListFilter{
		typeValue: strings.TrimSpace(query.Get("type")),
		status:    strings.ToLower(strings.TrimSpace(query.Get("status"))),
		source:    strings.ToLower(strings.TrimSpace(query.Get("source"))),
		search:    strings.ToLower(strings.TrimSpace(query.Get("search"))),
	}
}

func regcodeListFilterFromPayload(payload map[string]any) regcodeListFilter {
	filter, _ := payload["filter"].(map[string]any)
	return regcodeListFilter{
		typeValue: strings.TrimSpace(asString(filter["type"])),
		status:    strings.ToLower(strings.TrimSpace(asString(filter["status"]))),
		source:    strings.ToLower(strings.TrimSpace(asString(filter["source"]))),
		search:    strings.ToLower(strings.TrimSpace(asString(filter["search"]))),
	}
}

func (filter regcodeListFilter) matches(code store.RegCode) bool {
	if filter.typeValue != "" && filter.typeValue != "all" && strconv.Itoa(code.Type) != filter.typeValue {
		return false
	}
	if filter.source != "" && filter.source != "all" {
		source := code.Source
		if source == "" {
			source = "admin"
		}
		if source != filter.source {
			return false
		}
	}
	if filter.status != "" && filter.status != "all" && regcodeStatus(code) != filter.status {
		// Preserve the historical aliases used by the admin UI: "decoy" means
		// any decoy code and "active" means any code whose active flag is set.
		if !(filter.status == "decoy" && code.IsDecoy) && !(filter.status == "active" && code.Active) {
			return false
		}
	}
	return regcodeMatchesSearch(code, filter.search)
}
