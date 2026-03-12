package frontend

import (
	"embed"
	"io/fs"
)

//go:embed index.html app.js style.css
var assets embed.FS

func StaticFS() fs.FS {
	return assets
}
