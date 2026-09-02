package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web
var webFS embed.FS

// webRoot is the embedded interface, rooted so that "/" serves index.html.
var webRoot = func() fs.FS {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		// The files are embedded at build time, so this can only fail if the
		// build is broken.
		panic(err)
	}
	return sub
}()

// handleWeb serves the single page interface.
//
// It is registered on "/" and therefore also receives every unmatched path, so
// anything that is not a file in the bundle has to become a JSON 404 rather
// than the file server's plain text one.
func (s *Server) handleWeb(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		if _, err := fs.Stat(webRoot, r.URL.Path[1:]); err != nil {
			writeJSON(w, http.StatusNotFound, errorBody{Error: "not found"})
			return
		}
	}
	http.FileServerFS(webRoot).ServeHTTP(w, r)
}
