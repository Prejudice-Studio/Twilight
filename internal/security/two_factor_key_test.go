package security

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestTwoFactorKeyConfiguredFallback(t *testing.T) {
	file := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	environment := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	for _, tc := range []struct {
		name, env, configured string
		want                  byte
	}{
		{"file", "", file, 1},
		{"file whitespace", "", " " + file + "\n", 1},
		{"environment", environment, file, 2},
		{"environment only", environment, "", 2},
		{"invalid override", "invalid", file, 0},
		{"whitespace override", " ", file, 0},
		{"invalid file", "", "invalid", 0},
		{"wrong length", "", base64.StdEncoding.EncodeToString([]byte("short")), 0},
		{"missing", "", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TWILIGHT_TWO_FACTOR_KEY", tc.env)
			key, err := TwoFactorKey(tc.configured)
			if tc.want == 0 {
				if !errors.Is(err, ErrTwoFactorKey) || len(key) != 0 {
					t.Fatal("invalid deployment key accepted")
				}
				if strings.Contains(err.Error(), file) {
					t.Fatal("key leaked in error")
				}
			} else if err != nil || !bytes.Equal(key, bytes.Repeat([]byte{tc.want}, 32)) {
				t.Fatal("wrong deployment key selected")
			}
		})
	}
}
