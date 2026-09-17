package migration

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestCreateOpenPlainArchive(t *testing.T) {
	input := Input{
		TwilightVersion:       "test",
		DatabaseSchemaVersion: "1",
		Files: []InputFile{
			{Path: "data/state.json", Kind: "data", ContentType: "application/json", Data: []byte(`{"users":1}`)},
			{Path: "resources/avatar.bin", Kind: "resource", ContentType: "image/png", Data: []byte("avatar")},
		},
	}
	archive, manifest, err := Create(input)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if manifest.Encrypted || manifest.FormatVersion != FormatVersion {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	opened, err := Open(archive, "", DefaultLimits())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if string(opened.Files["data/state.json"]) != `{"users":1}` || string(opened.Files["resources/avatar.bin"]) != "avatar" {
		t.Fatalf("opened files mismatch: %#v", opened.Files)
	}
}

func TestCreateOpenEncryptedArchive(t *testing.T) {
	archive, manifest, err := Create(Input{
		TwilightVersion:       "test",
		DatabaseSchemaVersion: "1",
		Password:              "correct horse battery staple",
		Files:                 []InputFile{{Path: "data/state.json", Kind: "data", Data: []byte("secret")}},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !manifest.Encrypted || manifest.KDF == nil || manifest.KDF.Cipher != "AES-256-GCM" {
		t.Fatalf("missing encryption metadata: %+v", manifest)
	}
	if _, err := Open(archive, "wrong password", DefaultLimits()); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("wrong password error = %v, want ErrInvalidPassword", err)
	}
	if _, err := Open(archive, "", DefaultLimits()); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("missing password error = %v, want ErrPasswordRequired", err)
	}
	opened, err := Open(archive, "correct horse battery staple", DefaultLimits())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if string(opened.Files["data/state.json"]) != "secret" {
		t.Fatalf("decrypted content = %q", opened.Files["data/state.json"])
	}
}

func TestEncryptedArchiveBindsManifestAsAAD(t *testing.T) {
	archive, _, err := Create(Input{
		TwilightVersion:       "test",
		DatabaseSchemaVersion: "1",
		Password:              "correct horse battery staple",
		Files:                 []InputFile{{Path: "data/state.json", Kind: "data", Data: []byte("secret")}},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	tampered := rewriteOuterArchive(t, archive, func(name string, content []byte) []byte {
		if name != manifestName {
			return content
		}
		var manifest Manifest
		if err := json.Unmarshal(content, &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.TwilightVersion = "tampered"
		updated, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return updated
	})
	// The manifest hash is updated, so this specifically exercises GCM AAD,
	// rather than only the outer manifest checksum.
	manifestContent := readOuterEntry(t, tampered, manifestName)
	tampered = rewriteOuterArchive(t, tampered, func(name string, content []byte) []byte {
		if name != manifestHashName {
			return content
		}
		digest := sha256.Sum256(manifestContent)
		return []byte(hex.EncodeToString(digest[:]))
	})
	if _, err := Open(tampered, "correct horse battery staple", DefaultLimits()); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("tampered manifest error = %v, want ErrInvalidPassword", err)
	}
}

func TestOpenRejectsTraversalAndUnexpectedOuterFiles(t *testing.T) {
	traversal := makeZip(t, map[string][]byte{"../escape": []byte("x")})
	if _, err := Open(traversal, "", DefaultLimits()); !errors.Is(err, ErrPathRejected) {
		t.Fatalf("traversal error = %v, want ErrPathRejected", err)
	}

	archive, _, err := Create(Input{
		TwilightVersion: "test",
		Files:           []InputFile{{Path: "data/state.json", Kind: "data", Data: []byte("x")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	withExtra := rewriteOuterArchive(t, archive, func(name string, content []byte) []byte { return content })
	withExtra = appendZipEntry(t, withExtra, "unexpected", []byte("x"))
	if _, err := Open(withExtra, "", DefaultLimits()); !errors.Is(err, ErrInvalidArchive) {
		t.Fatalf("unexpected outer file error = %v, want ErrInvalidArchive", err)
	}
}

func TestOpenVerifiesFileHashAndCustomLimits(t *testing.T) {
	archive, _, err := Create(Input{
		TwilightVersion: "test",
		Files:           []InputFile{{Path: "data/state.json", Kind: "data", Data: []byte("content")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(archive, "", Limits{MaxFiles: 10, MaxFileBytes: 2, MaxTotalBytes: 10, MaxManifest: maxManifestBytes}); !errors.Is(err, ErrArchiveLimit) {
		t.Fatalf("custom limit error = %v, want ErrArchiveLimit", err)
	}

	tampered := rewriteOuterArchive(t, archive, func(name string, content []byte) []byte {
		if name != manifestName {
			return content
		}
		var manifest Manifest
		if err := json.Unmarshal(content, &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.Files[0].SHA256 = strings.Repeat("0", sha256.Size*2)
		updated, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return updated
	})
	manifestContent := readOuterEntry(t, tampered, manifestName)
	tampered = rewriteOuterArchive(t, tampered, func(name string, content []byte) []byte {
		if name != manifestHashName {
			return content
		}
		digest := sha256.Sum256(manifestContent)
		return []byte(hex.EncodeToString(digest[:]))
	})
	if _, err := Open(tampered, "", DefaultLimits()); err == nil {
		t.Fatal("tampered payload unexpectedly opened")
	}
}

func TestCreateRejectsPathOutsideMigrationNamespaces(t *testing.T) {
	_, _, err := Create(Input{
		TwilightVersion: "test",
		Files:           []InputFile{{Path: "private/state.json", Kind: "data", Data: []byte("x")}},
	})
	if !errors.Is(err, ErrPathRejected) {
		t.Fatalf("outside namespace error = %v, want ErrPathRejected", err)
	}
}

func makeZip(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func readOuterEntry(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range reader.File {
		if entry.Name != name {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		return content
	}
	t.Fatalf("missing outer entry %q", name)
	return nil
}

func rewriteOuterArchive(t *testing.T, data []byte, rewrite func(string, []byte) []byte) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, entry := range reader.File {
		rc, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		content = rewrite(entry.Name, content)
		header := &zip.FileHeader{Name: entry.Name, Method: zip.Deflate, Modified: time.Unix(0, 0).UTC()}
		w, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func appendZipEntry(t *testing.T, data []byte, name string, content []byte) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, entry := range reader.File {
		rc, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		old, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.Create(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(old); err != nil {
			t.Fatal(err)
		}
	}
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
