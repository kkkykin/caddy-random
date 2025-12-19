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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func (rf *RandomFile) Provision(ctx caddy.Context) error {
	rf.logger = ctx.Logger(rf)

	for _, p := range rf.Include {
		rf.includeLower = append(rf.includeLower, strings.ToLower(filepath.ToSlash(p)))
	}

	rf.cacheMu.Lock()
	if rf.cacheCond == nil {
		rf.cacheCond = sync.NewCond(&rf.cacheMu)
	}
	rf.cacheMu.Unlock()

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

func (rf *RandomFile) resolveTargetDir(root string, _ *http.Request) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	return rootAbs, nil
}

func (rf *RandomFile) pickRandomFile(dir string) (string, error) {
	candidates, err := rf.getCandidates(dir)
	if err != nil {
		return "", err
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

func (rf *RandomFile) getCandidates(dir string) ([]string, error) {
	ttl := time.Duration(rf.Cache)
	if ttl <= 0 {
		return rf.scanCandidates(dir)
	}

	now := time.Now()

	rf.cacheMu.Lock()
	if rf.cacheCond == nil {
		rf.cacheCond = sync.NewCond(&rf.cacheMu)
	}

	if rf.cacheReady && rf.cacheDir == dir && now.Before(rf.cacheExpiresAt) {
		candidates := rf.cacheCandidates
		err := rf.cacheErr
		rf.cacheMu.Unlock()
		return candidates, err
	}

	if rf.cacheRefreshing && rf.cacheDir == dir {
		for rf.cacheRefreshing && rf.cacheDir == dir {
			rf.cacheCond.Wait()
		}

		now = time.Now()
		if rf.cacheReady && rf.cacheDir == dir && now.Before(rf.cacheExpiresAt) {
			candidates := rf.cacheCandidates
			err := rf.cacheErr
			rf.cacheMu.Unlock()
			return candidates, err
		}
	}

	rf.cacheRefreshing = true
	rf.cacheDir = dir
	rf.cacheMu.Unlock()

	candidates, err := rf.scanCandidates(dir)
	expiresAt := time.Now().Add(ttl)

	rf.cacheMu.Lock()
	rf.cacheDir = dir
	rf.cacheCandidates = candidates
	rf.cacheErr = err
	rf.cacheExpiresAt = expiresAt
	rf.cacheReady = true
	rf.cacheRefreshing = false
	rf.cacheCond.Broadcast()
	rf.cacheMu.Unlock()

	return candidates, err
}

func (rf *RandomFile) scanCandidates(dir string) ([]string, error) {
	var candidates []string

	if rf.Recursive {
		walkFn := func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// Ignore entries we can't access; keep scanning others.
				return nil
			}
			if d.IsDir() {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return nil
			}
			if !info.Mode().IsRegular() {
				return nil
			}

			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			name := filepath.Base(rel)
			if !rf.matchInclude(rel) && !rf.matchInclude(name) {
				return nil
			}

			candidates = append(candidates, path)
			return nil
		}

		if err := filepath.WalkDir(dir, walkFn); err != nil {
			return nil, err
		}
	} else {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}

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
	}

	if len(candidates) == 0 {
		return nil, errNoMatchingFiles
	}
	return candidates, nil
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
