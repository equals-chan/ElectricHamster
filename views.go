package main

// DTOs shared with the frontend via generated bindings.

// VaultStatus describes the encrypted vault state.
type VaultStatus struct {
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	Unlocked bool   `json:"unlocked"`
}

// ItemView is a vault entry without its secret.
type ItemView struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	CreatedAt string `json:"createdAt"`
}

// TaskInput is the editable form for a task.
type TaskInput struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	SourceDir      string `json:"sourceDir"`
	DestDir        string `json:"destDir"`
	Codec          string `json:"codec"`
	Level          int    `json:"level"`
	Encrypt        bool   `json:"encrypt"`
	HeaderEncrypt  bool   `json:"headerEncrypt"`
	SplitSize      int64  `json:"splitSize"`
	PasswordMode   string `json:"passwordMode"`
	PasswordLength int    `json:"passwordLength"`
	PasswordFixed  string `json:"passwordFixed"`
	DedupRule      string `json:"dedupRule"`
	NamePrefix     string `json:"namePrefix"`
	NameSuffix     string `json:"nameSuffix"`
	SeqStart       int64  `json:"seqStart"`
	Include        string `json:"include"`
	Exclude        string `json:"exclude"`
}

// TaskView is a stored task plus derived statistics.
type TaskView struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	SourceDir     string `json:"sourceDir"`
	DestDir       string `json:"destDir"`
	Codec         string `json:"codec"`
	Level         int    `json:"level"`
	Encrypt       bool   `json:"encrypt"`
	HeaderEncrypt bool   `json:"headerEncrypt"`
	SplitSize     int64  `json:"splitSize"`
	PasswordMode  string `json:"passwordMode"`
	PasswordLen   int    `json:"passwordLength"`
	DedupRule     string `json:"dedupRule"`
	NamePrefix    string `json:"namePrefix"`
	NameSuffix    string `json:"nameSuffix"`
	SeqStart      int64  `json:"seqStart"`
	NextSeq       int64  `json:"nextSeq"`
	Include       string `json:"include"`
	Exclude       string `json:"exclude"`
	ArchiveCount  int    `json:"archiveCount"`
	Enabled       bool   `json:"enabled"`
}

// ArchiveView is a ledger entry for a produced archive.
type ArchiveView struct {
	ID          string `json:"id"`
	Seq         int64  `json:"seq"`
	FolderName  string `json:"folderName"`
	ArchivePath string `json:"archivePath"`
	SourcePath  string `json:"sourcePath"`
	Size        int64  `json:"size"`
	FileCount   int    `json:"fileCount"`
	Status      string `json:"status"`
	PasswordID  string `json:"passwordId"`
	CreatedAt   string `json:"createdAt"`
}

// RunEvent is streamed to the frontend during a run (event name "job:event").
type RunEvent struct {
	Kind        string  `json:"kind"`
	RunID       string  `json:"runId"`
	TaskID      string  `json:"taskId"`
	Folder      string  `json:"folder"`
	Seq         int64   `json:"seq"`
	Index       int     `json:"index"`
	Total       int     `json:"total"`
	Phase       string  `json:"phase"`
	Percent     float64 `json:"percent"`
	Overall     float64 `json:"overallPercent"`
	Current     string  `json:"current"`
	ArchivePath string  `json:"archivePath"`
	Error       string  `json:"error"`
	BytesIn     int64   `json:"bytesIn"`
	BytesOut    int64   `json:"bytesOut"`
	Done        int     `json:"done"`
	Skipped     int     `json:"skipped"`
	Failed      int     `json:"failed"`
	ElapsedMS   int64   `json:"elapsedMs"`
}

// ToolStatus reports archiver availability.
type ToolStatus struct {
	Available bool     `json:"available"`
	Path      string   `json:"path"`
	Version   string   `json:"version"`
	Codecs    []string `json:"codecs"`
	Error     string   `json:"error"`
}
