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

	"github.com/bmatcuk/doublestar/v4"
	"go.uber.org/zap"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func (rf *RandomFile) Provision(ctx caddy.Context) error {
	rf.logger = ctx.Logger(rf)

	rf.initIncludeLower()
	rf.initExcludeLower()

	rf.cacheMu.Lock()
	if rf.cacheCond == nil {
		rf.cacheCond = sync.NewCond(&rf.cacheMu)
	}
	// Reset cache state in case this module instance is re-provisioned.
	rf.cacheKey = ""
	rf.cacheExpiresAt = time.Time{}
	rf.cacheCandidates = nil
	rf.cacheErr = nil
	rf.cacheReady = false
	rf.cacheRefreshing = false
	rf.cacheMu.Unlock()

	return nil
}

func (rf *RandomFile) initIncludeLower() {
	rf.includeLower = normalizePatterns(rf.Include)
}

func (rf *RandomFile) initExcludeLower() {
	combined := make([]string, 0, len(rf.Exclude)+len(rf.ExcludeDir))
	combined = append(combined, rf.Exclude...)
	combined = append(combined, rf.ExcludeDir...)
	rf.excludeLower = normalizePatterns(combined)
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

const debugCandidateSampleLimit = 50

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
	selected := candidates[idx]
	if rf.logger != nil {
		rf.logger.Debug(
			"random_file selected",
			zap.String("dir", dir),
			zap.String("file", selected),
			zap.Int("candidates_total", len(candidates)),
		)
	}
	return selected, nil
}

func (rf *RandomFile) getCandidates(dir string) ([]string, error) {
	// Provision() sets includeLower and cacheCond, but callers might invoke methods
	// directly in tests.
	if rf.cacheCond == nil {
		rf.cacheMu.Lock()
		if rf.cacheCond == nil {
			rf.cacheCond = sync.NewCond(&rf.cacheMu)
		}
		rf.cacheMu.Unlock()
	}
	ttl := time.Duration(rf.Cache)
	if ttl <= 0 {
		return rf.scanCandidates(dir)
	}

	key := rf.candidateCacheKey(dir)
	now := time.Now()

	rf.cacheMu.Lock()
	if rf.cacheReady && rf.cacheKey == key && now.Before(rf.cacheExpiresAt) {
		candidates := rf.cacheCandidates
		err := rf.cacheErr
		rf.cacheMu.Unlock()
		return candidates, err
	}

	if rf.cacheRefreshing && rf.cacheKey == key {
		for rf.cacheRefreshing && rf.cacheKey == key {
			rf.cacheCond.Wait()
		}

		now = time.Now()
		if rf.cacheReady && rf.cacheKey == key && now.Before(rf.cacheExpiresAt) {
			candidates := rf.cacheCandidates
			err := rf.cacheErr
			rf.cacheMu.Unlock()
			return candidates, err
		}
	}

	rf.cacheRefreshing = true
	rf.cacheKey = key
	rf.cacheMu.Unlock()

	candidates, err := rf.scanCandidates(dir)
	expiresAt := time.Now().Add(ttl)

	rf.cacheMu.Lock()
	rf.cacheKey = key
	rf.cacheCandidates = candidates
	rf.cacheErr = err
	rf.cacheExpiresAt = expiresAt
	rf.cacheReady = true
	rf.cacheRefreshing = false
	rf.cacheCond.Broadcast()
	rf.cacheMu.Unlock()

	return candidates, err
}

func (rf *RandomFile) candidateCacheKey(dir string) string {
	inc := strings.Join(rf.includePatterns(), "\x00")
	exc := strings.Join(rf.excludePatterns(), "\x00")
	return fmt.Sprintf("dir=%s\x1frecursive=%t\x1finclude=%s\x1fexclude=%s", dir, rf.shouldRecurse(), inc, exc)
}

