//go:build noembed

// Slim build: no bundled SPA (used for the distributed CLI binary). The server
// UI is served from the full build shipped in the Docker image.
package web

import (
	"embed"
	"io/fs"
)

//go:embed stub
var stubFS embed.FS

func Dist() fs.FS {
	sub, err := fs.Sub(stubFS, "stub")
	if err != nil {
		panic(err)
	}
	return sub
}
