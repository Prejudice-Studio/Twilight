package migration

import (
	"archive/zip"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	FormatVersion = "twilight-export/v1"

	manifestName     = "manifest.json"
	manifestHashName = "manifest.sha256"
	payloadName      = "payload.enc"

	maxArchiveFiles      = 4096
	maxArchiveFileBytes  = 128 << 20
	maxArchiveTotalBytes = 512 << 20
	maxManifestBytes     = 1 << 20
	maxPasswordBytes     = 1024
	maxLogicalPathBytes  = 512
	maxArgon2MemoryKiB   = 256 * 1024
	maxArgon2Time        = 10
	maxArgon2Threads     = 8
	argon2MemoryKiB      = 64 * 1024
	argon2Time           = 3
	argon2Threads        = 2
	argon2KeyBytes       = 32
	argon2SaltBytes      = 16
	aesNonceBytes        = 12
)

// MaxArchiveBytes is the maximum encoded outer ZIP size accepted by the HTTP
// migration boundary. The format's uncompressed budget is smaller, but the
// outer archive also contains ZIP headers and can be incompressible.
const MaxArchiveBytes int64 = maxArchiveTotalBytes + 16<<20

var (
	ErrInvalidArchive    = errors.New("invalid Twilight migration archive")
	ErrUnsupportedFormat = errors.New("unsupported Twilight migration format")
	ErrPasswordRequired  = errors.New("migration archive password required")
	ErrInvalidPassword   = errors.New("invalid migration archive password")
	ErrArchiveLimit      = errors.New("migration archive exceeds safety limits")
	ErrPathRejected      = errors.New("migration archive path rejected")
	ErrIntegrity         = errors.New("migration archive integrity check failed")
)

type Manifest struct {
	FormatVersion         string      `json:"format_version"`
	TwilightVersion       string      `json:"twilight_version"`
	DatabaseSchemaVersion string      `json:"database_schema_version"`
	ExportedAt            time.Time   `json:"exported_at"`
	Encrypted             bool        `json:"encrypted"`
	KDF                   *KDFParams  `json:"kdf,omitempty"`
	Files                 []FileEntry `json:"files"`
}

