package randomfile

import (
	"fmt"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var rf RandomFile

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
			case "exclude":
				args := h.RemainingArgs()
				if len(args) == 0 {
					return nil, h.ArgErr()
				}
				rf.Exclude = append(rf.Exclude, args...)
			case "exclude_dir":
				args := h.RemainingArgs()
				if len(args) == 0 {
					return nil, h.ArgErr()
				}
				rf.ExcludeDir = append(rf.ExcludeDir, args...)
			case "recursive":
				rf.Recursive = true
				if h.NextArg() {
					return nil, h.ArgErr()
				}
			case "cache":
				if !h.NextArg() {
					return nil, h.ArgErr()
				}
				dur, err := caddy.ParseDuration(h.Val())
				if err != nil {
					return nil, err
				}
				rf.Cache = caddy.Duration(dur)
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
