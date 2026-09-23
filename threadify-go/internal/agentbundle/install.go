// Package agentbundle installs the pinned optional runtime and supervises the embedded agent.
package agentbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed assets/agent.tar.gz
var artifact []byte

//go:embed assets/runtimes.json
var runtimeManifest []byte

type runtimeEntry struct {
	SHA256  string `json:"sha256"`
	Archive string `json:"archive"`
	Python  string `json:"python"`
}
type Options struct{ ConfigPath, EngineURL, Version, CacheDir, ArchivePath string }

func runtimeSpec() (runtimeEntry, error) {
	var entries map[string]runtimeEntry
	if err := json.Unmarshal(runtimeManifest, &entries); err != nil {
		return runtimeEntry{}, err
	}
	entry, ok := entries[runtime.GOOS+"/"+runtime.GOARCH]
	digest, err := hex.DecodeString(entry.SHA256)
	if !ok || err != nil || len(digest) != 32 || !safePath(entry.Python) || filepath.Base(entry.Archive) != entry.Archive {
		return entry, errors.New("this Engine build has no pinned agent runtime for this platform")
	}
	return entry, nil
}

func cacheRoot(options Options) (string, error) {
	root := options.CacheDir
	if root == "" {
		root = os.Getenv("THREADIFY_AGENT_CACHE_DIR")
	}
	if root == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(base, "threadify", "agent")
	}
	return filepath.Abs(root)
}

// InstallRuntime verifies an archive against the Engine's embedded manifest before installation.
// ArchivePath permits offline provisioning without changing the trusted checksum.
func InstallRuntime(ctx context.Context, options Options) (string, error) {
	entry, err := runtimeSpec()
	if err != nil {
		return "", err
	}
	root, err := cacheRoot(options)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	unlock, err := lockCache(ctx, filepath.Join(root, entry.SHA256+".lock"))
	if err != nil {
		return "", err
	}
	defer unlock()
	destination := filepath.Join(root, entry.SHA256)
	if validRuntime(destination, entry) {
		return destination, nil
	}
	if _, err := os.Stat(destination); err == nil {
		return "", errors.New("agent runtime cache is incomplete; remove the affected cache directory and retry")
	}
	stage, err := os.MkdirTemp(root, ".install-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	var input io.ReadCloser
	if options.ArchivePath != "" {
		input, err = os.Open(options.ArchivePath)
	} else {
		version := strings.TrimPrefix(options.Version, "v")
		if version == "" || version == "dev" || strings.ContainsAny(version, "/\\?#") {
			return "", errors.New("development Engine requires --agent-runtime-archive or a preinstalled runtime")
		}
		url := "https://github.com/ThreadifyDev/engine/releases/download/v" + version + "/" + entry.Archive
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if requestErr != nil {
			return "", requestErr
		}
		client := &http.Client{Timeout: 15 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || len(via) > 5 {
				return errors.New("unsafe runtime download redirect")
			}
			return nil
		}}
		response, requestErr := client.Do(request)
		if requestErr != nil {
			return "", fmt.Errorf("download agent runtime: %w", requestErr)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return "", fmt.Errorf("download agent runtime: HTTP %d", response.StatusCode)
		}
		input = response.Body
	}
	if err != nil {
		return "", err
	}
	defer input.Close()
	downloaded, err := os.CreateTemp(root, ".archive-")
	if err != nil {
		return "", err
	}
	defer os.Remove(downloaded.Name())
	defer downloaded.Close()
	hash := sha256.New()
	limited := io.LimitReader(&contextReader{ctx, input}, 512<<20+1)
	count, err := io.Copy(io.MultiWriter(downloaded, hash), limited)
	if err != nil {
		return "", err
	}
	if count > 512<<20 {
		return "", errors.New("agent runtime archive exceeds limit")
	}
	if hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
		return "", errors.New("agent runtime checksum mismatch")
	}
	if _, err = downloaded.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	if err = extract(ctx, downloaded, stage, 1<<30); err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(stage, ".verified"), []byte(entry.SHA256), 0600); err != nil {
		return "", err
	}
	if !validRuntime(stage, entry) {
		return "", errors.New("agent runtime archive is missing required files")
	}
	if err = os.Rename(stage, destination); err != nil {
		return "", err
	}
	return destination, nil
}

func validRuntime(root string, entry runtimeEntry) bool {
	marker, err := os.ReadFile(filepath.Join(root, ".verified"))
	if err != nil || string(marker) != entry.SHA256 {
		return false
	}
	for _, name := range []string{entry.Python, "packages/harnest/runtime.py"} {
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}
func safePath(name string) bool {
	return name != "" && !strings.ContainsAny(name, "\\:") && !strings.HasPrefix(name, "/") && filepath.IsLocal(filepath.FromSlash(name))
}
func extract(ctx context.Context, input io.Reader, destination string, limit int64) error {
	gz, err := gzip.NewReader(input)
	if err != nil {
		return err
	}
	defer gz.Close()
	tarReader := tar.NewReader(gz)
	var total int64
	var count int
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		count++
		if count > 100000 {
			return errors.New("too many runtime archive entries")
		}
		if !safePath(header.Name) {
			return errors.New("unsafe runtime archive path")
		}
		target := filepath.Join(destination, filepath.FromSlash(header.Name))
		switch header.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(target, 0700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			total += header.Size
			if header.Size < 0 || total > limit {
				return errors.New("expanded runtime archive exceeds limit")
			}
			if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			mode := os.FileMode(0600)
			if header.Mode&0111 != 0 {
				mode = 0700
			}
			file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, &contextReader{ctx, tarReader})
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return errors.New("runtime archive links and special files are forbidden")
		}
	}
}

type contextReader struct {
	ctx   context.Context
	input io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.input.Read(p)
}
func extractAgent(ctx context.Context, destination string) error {
	return extract(ctx, bytes.NewReader(artifact), destination, 16<<20)
}
