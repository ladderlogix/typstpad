//go:build !withui

// Default build: no bundled SPA (keeps the CLI small and makes `go install`
// work without a committed dist/). The full server UI is embedded only when
// building with `-tags withui` (the Docker image); see embed.go.
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
