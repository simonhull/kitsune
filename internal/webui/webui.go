// Package webui holds the built web client, so one binary is all there is to
// run. scripts/build.sh fills dist before compiling; in git it is empty, and
// a plain go build gives a server with the API alone.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Files returns the web client, or nil when this binary was built without one.
func Files() fs.FS { return client(dist) }

// client returns root's dist folder if it holds a built client, else nil.
func client(root fs.FS) fs.FS {
	files, err := fs.Sub(root, "dist")
	if err != nil {
		return nil
	}
	if _, err = fs.Stat(files, "index.html"); err != nil {
		return nil
	}
	return files
}
