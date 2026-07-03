//go:build withui

// Package web embeds the built React SPA (vite build output in dist/). This is
// opt-in via the `withui` build tag, used only by the Docker server image. The
// default build (see embed_stub.go) ships no SPA, so the CLI stays small and
// `go install` works without a web build / committed dist/.
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
