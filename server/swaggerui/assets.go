package swaggerui

import (
	"embed"
	"io/fs"
)

//go:embed index.html swagger-ui.css swagger-ui-bundle.js swagger-ui-standalone-preset.js
var assets embed.FS

func StaticFS() fs.FS {
	return assets
}
