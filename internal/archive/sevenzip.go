package archive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SevenZip is an Archiver backed by a 7-Zip ZS (or compatible) executable.
type SevenZip struct {
	bin  string
	info ToolInfo
}

var _ Archiver = (*SevenZip)(nil)

// NewSevenZip resolves a 7-Zip binary and probes its capabilities.
func NewSevenZip(ctx context.Context) (*SevenZip, error) {
	bin, err := ResolveBinary()
	if err != nil {
		return nil, err
	}
	z := &SevenZip{bin: bin}
	info, err := z.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("archive: probe %s: %w", bin, err)
	}
	z.info = info
	return z, nil
}

// Binary returns the resolved executable path.
func (z *SevenZip) Binary() string { return z.bin }

// Info returns cached capability information.
func (z *SevenZip) Info(ctx context.Context) (ToolInfo, error) {
	out, _, err := z.capture(ctx, []string{"i", "-sccUTF-8"})
	if err != nil {
		return ToolInfo{}, err
	}
	info := ToolInfo{Path: z.bin, Version: parseVersion(out)}
	add := func(name string) {
		for _, c := range info.Codecs {
			if c == name {
				return
			}
		}
		info.Codecs = append(info.Codecs, name)
	}
	if strings.Contains(out, "ZSTD") {
		info.HasZstd = true
		add("zstd")
	}
	if strings.Contains(out, "7zAES") {
		info.HasAES = true
		add("7zAES")
	}
	for _, c := range []string{"LZMA2", "BROTLI", "BZip2"} {
		if strings.Contains(out, c) {
			add(strings.ToLower(c))
		}
	}
	return info, nil
}

