//go:build embed_runtime && !windows && !linux

package archive

// embeddedBinary reports that no bundled binary is available on this platform.
func embeddedBinary() ([]byte, string, bool) {
	return nil, "", false
}
