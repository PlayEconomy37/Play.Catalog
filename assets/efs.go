package assets

import (
	"embed"
)

//go:embed "cert"
var EmbeddedFiles embed.FS