func (rf *RandomFile) scanCandidates(dir string) ([]string, error) {
	var candidates []string

	if rf.shouldRecurse() {
		walkFn := func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// Ignore entries we can't access; keep scanning others.
				return nil
			}
			if d.IsDir() {
				if path == dir {
					return nil
				}
				rel, err := filepath.Rel(dir, path)
				if err != nil {
					return nil
				}
				rel = filepath.ToSlash(rel)
				name := filepath.Base(rel)
				if rf.matchExcludeDir(rel, name) {
					return fs.SkipDir
				}
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
			if rf.matchExclude(rel) || rf.matchExclude(name) {
				return nil
			}
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
			if rf.matchExclude(rel) {
				continue
			}
			if !rf.matchInclude(rel) {
				continue
			}
			candidates = append(candidates, filepath.Join(dir, name))
		}
	}

	if len(candidates) == 0 {
		if rf.logger != nil {
			rf.logger.Debug(
				"random_file candidates",
				zap.String("dir", dir),
				zap.Int("candidates_total", 0),
				zap.Int("candidates_sample_size", 0),
				zap.Bool("candidates_truncated", false),
				zap.Strings("candidates_sample", nil),
			)
		}
		return nil, errNoMatchingFiles
	}

	if rf.logger != nil {
		sample := candidates
		truncated := false
		if len(sample) > debugCandidateSampleLimit {
			sample = sample[:debugCandidateSampleLimit]
			truncated = true
		}
		rf.logger.Debug(
			"random_file candidates",
			zap.String("dir", dir),
			zap.Int("candidates_total", len(candidates)),
			zap.Int("candidates_sample_size", len(sample)),
			zap.Bool("candidates_truncated", truncated),
			zap.Strings("candidates_sample", sample),
		)
	}
	return candidates, nil
}

func (rf *RandomFile) matchInclude(relPath string) bool {
	patterns := rf.includePatterns()
	if len(patterns) == 0 {
		return true
	}
	return matchAnyPattern(patterns, relPath)
}

func (rf *RandomFile) matchExclude(relPath string) bool {
	patterns := rf.excludePatterns()
	if len(patterns) == 0 {
		return false
	}
	return matchAnyPattern(patterns, relPath)
}

func (rf *RandomFile) includePatterns() []string {
	if len(rf.Include) == 0 {
		return nil
	}
	if len(rf.includeLower) != 0 {
		return rf.includeLower
	}
	return normalizePatterns(rf.Include)
}

func (rf *RandomFile) excludePatterns() []string {
	if len(rf.Exclude) == 0 && len(rf.ExcludeDir) == 0 {
		return nil
	}
	if len(rf.excludeLower) != 0 {
		return rf.excludeLower
	}
	combined := make([]string, 0, len(rf.Exclude)+len(rf.ExcludeDir))
	combined = append(combined, rf.Exclude...)
	combined = append(combined, rf.ExcludeDir...)
	return normalizePatterns(combined)
}

func (rf *RandomFile) matchExcludeDir(rel, name string) bool {
	patterns := rf.excludePatterns()
	if len(patterns) == 0 {
		return false
	}
	if matchAnyPattern(patterns, rel) || matchAnyPattern(patterns, name) {
		return true
	}
	// Match patterns that target descendants (e.g. **/dir/**) by testing
	// a synthetic child path.
	return matchAnyPattern(patterns, filepath.ToSlash(rel)+"/__dir__")
}

func (rf *RandomFile) shouldRecurse() bool {
	if rf.Recursive {
		return true
	}
	for _, pattern := range rf.includePatterns() {
		if patternRequiresRecursive(pattern) {
			return true
		}
	}
	for _, pattern := range rf.excludePatterns() {
		if patternRequiresRecursive(pattern) {
			return true
		}
	}
	return false
}

func patternRequiresRecursive(pattern string) bool {
	return strings.Contains(pattern, "/") || strings.Contains(pattern, "**")
}

func matchAnyPattern(patterns []string, path string) bool {
	name := strings.ToLower(path)
	name = filepath.ToSlash(name)
	for _, pattern := range patterns {
		ok, err := doublestar.Match(pattern, name)
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

func normalizePatterns(patterns []string) []string {
	if len(patterns) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(patterns))
	for _, p := range patterns {
		normalized = append(normalized, strings.ToLower(filepath.ToSlash(p)))
	}
	return normalized
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
