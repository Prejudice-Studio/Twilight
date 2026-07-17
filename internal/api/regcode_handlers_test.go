package api

import (
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestRegcodeMatchesSearchFields(t *testing.T) {
	code := store.RegCode{
		Code:                   "REG-ALPHA",
		Note:                   "VIP Renewal",
		TargetUsername:         "alice",
		TargetTelegramUsername: "alpha_tg",
		TargetTelegramID:       424242,
		UsedBy:                 12,
		UsedByUIDs:             []int64{34, 56},
		UsedByTelegramIDs:      []int64{987654},
	}

	for _, query := range []string{
		"reg-alpha",
		"vip",
		"ALICE",
		"alpha_tg",
		"424242",
		"34",
		"987654",
		"alice alpha_tg",
		"12,34",
	} {
		if !regcodeMatchesSearch(code, query) {
			t.Fatalf("expected regcode search %q to match", query)
		}
	}
	if regcodeMatchesSearch(code, "missing") {
		t.Fatal("unexpected match for missing search text")
	}
}
