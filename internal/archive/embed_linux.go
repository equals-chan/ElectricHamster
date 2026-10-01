//go:build embed_runtime && linux

package archive

import "embed"

//go:embed runtime/linux-amd64
var runtimeFS embed.FS

// embeddedBinary returns the bundled 7-Zip executable for Linux.
func embeddedBinary() ([]byte, string, bool) {
	return readEmbeddedBinary(runtimeFS, "runtime/linux-amd64")
}