type KDFParams struct {
	Algorithm  string `json:"algorithm"`
	MemoryKiB  uint32 `json:"memory_kib"`
	Iterations uint32 `json:"iterations"`
	Threads    uint8  `json:"threads"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Cipher     string `json:"cipher"`
}

type FileEntry struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	Kind        string `json:"kind"`
	ContentType string `json:"content_type,omitempty"`
}

type Input struct {
	TwilightVersion       string
	DatabaseSchemaVersion string
	Files                 []InputFile
	Password              string
}

type InputFile struct {
	Path        string
	Kind        string
	ContentType string
	Data        []byte
}

type Archive struct {
	Manifest Manifest
	Files    map[string][]byte
}

type Limits struct {
	MaxFiles      int
	MaxFileBytes  int64
	MaxTotalBytes int64
	MaxManifest   int64
}

func DefaultLimits() Limits {
	return Limits{
		MaxFiles:      maxArchiveFiles,
		MaxFileBytes:  maxArchiveFileBytes,
		MaxTotalBytes: maxArchiveTotalBytes,
		MaxManifest:   maxManifestBytes,
	}
}

func (m Manifest) validate() error {
	if m.FormatVersion != FormatVersion {
		return fmt.Errorf("%w: %q", ErrUnsupportedFormat, m.FormatVersion)
	}
	if m.ExportedAt.IsZero() || len(m.Files) == 0 || m.TwilightVersion == "" || m.DatabaseSchemaVersion == "" || len(m.TwilightVersion) > 256 || len(m.DatabaseSchemaVersion) > 256 {
		return fmt.Errorf("%w: missing export metadata or files", ErrInvalidArchive)
	}
	if len(m.Files) > maxArchiveFiles {
		return ErrArchiveLimit
	}
	seen := make(map[string]struct{}, len(m.Files))
	var total int64
	for _, entry := range m.Files {
		if err := validateDataPath(entry.Path); err != nil {
			return err
		}
		if entry.Size < 0 || entry.Size > maxArchiveFileBytes || total+entry.Size > maxArchiveTotalBytes || !isSHA256(entry.SHA256) || len(entry.Kind) > 32 || len(entry.ContentType) > 128 {
			return fmt.Errorf("%w: invalid file entry", ErrInvalidArchive)
		}
		if _, ok := seen[entry.Path]; ok {
			return fmt.Errorf("%w: duplicate file %q", ErrInvalidArchive, entry.Path)
		}
		seen[entry.Path] = struct{}{}
		if entry.Kind != "data" && entry.Kind != "config" && entry.Kind != "resource" {
			return fmt.Errorf("%w: invalid file kind", ErrInvalidArchive)
		}
		if (entry.Kind == "data" && !strings.HasPrefix(entry.Path, "data/")) || (entry.Kind == "config" && !strings.HasPrefix(entry.Path, "config/")) || (entry.Kind == "resource" && !strings.HasPrefix(entry.Path, "resources/")) {
			return fmt.Errorf("%w: file kind does not match path namespace", ErrInvalidArchive)
		}
		total += entry.Size
	}
	if m.Encrypted {
		if m.KDF == nil {
			return fmt.Errorf("%w: encrypted archive has no KDF", ErrInvalidArchive)
		}
		if err := validateKDF(*m.KDF); err != nil {
			return err
		}
	} else if m.KDF != nil {
		return fmt.Errorf("%w: unencrypted archive has KDF metadata", ErrInvalidArchive)
	}
	return nil
}

func validateKDF(k KDFParams) error {
	if k.Algorithm != "argon2id" || k.Cipher != "AES-256-GCM" || k.MemoryKiB == 0 || k.MemoryKiB > maxArgon2MemoryKiB || k.Iterations == 0 || k.Iterations > maxArgon2Time || k.Threads == 0 || k.Threads > maxArgon2Threads {
		return fmt.Errorf("%w: invalid encryption parameters", ErrInvalidArchive)
	}
	salt, err := base64.RawStdEncoding.DecodeString(k.Salt)
	if err != nil || len(salt) != argon2SaltBytes {
		return fmt.Errorf("%w: invalid encryption salt", ErrInvalidArchive)
	}
	nonce, err := base64.RawStdEncoding.DecodeString(k.Nonce)
	if err != nil || len(nonce) != aesNonceBytes {
		return fmt.Errorf("%w: invalid encryption nonce", ErrInvalidArchive)
	}
	return nil
}

func validateLogicalPath(raw string) error {
	if raw == "" || len(raw) > maxLogicalPathBytes || strings.ContainsRune(raw, '\x00') || strings.Contains(raw, "\\") || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return fmt.Errorf("%w: %q", ErrPathRejected, raw)
	}
	clean := path.Clean(raw)
	if clean != raw || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%w: %q", ErrPathRejected, raw)
	}
	return nil
}

func validateDataPath(raw string) error {
	if err := validateLogicalPath(raw); err != nil {
		return err
	}
	if !strings.HasPrefix(raw, "data/") && !strings.HasPrefix(raw, "config/") && !strings.HasPrefix(raw, "resources/") {
		return fmt.Errorf("%w: %q is outside an allowed migration namespace", ErrPathRejected, raw)
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func Create(input Input) ([]byte, Manifest, error) {
	if len(input.Files) == 0 || len(input.Files) > maxArchiveFiles || len([]byte(input.Password)) > maxPasswordBytes {
		return nil, Manifest{}, fmt.Errorf("%w: invalid input", ErrInvalidArchive)
	}
	if input.Password != "" && len([]byte(input.Password)) < 8 {
		return nil, Manifest{}, ErrInvalidPassword
	}
	files := make(map[string]InputFile, len(input.Files))
	manifestFiles := make([]FileEntry, 0, len(input.Files))
	var total int64
	for _, file := range input.Files {
		if err := validateDataPath(file.Path); err != nil {
			return nil, Manifest{}, err
		}
		if file.Path == manifestName || file.Path == manifestHashName || file.Path == payloadName {
			return nil, Manifest{}, fmt.Errorf("%w: reserved path", ErrPathRejected)
		}
		if file.Kind != "data" && file.Kind != "config" && file.Kind != "resource" {
			return nil, Manifest{}, fmt.Errorf("%w: invalid file kind", ErrInvalidArchive)
		}
		if int64(len(file.Data)) > maxArchiveFileBytes || total+int64(len(file.Data)) > maxArchiveTotalBytes {
			return nil, Manifest{}, ErrArchiveLimit
		}
		if _, exists := files[file.Path]; exists {
			return nil, Manifest{}, fmt.Errorf("%w: duplicate file %q", ErrInvalidArchive, file.Path)
		}
		files[file.Path] = file
		digest := sha256.Sum256(file.Data)
		manifestFiles = append(manifestFiles, FileEntry{
			Path: file.Path, Size: int64(len(file.Data)), SHA256: hex.EncodeToString(digest[:]), Kind: file.Kind, ContentType: file.ContentType,
		})
		total += int64(len(file.Data))
	}

	manifest := Manifest{
		FormatVersion: FormatVersion, TwilightVersion: input.TwilightVersion, DatabaseSchemaVersion: input.DatabaseSchemaVersion,
		ExportedAt: time.Now().UTC(), Encrypted: input.Password != "", Files: manifestFiles,
	}
	if manifest.TwilightVersion == "" {
		manifest.TwilightVersion = "unknown"
	}
	if manifest.DatabaseSchemaVersion == "" {
		manifest.DatabaseSchemaVersion = "unknown"
	}
	var payload []byte
	if manifest.Encrypted {
		var err error
		payload, err = createEncryptedPayload(files, manifestFiles, input.Password, &manifest)
		if err != nil {
			return nil, Manifest{}, err
		}
	} else {
		var err error
		payload, err = createPlainPayload(files, manifestFiles)
		if err != nil {
			return nil, Manifest{}, err
		}
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, Manifest{}, err
	}
	if int64(len(manifestBytes)) > maxManifestBytes {
		return nil, Manifest{}, ErrArchiveLimit
	}
	return writeOuterArchive(manifestBytes, payload, manifest.Encrypted)
}

func createPlainPayload(files map[string]InputFile, entries []FileEntry) ([]byte, error) {
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, entry := range entries {
		if err := writeZipFile(zw, entry.Path, files[entry.Path].Data); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func createEncryptedPayload(files map[string]InputFile, entries []FileEntry, password string, manifest *Manifest) ([]byte, error) {
	inner, err := createPlainPayload(files, entries)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, argon2SaltBytes)
	nonce := make([]byte, aesNonceBytes)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	manifest.KDF = &KDFParams{Algorithm: "argon2id", MemoryKiB: argon2MemoryKiB, Iterations: argon2Time, Threads: argon2Threads, Salt: base64.RawStdEncoding.EncodeToString(salt), Nonce: base64.RawStdEncoding.EncodeToString(nonce), Cipher: "AES-256-GCM"}
	key := argon2.IDKey([]byte(password), salt, argon2Time, argon2MemoryKiB, argon2Threads, argon2KeyBytes)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if int64(len(manifestBytes)) > maxManifestBytes {
		return nil, ErrArchiveLimit
	}
	return gcm.Seal(nil, nonce, inner, manifestBytes), nil
}

func writeOuterArchive(manifestBytes, payload []byte, encrypted bool) ([]byte, Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, Manifest{}, err
	}
	digest := sha256.Sum256(manifestBytes)
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	if err := writeZipFile(zw, manifestName, manifestBytes); err != nil {
		return nil, Manifest{}, err
	}
	if err := writeZipFile(zw, manifestHashName, []byte(hex.EncodeToString(digest[:]))); err != nil {
		return nil, Manifest{}, err
	}
	name := "payload.zip"
	if encrypted {
		name = payloadName
	}
	if err := writeZipFile(zw, name, payload); err != nil {
		return nil, Manifest{}, err
	}
	if err := zw.Close(); err != nil {
		return nil, Manifest{}, err
	}
	return out.Bytes(), manifest, nil
}

func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	if err := validateLogicalPath(name); err != nil {
		return err
	}
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func Open(data []byte, password string, limits Limits) (Archive, error) {
	if len(data) == 0 {
		return Archive{}, fmt.Errorf("%w: empty archive", ErrInvalidArchive)
	}
	if limits.MaxFiles <= 0 || limits.MaxFileBytes <= 0 || limits.MaxTotalBytes <= 0 || limits.MaxManifest <= 0 {
		limits = DefaultLimits()
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Archive{}, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
	}
	if len(reader.File) > limits.MaxFiles+3 {
		return Archive{}, ErrArchiveLimit
	}
	outer := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		if err := validateZipEntry(file); err != nil {
			return Archive{}, err
		}
		if _, exists := outer[file.Name]; exists {
			return Archive{}, fmt.Errorf("%w: duplicate outer file", ErrInvalidArchive)
		}
		outer[file.Name] = file
	}
	manifestBytes, err := readZipEntry(outer[manifestName], limits.MaxManifest)
	if err != nil {
		return Archive{}, err
	}
	hashBytes, err := readZipEntry(outer[manifestHashName], 128)
	if err != nil {
		return Archive{}, err
	}
	actual := sha256.Sum256(manifestBytes)
	if !strings.EqualFold(strings.TrimSpace(string(hashBytes)), hex.EncodeToString(actual[:])) {
		return Archive{}, ErrIntegrity
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return Archive{}, fmt.Errorf("%w: malformed manifest", ErrInvalidArchive)
	}
	if err := manifest.validate(); err != nil {
		return Archive{}, err
	}
	if len(outer) != 3 {
		return Archive{}, fmt.Errorf("%w: unexpected outer files", ErrInvalidArchive)
	}
	payloadEntryName := "payload.zip"
	if manifest.Encrypted {
		payloadEntryName = payloadName
	}
	for _, name := range []string{manifestName, manifestHashName, payloadEntryName} {
		if outer[name] == nil {
			return Archive{}, fmt.Errorf("%w: missing outer file", ErrInvalidArchive)
		}
	}
	if manifest.Encrypted && password == "" {
		return Archive{}, ErrPasswordRequired
	}
	if !manifest.Encrypted && password != "" {
		return Archive{}, fmt.Errorf("%w: archive is not password protected", ErrInvalidPassword)
	}
	var payload []byte
	if manifest.Encrypted {
		payload, err = readZipEntry(outer[payloadName], limits.MaxTotalBytes)
		if err != nil {
			return Archive{}, err
		}
		payload, err = decryptPayload(payload, password, *manifest.KDF, manifestBytes)
		if err != nil {
			return Archive{}, ErrInvalidPassword
		}
	} else {
		payload, err = readZipEntry(outer["payload.zip"], limits.MaxTotalBytes)
		if err != nil {
			return Archive{}, err
		}
	}
	files, err := readPayload(payload, manifest, limits)
	if err != nil {
		return Archive{}, err
	}
	return Archive{Manifest: manifest, Files: files}, nil
}

func decryptPayload(payload []byte, password string, k KDFParams, manifestBytes []byte) ([]byte, error) {
	salt, err := base64.RawStdEncoding.DecodeString(k.Salt)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.RawStdEncoding.DecodeString(k.Nonce)
	if err != nil {
		return nil, err
	}
	key := argon2.IDKey([]byte(password), salt, k.Iterations, k.MemoryKiB, k.Threads, argon2KeyBytes)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, payload, manifestBytes)
}

func readPayload(payload []byte, manifest Manifest, limits Limits) (map[string][]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, fmt.Errorf("%w: malformed payload", ErrInvalidArchive)
	}
	if len(reader.File) != len(manifest.Files) || len(reader.File) > limits.MaxFiles {
		return nil, ErrArchiveLimit
	}
	wanted := make(map[string]FileEntry, len(manifest.Files))
	for _, entry := range manifest.Files {
		wanted[entry.Path] = entry
	}
	files := make(map[string][]byte, len(manifest.Files))
	seen := make(map[string]struct{}, len(manifest.Files))
	var total int64
	for _, file := range reader.File {
		if err := validateZipEntry(file); err != nil {
			return nil, err
		}
		entry, ok := wanted[file.Name]
		if !ok || file.UncompressedSize64 > uint64(limits.MaxFileBytes) || file.UncompressedSize64 > uint64(maxArchiveFileBytes) {
			return nil, ErrArchiveLimit
		}
		if _, duplicate := seen[file.Name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate payload file", ErrInvalidArchive)
		}
		if total+int64(file.UncompressedSize64) > limits.MaxTotalBytes {
			return nil, ErrArchiveLimit
		}
		content, err := readZipEntry(file, minPositive(limits.MaxFileBytes, maxArchiveFileBytes))
		if err != nil {
			return nil, err
		}
		if int64(len(content)) != entry.Size {
			return nil, ErrIntegrity
		}
		digest := sha256.Sum256(content)
		if !strings.EqualFold(entry.SHA256, hex.EncodeToString(digest[:])) {
			return nil, ErrIntegrity
		}
		files[file.Name] = content
		seen[file.Name] = struct{}{}
		total += int64(len(content))
	}
	if len(files) != len(manifest.Files) {
		return nil, fmt.Errorf("%w: payload file set does not match manifest", ErrIntegrity)
	}
	return files, nil
}

func validateZipEntry(file *zip.File) error {
	if file == nil || file.Name == "" || file.Name == "." {
		return fmt.Errorf("%w: missing ZIP entry", ErrPathRejected)
	}
	if err := validateLogicalPath(file.Name); err != nil {
		return err
	}
	if file.FileInfo().Mode()&0o170000 != 0 || !file.FileInfo().Mode().IsRegular() {
		return fmt.Errorf("%w: non-regular ZIP entry", ErrPathRejected)
	}
	return nil
}

func readZipEntry(file *zip.File, limit int64) ([]byte, error) {
	if file == nil {
		return nil, fmt.Errorf("%w: missing ZIP entry", ErrInvalidArchive)
	}
	if limit <= 0 || file.UncompressedSize64 > uint64(limit) {
		return nil, ErrArchiveLimit
	}
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrArchiveLimit
	}
	return data, nil
}

func minPositive(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
