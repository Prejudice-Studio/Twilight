package api

import (
	"errors"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// The deployment key stays server-managed even when the raw TOML editor is used.
// Compare the TOML value, not the effective config: environment overrides must
// neither hide a file change nor be copied into the primary file by a web save.
func preserveTwoFactorConfigKey(submitted, existing string) error {
	before, err := twoFactorConfigKeyValue(existing)
	if err != nil {
		return err
	}
	after, err := twoFactorConfigKeyValue(submitted)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return errors.New("two-factor deployment key is not editable from web config")
	}
	return nil
}

func twoFactorConfigKeyValue(content string) (any, error) {
	var tree map[string]any
	if err := toml.Unmarshal([]byte(content), &tree); err != nil {
		return nil, err
	}
	var result any
	found := false
	for name, value := range tree {
		if !strings.EqualFold(name, "Security") {
			continue
		}
		section, _ := value.(map[string]any)
		for key, value := range section {
			if strings.EqualFold(key, "two_factor_key") {
				if found {
					return nil, errors.New("ambiguous two-factor deployment key")
				}
				result, found = value, true
			}
		}
	}
	return result, nil
}
