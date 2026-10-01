//go:build !embed_runtime

package archive

// embeddedBinary is the no-op variant used by default builds. Build with
// `-tags embed_runtime` and a populated internal/archive/runtime/<os>-<arch>/
// folder to embed the 7-Zip ZS binary into the executable.
func embeddedBinary() ([]byte, string, bool) {
	return nil, "", false
}
