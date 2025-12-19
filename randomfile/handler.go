package randomfile

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func (rf *RandomFile) Provision(ctx caddy.Context) error {
	rf.logger = ctx.Logger(rf)

	for _, p := range rf.Include {
		rf.includeLower = append(rf.includeLower, strings.ToLower(filepath.ToSlash(p)))
	}

	return nil
}

func (rf *RandomFile) Validate() error {
	return nil
}

func (rf *RandomFile) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	root := rf.Root
	if root == "" {
		root = "."
	}

	targetDir, err := rf.resolveTargetDir(root, r)
	if err != nil {
		return caddyhttp.Error(http.StatusBadRequest, err)
	}

	selectedPath, err := rf.pickRandomFile(targetDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return caddyhttp.Error(http.StatusNotFound, err)
		}
		if errors.Is(err, errNoMatchingFiles) {
			return caddyhttp.Error(http.StatusNotFound, err)
		}
		rf.logger.Debug("failed selecting random file", zap.Error(err), zap.String("dir", targetDir))
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	// Serve the selected file directly. We don't rewrite URL.Path; instead we use
	// http.ServeFile which will set appropriate headers.
	http.ServeFile(w, r, selectedPath)
	return nil
}

var errNoMatchingFiles = errors.New("no matching files")

func (rf *RandomFile) resolveTargetDir(root string, r *http.Request) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}

	subdir := ""
	if rf.UseURLPathSubdir {
		// Use URL path as subdir. Strip leading slash.
		p := strings.TrimPrefix(r.URL.Path, "/")
		p = strings.TrimSuffix(p, "/")
		p = strings.TrimSpace(p)
		subdir = p
	}

	if subdir == "" {
		return rootAbs, nil
	}

	rel, err := sanitizeRelativeSubdir(subdir)
	if err != nil {
		return "", err
	}

	candidate := filepath.Join(rootAbs, filepath.FromSlash(rel))
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve subdir: %w", err)
	}

	within, err := isWithinDir(rootAbs, candidateAbs)
	if err != nil {
		return "", err
	}
	if !within {
		return "", fmt.Errorf("subdir escapes root")
	}

	return candidateAbs, nil
}

func sanitizeRelativeSubdir(subdir string) (string, error) {
	s := strings.TrimSpace(subdir)
	if s == "" {
		return "", fmt.Errorf("subdir is empty")
	}

	// Use slash paths for sanitization.
	s = filepath.ToSlash(s)

	// Reject absolute paths.
	if strings.HasPrefix(s, "/") {
		return "", fmt.Errorf("subdir must be relative")
	}

	// Clean using path (always slash-separated).
	clean := path.Clean(s)
	if clean == "." || clean == "" {
		return "", fmt.Errorf("subdir is empty")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("subdir must not traverse up")
	}

	// Also reject any remaining '..' segments.
	for _, seg := range strings.Split(clean, "/") {
		if seg == ".." {
			return "", fmt.Errorf("subdir must not traverse up")
		}
	}

	return clean, nil
}

func isWithinDir(rootAbs, candidateAbs string) (bool, error) {
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return false, fmt.Errorf("rel check: %w", err)
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return true, nil
	}
	if strings.HasPrefix(rel, "../") || rel == ".." {
		return false, nil
	}
	return true, nil
}

func (rf *RandomFile) pickRandomFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	var candidates []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}

		name := entry.Name()
		rel := filepath.ToSlash(name)
		if !rf.matchInclude(rel) {
			continue
		}
		candidates = append(candidates, filepath.Join(dir, name))
	}

	if len(candidates) == 0 {
		return "", errNoMatchingFiles
	}

	idx, err := cryptoRandIndex(len(candidates))
	if err != nil {
		return "", err
	}
	return candidates[idx], nil
}

func (rf *RandomFile) matchInclude(relPath string) bool {
	if len(rf.includeLower) == 0 {
		return true
	}

	name := strings.ToLower(relPath)
	name = filepath.ToSlash(name)
	for _, pattern := range rf.includeLower {
		ok, err := filepath.Match(pattern, name)
		if err != nil {
			// Invalid pattern: ignore it and continue (treat as non-match).
			continue
		}
		if ok {
			return true
		}
	}
	return false
}

func cryptoRandIndex(n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("n must be positive")
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

// Unused currently, but keep a context import ready if we later switch to
// caddyhttp fileserver internals.
var _ = context.Background
