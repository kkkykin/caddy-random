package randomfile

import (
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


	logger *zap.Logger

	includeLower []string
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
