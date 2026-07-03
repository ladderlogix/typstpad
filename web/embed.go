//go:build !noembed

// Package web embeds the built React SPA (vite build output in dist/). The
// `noembed` build tag swaps in a stub (see embed_noembed.go) so the distributed
// CLI binary stays small — the server image builds without the tag.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

func Dist() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
