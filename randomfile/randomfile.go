package randomfile

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	caddy.RegisterModule(RandomFile{})
	httpcaddyfile.RegisterHandlerDirective("random_file", parseCaddyfile)
}

// RandomFile is an HTTP handler that responds by choosing a random file from a
// directory and serving it.
//
// It is similar to Caddy's file_server, but instead of serving a specific file
// based on the request path, it selects a random file from a target directory.
type RandomFile struct {
	// Root is the base directory under which files are selected.
	Root string `json:"root,omitempty"`

	// Include is a list of glob patterns. If empty, all regular files in the
	// target directory are eligible.
	//
	// Matching is case-insensitive.
	Include []string `json:"include,omitempty"`

	// Recursive enables scanning subdirectories under Root.
	//
	// Default is false to preserve existing behavior.
	Recursive bool `json:"recursive,omitempty"`

	// Cache is an optional cache TTL for the candidate file list. When set, the
	// directory scan is performed at most once per TTL.
	//
	// Default is 0 (disabled).
	Cache caddy.Duration `json:"cache,omitempty"`

	logger *zap.Logger

	includeLower []string

	cacheMu   sync.Mutex
	cacheCond *sync.Cond

	cacheKey        string
	cacheExpiresAt  time.Time
	cacheCandidates []string
	cacheErr        error
	cacheReady      bool
	cacheRefreshing bool
}

func (RandomFile) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.random_file",
		New: func() caddy.Module { return new(RandomFile) },
	}
}

var (
	_ caddy.Provisioner           = (*RandomFile)(nil)
	_ caddy.Validator             = (*RandomFile)(nil)
	_ caddyhttp.MiddlewareHandler = (*RandomFile)(nil)
)
