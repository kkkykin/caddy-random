package randomfile

import (
	"fmt"
	"strings"

	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var rf RandomFile

	// Defaults.
	rf.SubdirParam = "subdir"

	for h.Next() {
		for h.NextBlock(0) {
			switch h.Val() {
			case "root":
				if !h.NextArg() {
					return nil, h.ArgErr()
				}
				rf.Root = h.Val()
				if h.NextArg() {
					return nil, h.ArgErr()
				}
			case "include":
				args := h.RemainingArgs()
				if len(args) == 0 {
					return nil, h.ArgErr()
				}
				rf.Include = append(rf.Include, args...)
			case "subdir_param":
				if !h.NextArg() {
					return nil, h.ArgErr()
				}
				rf.SubdirParam = h.Val()
				if h.NextArg() {
					return nil, h.ArgErr()
				}
			case "use_url_path_subdir":
				rf.UseURLPathSubdir = true
				if h.NextArg() {
					return nil, h.ArgErr()
				}
			default:
				return nil, fmt.Errorf("unknown subdirective %q", h.Val())
			}
		}
	}

	if strings.TrimSpace(rf.Root) == "" {
		return nil, fmt.Errorf("root is required")
	}

	return &rf, nil
}
