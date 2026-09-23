package agentbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testArchive(t *testing.T, headers []*tar.Header) []byte {
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tw := tar.NewWriter(gz)
	for _, h := range headers {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			tw.Write(bytes.Repeat([]byte("x"), int(h.Size)))
		}
	}
	tw.Close()
	gz.Close()
	return data.Bytes()
}
func TestExtractRejectsUnsafeArchives(t *testing.T) {
	for _, h := range []*tar.Header{{Name: "../outside", Typeflag: tar.TypeReg}, {Name: "/absolute", Typeflag: tar.TypeReg}, {Name: `C:\escape`, Typeflag: tar.TypeReg}, {Name: "linked", Typeflag: tar.TypeSymlink, Linkname: "../outside"}, {Name: "hard", Typeflag: tar.TypeLink, Linkname: "outside"}, {Name: "device", Typeflag: tar.TypeChar}} {
		t.Run(h.Name, func(t *testing.T) {
			if err := extract(context.Background(), bytes.NewReader(testArchive(t, []*tar.Header{h})), t.TempDir(), 1024); err == nil {
				t.Fatal("accepted unsafe archive")
			}
		})
	}
	if err := extract(context.Background(), bytes.NewReader(testArchive(t, []*tar.Header{{Name: "large", Size: 32, Typeflag: tar.TypeReg}})), t.TempDir(), 8); err == nil {
		t.Fatal("accepted oversized archive")
	}
}
func TestInstallVerifiedCacheAndChecksum(t *testing.T) {
	original := runtimeManifest
	defer func() { runtimeManifest = original }()
	archive := testArchive(t, []*tar.Header{{Name: "python/bin/python3.12", Typeflag: tar.TypeReg, Size: 1, Mode: 0755}, {Name: "packages/harnest/runtime.py", Typeflag: tar.TypeReg, Size: 1}})
	digest := sha256.Sum256(archive)
	sum := hex.EncodeToString(digest[:])
	entry := runtimeEntry{sum, "runtime-test.tar.gz", "python/bin/python3.12"}
	runtimeManifest, _ = json.Marshal(map[string]runtimeEntry{runtime.GOOS + "/" + runtime.GOARCH: entry})
	input := filepath.Join(t.TempDir(), "runtime.tar.gz")
	os.WriteFile(input, archive, 0600)
	options := Options{CacheDir: t.TempDir(), ArchivePath: input}
	first, err := InstallRuntime(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(input)
	second, err := InstallRuntime(context.Background(), options)
	if err != nil || first != second {
		t.Fatalf("cache miss: %v", err)
	}
	options.CacheDir = t.TempDir()
	os.WriteFile(input, []byte("corrupt"), 0600)
	if _, err = InstallRuntime(context.Background(), options); err == nil {
		t.Fatal("accepted invalid checksum")
	}
}
func TestCacheLockCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	unlock, err := lockCache(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := lockCache(ctx, path); err == nil {
		t.Fatal("lock ignored cancellation")
	}
}
func TestEmbeddedAgentIsProductionArtifact(t *testing.T) {
	dir := t.TempDir()
	if err := extractAgent(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"launch.py", "harnest-manifest.json", "source/agent.py", "source/tools/get_page_context.py"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "source/extensions")); !os.IsNotExist(err) {
		t.Fatal("demo SDK extensions included")
	}
}
