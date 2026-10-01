package store

import (
	"strconv"
	"time"
)

// PasswordPolicy describes how a task's archive passwords are obtained.
type PasswordPolicy struct {
	Mode    string `json:"mode"` // "random" | "fixed" | "none"
	Length  int    `json:"length,omitempty"`
	Charset string `json:"charset,omitempty"`
	Fixed   string `json:"fixed,omitempty"`
}

// Task is a reusable archiving configuration.
type Task struct {
	ID            string
	Name          string
	SourceDir     string
	DestDir       string
	IncludeGlobs  []string
	ExcludeGlobs  []string
	Format        string
	Codec         string
	Level         int
	Encrypt       bool
	HeaderEncrypt bool
	SplitSize     int64
	Password      PasswordPolicy
	DedupRule     string
	NamePrefix    string
	NameSuffix    string
	SeqStart      int64
	Enabled       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Run status values.
const (
	RunPending     = "pending"
	RunRunning     = "running"
	RunPaused      = "paused"
	RunDone        = "done"
	RunFailed      = "failed"
	RunCanceled    = "canceled"
	RunInterrupted = "interrupted"
)

// Run records a single execution of a task.
type Run struct {
	ID         string
	TaskID     string
	Status     string
	Total      int
	Done       int
	BytesIn    int64
	BytesOut   int64
	StartedAt  time.Time
	FinishedAt time.Time
	Error      string
}

// Archive status values.
const (
	ArchiveOK      = "ok"
	ArchiveCorrupt = "corrupt"
	ArchiveMissing = "missing"
)

// SeqCounterName returns the per-task counter key used for archive numbering.
func SeqCounterName(taskID string) string { return "task:" + taskID }

// ArchiveFileName builds the output file name for a sequence number using the
// task's prefix/suffix, e.g. prefix "game_" + seq 10000 + suffix "_a" + ".7z".
func (t Task) ArchiveFileName(seq int64, format string) string {
	if format == "" {
		format = t.Format
	}
	if format == "" {
		format = "7z"
	}
	return t.NamePrefix + strconv.FormatInt(seq, 10) + t.NameSuffix + "." + format
}

// Archive is a produced archive file recorded in the ledger.
type Archive struct {
	ID          string
	RunID       string
	TaskID      string
	Seq         int64
	FolderName  string
	SourcePath  string
	ArchivePath string
	ArchiveSize int64
	FileCount   int
	ContentSig  string
	PasswordID  string
	Status      string
	CreatedAt   time.Time
}
