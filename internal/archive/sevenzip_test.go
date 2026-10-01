package archive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newTestSevenZip(t *testing.T) *SevenZip {
	t.Helper()
	z, err := NewSevenZip(context.Background())
	if err != nil {
		if errors.Is(err, ErrToolNotFound) {
			t.Skipf("no 7-Zip binary available: %v", err)
		}
		t.Fatalf("NewSevenZip: %v", err)
	}
	return z
}

func writeSampleTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"a.txt":     stringsRepeat("alpha\n", 2000),
		"b.txt":     stringsRepeat("beta\n", 2000),
		"sub/c.txt": stringsRepeat("gamma\n", 2000),
		"中文.txt":    stringsRepeat("汉\n", 500),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return src
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

func TestSevenZipCapabilities(t *testing.T) {
	z := newTestSevenZip(t)
	info, err := z.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	t.Logf("binary=%s version=%s codecs=%v zstd=%v aes=%v",
		info.Path, info.Version, info.Codecs, info.HasZstd, info.HasAES)
	if !info.HasAES {
		t.Error("expected AES support (7zAES)")
	}
	if len(info.Codecs) == 0 {
		t.Error("expected at least one codec")
	}
}

func TestSevenZipRoundTrip(t *testing.T) {
	z := newTestSevenZip(t)
	info, _ := z.Info(context.Background())
	codec := CodecZstd
	if !info.HasZstd {
		codec = CodecLZMA2
	}

	src := writeSampleTree(t)
	root := filepath.Dir(src)
	out := filepath.Join(root, "out.7z")
	const pwd = "test-pass-123"

	opt := DefaultOptions()
	opt.Codec = codec
	opt.Level = 5
	opt.Password = pwd

	var phases []Phase
	err := z.Compress(context.Background(), src, out, opt, func(p Progress) {
		phases = append(phases, p.Phase)
	})
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	fi, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat archive: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatal("archive is empty")
	}
	if len(phases) == 0 || phases[len(phases)-1] != PhaseDone {
		t.Errorf("progress phases = %v, want trailing PhaseDone", phases)
	}

	if err := z.Verify(context.Background(), out, pwd); err != nil {
		t.Fatalf("Verify(correct pwd): %v", err)
	}
	if err := z.Verify(context.Background(), out, "wrong-password"); err == nil {
		t.Error("Verify(wrong pwd) unexpectedly succeeded")
	}

	entries, err := z.List(context.Background(), out, pwd)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var fileCount int
	for _, e := range entries {
		if !e.IsDir {
			fileCount++
		}
	}
	if fileCount != 4 {
		t.Errorf("List file count = %d, want 4 (%+v)", fileCount, entries)
	}
	if _, err := z.List(context.Background(), out, "wrong-password"); err == nil {
		t.Error("List(wrong pwd) unexpectedly succeeded")
	}

	extractDir := filepath.Join(root, "extract")
	if err := z.Extract(context.Background(), out, extractDir, pwd, nil); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(extractDir, "src", "a.txt"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(got) != stringsRepeat("alpha\n", 2000) {
		t.Error("extracted content mismatch")
	}
}

func TestSevenZipUnencrypted(t *testing.T) {
	z := newTestSevenZip(t)
	src := writeSampleTree(t)
	root := filepath.Dir(src)
	out := filepath.Join(root, "plain.7z")

	opt := DefaultOptions()
	opt.Encrypt = false
	opt.Codec = CodecLZMA2
	opt.Level = 1

	if err := z.Compress(context.Background(), src, out, opt, nil); err != nil {
		t.Fatalf("Compress: %v", err)
	}
	if err := z.Verify(context.Background(), out, ""); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if _, err := z.List(context.Background(), out, ""); err != nil {
		t.Fatalf("List: %v", err)
	}
}

func TestSevenZipCancellation(t *testing.T) {
	z := newTestSevenZip(t)
	src := writeSampleTree(t)
	root := filepath.Dir(src)
	out := filepath.Join(root, "cancel.7z")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opt := DefaultOptions()
	opt.Password = "x"

	if err := z.Compress(ctx, src, out, opt, nil); err == nil {
		t.Fatal("expected cancellation error")
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("expected no output archive after cancellation")
	}
}
