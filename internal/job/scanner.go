// Package job implements the archiving pipeline: scan a source directory,
// detect already-archived folders, compress the rest, and record the results.
package job

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Unit is one archivable folder.
type Unit struct {
	Name string
	Path string
}

// Scan returns the immediate subdirectories of root, sorted by name. depth is
// reserved for future multi-level scanning; only depth <= 1 is implemented and
// matches the original tool's behaviour.
func Scan(root string, depth int) ([]Unit, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("job: source %q is not a directory", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var units []Unit
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		units = append(units, Unit{Name: e.Name(), Path: filepath.Join(root, e.Name())})
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Name < units[j].Name })
	return units, nil
}

// MatchFilters reports whether name passes the include/exclude glob lists.
// Excludes win; an empty include list means "include everything".
func MatchFilters(name string, include, exclude []string) bool {
	for _, g := range exclude {
		if ok, _ := filepath.Match(g, name); ok {
			return false
		}
	}
	if len(include) == 0 {
		return true
	}
	for _, g := range include {
		if ok, _ := filepath.Match(g, name); ok {
			return true
		}
	}
	return false
}

// TreeInfo summarises a folder's contents.
type TreeInfo struct {
	Signature string
	Bytes     int64
	Files     int
}

// ScanTree walks root and computes a signature over the sorted list of
// (relative path, size, mtime). Renames, edits or additions change the
// signature, which is used for de-duplication instead of the folder name.
func ScanTree(root string) (TreeInfo, error) {
	type entry struct {
		rel   string
		size  int64
		mtime int64
	}
	var entries []entry
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entries = append(entries, entry{filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano()})
		return nil
	})
	if err != nil {
		return TreeInfo{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })

	h := sha256.New()
	var total int64
	for _, e := range entries {
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", e.rel, e.size, e.mtime)
		total += e.size
	}
	sum := h.Sum(nil)
	return TreeInfo{
		Signature: hex.EncodeToString(sum[:16]),
		Bytes:     total,
		Files:     len(entries),
	}, nil
}
