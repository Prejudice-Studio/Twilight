package api

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBangumiCoverCachedOnDisk 锁定「出站前短路」的判据：任一已知扩展名命中
// 即视为已缓存，且必须拒绝符号链接 / 目录，避免被伪造缓存项骗过短路检查。
func TestBangumiCoverCachedOnDisk(t *testing.T) {
	dir := t.TempDir()

	if bangumiCoverCachedOnDisk(dir, "100") {
		t.Fatal("empty dir should not report cached cover")
	}

	if err := os.WriteFile(filepath.Join(dir, "100.jpg"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !bangumiCoverCachedOnDisk(dir, "100") {
		t.Fatal("existing .jpg cover should count as cached")
	}

	// 其它扩展名同样命中。
	if err := os.WriteFile(filepath.Join(dir, "200.webp"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !bangumiCoverCachedOnDisk(dir, "200") {
		t.Fatal("existing .webp cover should count as cached")
	}

	// 子目录不算命中（只认目录直下的普通文件）。
	if err := os.Mkdir(filepath.Join(dir, "300.jpg"), 0o700); err != nil {
		t.Fatal(err)
	}
	if bangumiCoverCachedOnDisk(dir, "300") {
		t.Fatal("directory named like a cover must not count as cached")
	}
}

// TestIsSafeBangumiImageURL 锁定封面回源 / 下载的 URL 白名单：仅 https + bgm.tv /
// bangumi.tv 后缀主机，防止被诱导向任意外部主机发起出站请求（SSRF 面）。
func TestIsSafeBangumiImageURL(t *testing.T) {
	safe := []string{
		"https://lain.bgm.tv/pic/cover/l/1.jpg",
		"https://api.bgm.tv/x.png",
		"https://foo.bangumi.tv/y.webp",
	}
	for _, u := range safe {
		if !isSafeBangumiImageURL(u) {
			t.Fatalf("expected safe url: %s", u)
		}
	}
	unsafe := []string{
		"",
		"http://lain.bgm.tv/x.jpg",      // 非 https
		"https://evil.com/x.jpg",        // 非白名单主机
		"https://bgm.tv.evil.com/x.jpg", // 后缀伪装
		"https://127.0.0.1/x.jpg",       // 内网
		"ftp://lain.bgm.tv/x.jpg",       // 非 http(s)
	}
	for _, u := range unsafe {
		if isSafeBangumiImageURL(u) {
			t.Fatalf("expected unsafe url rejected: %s", u)
		}
	}
}
