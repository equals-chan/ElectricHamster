//go:build embed_runtime && windows

package archive

import "embed"

//go:embed runtime/windows-amd64
var runtimeFS embed.FS

// embeddedBinary returns the bundled 7-Zip executable for Windows.
func embeddedBinary() ([]byte, string, bool) {
	return readEmbeddedBinary(runtimeFS, "runtime/windows-amd64")
}
