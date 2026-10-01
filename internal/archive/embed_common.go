package archive

import (
	"embed"
	"io/fs"
	"path"
	"strings"
)

// readEmbeddedBinary picks the first recognized executable from dir inside fsys.
// It is shared by the per-platform embed files.
func readEmbeddedBinary(fsys embed.FS, dir string) ([]byte, string, bool) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, "", false
	}
	for _, want := range binaryNames() {
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(e.Name(), want) {
				continue
			}
			data, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
			if err == nil {
				return data, e.Name(), true
			}
		}
	}
	return nil, "", false
}