var reVersion = regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)`)

func parseVersion(out string) string {
	// The banner is the first line mentioning 7-Zip, e.g.:
	// "7-Zip (z) 26.02 ZS v1.5.7 R1 (x64) : Copyright ..."
	// (7-Zip prefixes its output with an empty line.)
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "7-Zip") {
			continue
		}
		if m := reVersion.FindStringSubmatch(line); m != nil {
			return m[1]
		}
		return strings.TrimSpace(line)
	}
	return ""
}

// Compress archives srcDir into outFile. Compression happens against a
// temporary file and is renamed into place only on success.
func (z *SevenZip) Compress(ctx context.Context, srcDir, outFile string, opt Options, onProgress ProgressFunc) error {
	opt, err := opt.normalized()
	if err != nil {
		return err
	}
	st, err := os.Stat(srcDir)
	if err != nil {
		return fmt.Errorf("archive: source: %w", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("archive: source %q is not a directory", srcDir)
	}
	if opt.Encrypt && opt.Password == "" {
		return ErrPasswordRequired
	}
	absOut, err := filepath.Abs(outFile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absOut), 0o755); err != nil {
		return err
	}
	absSrc, err := filepath.Abs(srcDir)
	if err != nil {
		return err
	}

	tmp := absOut + ".tmp"
	z.removeArtifacts(tmp)

	args := []string{"a", "-t" + string(opt.Format), "-m0=" + string(opt.Codec)}
	args = append(args, "-mx="+strconv.Itoa(opt.Level))
	if opt.Encrypt {
		args = append(args, "-p"+opt.Password)
		if opt.HeaderEncrypt {
			args = append(args, "-mhe=on")
		} else {
			args = append(args, "-mhe=off")
		}
	}
	if opt.SplitSize > 0 {
		args = append(args, "-v"+strconv.FormatInt(opt.SplitSize, 10)+"b")
	}
	if opt.Threads > 0 {
		args = append(args, "-mmt="+strconv.Itoa(opt.Threads))
	}
	args = append(args, "-bsp1", "-bso0", "-sccUTF-8", "-y")
	args = append(args, opt.ExtraArgs...)
	args = append(args, "--", tmp, filepath.Base(absSrc))

	if err := z.stream(ctx, args, filepath.Dir(absSrc), tmp, onProgress); err != nil {
		z.removeArtifacts(tmp)
		return err
	}
	if err := finalize(tmp, absOut, opt.SplitSize > 0); err != nil {
		z.removeArtifacts(tmp)
		return err
	}
	if opt.Verify {
		ref := absOut
		if opt.SplitSize > 0 {
			ref = absOut + ".001"
		}
		if err := z.Verify(ctx, ref, opt.Password); err != nil {
			return fmt.Errorf("archive: verify: %w", err)
		}
	}
	if onProgress != nil {
		onProgress(Progress{Phase: PhaseDone, Percent: 100})
	}
	return nil
}

// Extract unpacks archivePath into outDir.
func (z *SevenZip) Extract(ctx context.Context, archivePath, outDir, password string, onProgress ProgressFunc) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	args := []string{"x", "-o" + outDir, "-y", "-bsp1", "-bso0", "-sccUTF-8"}
	if password != "" {
		args = append(args, "-p"+password)
	}
	args = append(args, "--", archivePath)
	return z.stream(ctx, args, "", "", onProgress)
}

// List returns the entries of an archive using 7-Zip's -slt output.
func (z *SevenZip) List(ctx context.Context, archivePath, password string) ([]Entry, error) {
	args := []string{"l", "-slt", "-sccUTF-8"}
	if password != "" {
		args = append(args, "-p"+password)
	}
	args = append(args, "--", archivePath)
	out, _, err := z.capture(ctx, args)
	if err != nil {
		return nil, err
	}
	return parseSLT(out), nil
}

// Verify performs an integrity test and fails if the archive is corrupt or the
// password is wrong.
func (z *SevenZip) Verify(ctx context.Context, archivePath, password string) error {
	args := []string{"t", "-sccUTF-8", "-bso0"}
	if password != "" {
		args = append(args, "-p"+password)
	}
	args = append(args, "--", archivePath)
	_, stderr, err := z.capture(ctx, args)
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr))
		}
		return err
	}
	return nil
}

// --- process helpers ---

func (z *SevenZip) capture(ctx context.Context, args []string) (string, string, error) {
	cmd := exec.CommandContext(ctx, z.bin, args...)
	configureProcess(cmd)
	cmd.WaitDelay = 10 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), stderr.String(), wrapExit(ctx, err)
	}
	return stdout.String(), stderr.String(), nil
}

func (z *SevenZip) stream(ctx context.Context, args []string, workDir, statPath string, onProgress ProgressFunc) error {
	cmd := exec.CommandContext(ctx, z.bin, args...)
	configureProcess(cmd)
	cmd.WaitDelay = 10 * time.Second
	if workDir != "" {
		cmd.Dir = workDir
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	start := time.Now()
	var lastPct float64 = -1
	var lastCur string
	handle := func(line string) {
		if onProgress == nil {
			return
		}
		pr, ok := parseProgressLine(line)
		if !ok {
			return
		}
		if pr.Percent == lastPct && pr.Current == lastCur && pr.Phase == PhaseCompress {
			return
		}
		lastPct, lastCur = pr.Percent, pr.Current
		pr.Elapsed = time.Since(start)
		if statPath != "" {
			if fi, err := os.Stat(statPath); err == nil {
				pr.BytesOut = fi.Size()
			}
		}
		onProgress(pr)
	}

	readErr := newProgressParser(handle).consume(stdout)
	waitErr := cmd.Wait()
	if waitErr != nil {
		return wrapExit(ctx, waitErr)
	}
	if readErr != nil && ctx.Err() == nil {
		return readErr
	}
	return nil
}

// ExitError reports a non-zero 7-Zip exit status.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("archive: 7-Zip exited with code %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

func wrapExit(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return &ExitError{Code: ee.ExitCode(), Err: err}
	}
	return err
}

// finalize moves the temporary archive (and any split volumes) into place.
func finalize(tmp, out string, split bool) error {
	if !split {
		return os.Rename(tmp, out)
	}
	matches, _ := filepath.Glob(tmp + ".*")
	if len(matches) == 0 {
		return os.Rename(tmp, out)
	}
	for _, m := range matches {
		suffix := strings.TrimPrefix(m, tmp)
		if err := os.Rename(m, out+suffix); err != nil {
			return err
		}
	}
	return nil
}

// removeArtifacts deletes a temporary archive and any split volumes.
func (z *SevenZip) removeArtifacts(tmp string) {
	os.Remove(tmp)
	if matches, _ := filepath.Glob(tmp + ".*"); len(matches) > 0 {
		for _, m := range matches {
			os.Remove(m)
		}
	}
}

// --- -slt parsing ---

var reSLTLine = regexp.MustCompile(`^\s*([^=]+?)\s*=\s*(.*)$`)

func parseSLT(out string) []Entry {
	var entries []Entry
	var cur *Entry
	started := false
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if !started {
			if strings.HasPrefix(strings.TrimSpace(line), "----------") {
				started = true
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			if cur != nil {
				entries = append(entries, *cur)
				cur = nil
			}
			continue
		}
		m := reSLTLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key, val := m[1], m[2]
		if cur == nil {
			cur = &Entry{}
		}
		switch key {
		case "Path":
			cur.Path = val
		case "Size":
			cur.Size, _ = strconv.ParseInt(val, 10, 64)
		case "Packed Size":
			cur.Packed, _ = strconv.ParseInt(val, 10, 64)
		case "Modified":
			cur.Modified = parseSLTTime(val)
		case "Attributes":
			cur.IsDir = strings.HasPrefix(strings.TrimSpace(val), "D")
		case "Encrypted":
			cur.Encrypted = val == "+"
		case "Method":
			cur.Method = val
		}
	}
	if cur != nil {
		entries = append(entries, *cur)
	}
	return entries
}

func parseSLTTime(v string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t
		}
	}
	return time.Time{}
}
