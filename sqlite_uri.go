package main

import (
	"net/url"
	"path/filepath"
)

func sqliteFileURL(path string) url.URL {
	path = filepath.ToSlash(path)
	// SQLite requires /C:/... so the drive isn't parsed as a URI authority.
	if len(path) >= 3 && path[1] == ':' && path[2] == '/' &&
		(path[0] >= 'A' && path[0] <= 'Z' || path[0] >= 'a' && path[0] <= 'z') {
		path = "/" + path
	}
	return url.URL{Scheme: "file", Path: path}
}
