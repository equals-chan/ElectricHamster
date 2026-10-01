package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// EnvBinary is the environment variable that overrides binary discovery.
const EnvBinary = "EH_SEVENZIP"

// PlatformKey returns the runtime/<os>-<arch> folder name, e.g. "linux-amd64".
func PlatformKey() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

// binaryNames returns candidate executable names for the current OS, in
// preference order. 7-Zip ZS ships 7zz/7z/7za/7zr; on Windows they are .exe.
func binaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"7zz.exe", "7z.exe", "7za.exe", "7zr.exe"}
	}
	return []string{"7zz", "7z", "7za", "7zr"}
}

// ResolveBinary locates a usable 7-Zip executable. Search order:
//
//  1. $EH_SEVENZIP
//  2. embedded runtime asset (build tag `embed_runtime`)
//  3. <exeDir>/runtime/<os>-<arch>/ and <cwd>/runtime/<os>-<arch>/
//  4. <exeDir>/ and <cwd>/
//  5. $PATH
func ResolveBinary() (string, error) {
	if p := os.Getenv(EnvBinary); p != "" {
		if isExecutableFile(p) {
			return p, nil
		}
		return "", fmt.Errorf("%w: %s=%q is not an executable file", ErrToolNotFound, EnvBinary, p)
	}

	if data, name, ok := embeddedBinary(); ok {
		p, err := materialize(data, name)
		if err == nil {
			return p, nil
		}
	}

	key := PlatformKey()
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		dirs = append(dirs, filepath.Join(exeDir, "runtime", key), exeDir)
	}
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(cwd, "runtime", key), cwd)
	}
	for _, dir := range dirs {
		for _, name := range binaryNames() {
			p := filepath.Join(dir, name)
			if isExecutableFile(p) {
				return p, nil
			}
		}
	}

	for _, name := range binaryNames() {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w (platform %s)", ErrToolNotFound, key)
}

// materialize writes an embedded binary into the user cache directory and
// returns its path, reusing an existing identical copy when possible.
func materialize(data []byte, name string) (string, error) {
	sum := sha256.Sum256(data)
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "electric-hamster", "runtime", hex.EncodeToString(sum[:])[:16])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, name)
	if fi, err := os.Stat(dest); err == nil && fi.Size() == int64(len(data)) {
		return dest, nil
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func isExecutableFile(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return fi.Mode().Perm()&0o111 != 0
}
