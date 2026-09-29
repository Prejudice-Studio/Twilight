package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var ErrTwoFactorKey = errors.New("two-factor encryption key unavailable or invalid")
var ErrTwoFactorCode = errors.New("two-factor code invalid or already used")

// TwoFactorKey accepts the server configuration as a fallback. A nonempty
// environment override takes precedence, including invalid values (fail closed).
// Callers without a configuration retain the environment-only behavior.
func TwoFactorKey(configured ...string) ([]byte, error) {
	value := os.Getenv("TWILIGHT_TWO_FACTOR_KEY")
	if value == "" && len(configured) > 0 {
		value = configured[0]
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(key) != 32 {
		return nil, ErrTwoFactorKey
	}
	return key, nil
}

func NewTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

func TOTPUri(secret, issuer, account string) string {
	q := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + q.Encode()
}

func totpAt(secret string, step int64, digits int) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil || len(key) < 20 || step < 0 || (digits != 6 && digits != 8) {
		return "", ErrTwoFactorCode
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	h := hmac.New(sha1.New, key)
	h.Write(counter[:])
	sum := h.Sum(nil)
	offset := sum[len(sum)-1] & 15
	n := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	mod := uint32(1000000)
	if digits == 8 {
		mod = 100000000
	}
	return fmt.Sprintf("%0*d", digits, n%mod), nil
}

func TOTPCode(secret string, now time.Time) (string, error) { return totpAt(secret, now.Unix()/30, 6) }

// The caller persists the accepted step in the same transaction as consumption.
func VerifyTOTP(secret, code string, now time.Time, lastStep int64) (int64, error) {
	if len(code) != 6 {
		return 0, ErrTwoFactorCode
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return 0, ErrTwoFactorCode
		}
	}
	step := now.Unix() / 30
	for _, candidate := range []int64{step, step - 1, step + 1} {
		expected, err := totpAt(secret, candidate, 6)
		if err == nil && candidate > lastStep && subtle.ConstantTimeCompare([]byte(code), []byte(expected)) == 1 {
			return candidate, nil
		}
	}
	return 0, ErrTwoFactorCode
}

func SealTOTP(key []byte, uid int64, secret string) (string, error) {
	if len(key) != 32 {
		return "", ErrTwoFactorKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", ErrTwoFactorKey
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(secret), []byte("twilight:totp:v1:"+strconv.FormatInt(uid, 10)))
	return "v1:" + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func OpenTOTP(key []byte, uid int64, sealed string) (string, error) {
	if len(key) != 32 || !strings.HasPrefix(sealed, "v1:") {
		return "", ErrTwoFactorKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", ErrTwoFactorKey
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	b, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(sealed, "v1:"))
	if err != nil || len(b) < gcm.NonceSize()+gcm.Overhead() {
		return "", ErrTwoFactorKey
	}
	plain, err := gcm.Open(nil, b[:gcm.NonceSize()], b[gcm.NonceSize():], []byte("twilight:totp:v1:"+strconv.FormatInt(uid, 10)))
	if err != nil {
		return "", ErrTwoFactorKey
	}
	return string(plain), nil
}

func TwoFactorDigest(kind, value string) string {
	s := sha256.Sum256([]byte("twilight:two-factor:v1:" + kind + ":" + value))
	return hex.EncodeToString(s[:])
}

func RecoveryDigest(code string) string {
	return TwoFactorDigest("recovery", strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", "")))
}

func NewRecoveryCodes() ([]string, []string, error) {
	codes, hashes := make([]string, 10), make([]string, 10)
	for i := range codes {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, err
		}
		s := strings.ToUpper(hex.EncodeToString(b))
		codes[i] = s[:8] + "-" + s[8:16] + "-" + s[16:24] + "-" + s[24:]
		hashes[i] = RecoveryDigest(codes[i])
	}
	return codes, hashes, nil
}
