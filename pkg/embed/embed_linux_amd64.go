package embed

import (
	"embed"
)

//go:embed authenticate_cgi_linux_amd64
var AuthenticateGgi []byte

//go:embed linux_amd64/lib
var lib embed.FS
