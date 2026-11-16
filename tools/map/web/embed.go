// Package web holds the map's page: plain HTML, CSS and JS modules, embedded in the binary so
// the map needs nothing from the internet.
package web

import "embed"

//go:embed *.html *.css js
var Files embed.FS
