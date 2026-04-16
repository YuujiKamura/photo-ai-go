package web

import "embed"

//go:embed *.html *.json
var StaticAssets embed.FS
