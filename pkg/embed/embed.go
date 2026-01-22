package embed

import (
	"embed"
	"io/fs"
	"os"
	"runtime"
)

//go:embed etc
var etc embed.FS

func ExtractEmbed(root string) {
	lib, _ := fs.Sub(lib, "linux_"+runtime.GOARCH)
	os.CopyFS(root, lib)
	os.CopyFS(root, etc)
}
