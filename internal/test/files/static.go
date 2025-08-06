package files

import "embed"

//go:embed *.jpeg
var Static embed.FS
