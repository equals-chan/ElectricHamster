package archive

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Format is the container format produced by the engine.
type Format string

const (
	Format7z Format = "7z"
)

// Codec is the compression method used inside the container.
type Codec string

const (
	CodecZstd  Codec = "zstd"
	CodecLZMA2 Codec = "lzma2"
	CodecCopy  Codec = "copy"
)

// Phase describes what the engine is currently doing.
type Phase string

const (
	PhaseScan     Phase = "scan"
	PhaseCompress Phase = "compress"
	PhaseHeader   Phase = "header"
	PhaseExtract  Phase = "extract"
	PhaseDone     Phase = "done"
)

// Options controls a single compression job.
type Options struct {
	Format        Format
	Codec         Codec
	Level         int
	Encrypt       bool
	HeaderEncrypt bool
	SplitSize     int64 // bytes; 0 = single volume
	Password      string
	Threads       int  // -mmt; 0 = 7-Zip default (all cores)
	Verify        bool // run an integrity test after compressing
	ExtraArgs     []string
}

// DefaultOptions returns the project defaults: 7z container, zstd level 15,
// AES-256 with encrypted headers.
func DefaultOptions() Options {
	return Options{
		Format:        Format7z,
		Codec:         CodecZstd,
		Level:         15,
		Encrypt:       true,
		HeaderEncrypt: true,
		Verify:        true,
	}
}

func (o Options) normalized() (Options, error) {
	if o.Format == "" {
		o.Format = Format7z
	}
	if o.Format != Format7z {
		return o, fmt.Errorf("archive: unsupported format %q", o.Format)
	}
	if o.Codec == "" {
		o.Codec = CodecZstd
	}
	switch o.Codec {
	case CodecZstd:
		o.Level = clamp(o.Level, 0, 22)
	case CodecLZMA2:
		o.Level = clamp(o.Level, 0, 9)
	case CodecCopy:
		o.Level = 0
	default:
		return o, fmt.Errorf("archive: unsupported codec %q", o.Codec)
	}
	return o, nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Entry is a single item inside an archive.
type Entry struct {
	Path      string
	Size      int64
	Packed    int64
	Modified  time.Time
	IsDir     bool
	Encrypted bool
	Method    string
}

// Progress is reported while an engine operation runs.
type Progress struct {
	Phase    Phase
	Percent  float64
	Current  string
	BytesIn  int64
	BytesOut int64
	Speed    int64 // bytes/sec, estimated
	Elapsed  time.Duration
}

// ProgressFunc receives progress updates. Implementations must be safe to call
// from a single reader goroutine; the engine never calls it concurrently.
type ProgressFunc func(Progress)

// ToolInfo describes the discovered 7-Zip binary.
type ToolInfo struct {
	Path    string
	Version string
	Codecs  []string
	HasZstd bool
	HasAES  bool
}

// Archiver is the engine abstraction. Implementations must be safe for
// concurrent use.
type Archiver interface {
	Compress(ctx context.Context, srcDir, outFile string, opt Options, onProgress ProgressFunc) error
	Extract(ctx context.Context, archivePath, outDir, password string, onProgress ProgressFunc) error
	List(ctx context.Context, archivePath, password string) ([]Entry, error)
	Verify(ctx context.Context, archivePath, password string) error
	Info(ctx context.Context) (ToolInfo, error)
}

var (
	// ErrToolNotFound is returned when no usable 7-Zip binary can be located.
	ErrToolNotFound = errors.New("archive: 7-Zip binary not found")
	// ErrPasswordRequired is returned when encryption is requested without a password.
	ErrPasswordRequired = errors.New("archive: encryption requested but no password supplied")
)
