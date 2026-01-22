package embed

import (
	"embed"
)

//go:embed authenticate_cgi_linux_arm64
var AuthenticateGgi []byte

//go:embed linux_arm64/lib
var lib embed.FS
