// Package legacy imports data from the original Java ElectricHamster tool:
// a config.properties plus the SQLite database it produced.
package legacy

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config mirrors the fields of the old config.properties.
type Config struct {
	FolderPath      string
	ZippedFilesPath string
	ExcelPath       string
	SqlPath         string
	ExcelOpNum      string
	IfRandomPwd     string
	UseThisPwd      string
}

// ParseConfigProperties reads a legacy config.properties file.
func ParseConfigProperties(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	kv := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		kv[key] = val
	}
	if err := sc.Err(); err != nil {
		return Config{}, err
	}
	return Config{
		FolderPath:      kv["folderPath"],
		ZippedFilesPath: kv["zippedFilesPath"],
		ExcelPath:       kv["excelPath"],
		SqlPath:         kv["sqlPath"],
		ExcelOpNum:      kv["excelOpNum"],
		IfRandomPwd:     kv["ifRandomPwd"],
		UseThisPwd:      kv["useThisPwd"],
	}, nil
}

// PasswordMode maps the legacy flags to a modern password policy mode.
func (c Config) PasswordMode() string {
	switch c.IfRandomPwd {
	case "0":
		return "random"
	case "1":
		return "fixed"
	default:
		return "random"
	}
}

// BaseDir returns the directory that relative paths in the config are
// resolved against.
func (c Config) BaseDir(configPath string) string {
	if c.SqlPath != "" {
		return filepath.Clean(c.SqlPath)
	}
	return filepath.Dir(configPath)
}

// Validate reports missing required fields.
func (c Config) Validate() error {
	if c.FolderPath == "" {
		return fmt.Errorf("legacy: config has no folderPath")
	}
	if c.ZippedFilesPath == "" {
		return fmt.Errorf("legacy: config has no zippedFilesPath")
	}
	return nil
}
