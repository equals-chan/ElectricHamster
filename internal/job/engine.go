package job

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/equals-chan/ElectricHamster/internal/archive"
	"github.com/equals-chan/ElectricHamster/internal/id"
	"github.com/equals-chan/ElectricHamster/internal/store"
	"github.com/equals-chan/ElectricHamster/internal/vault"
)

// EventKind identifies a pipeline event.
type EventKind string

const (
	EventRunStarted  EventKind = "job:started"
	EventItemStarted EventKind = "job:item_started"
	EventProgress    EventKind = "job:progress"
	EventItemDone    EventKind = "job:item_done"
	EventItemSkipped EventKind = "job:item_skipped"
	EventRunFinished EventKind = "job:finished"
	EventPaused      EventKind = "job:paused"
	EventResumed     EventKind = "job:resumed"
)

// Stats summarises a run.
type Stats struct {
	Total    int
	Done     int
	Skipped  int
	Failed   int
	BytesIn  int64
	BytesOut int64
	Elapsed  time.Duration
}

// Event is delivered to the emit callback. Fields are populated per kind.
type Event struct {
	Kind           EventKind
	RunID          string
	TaskID         string
	Folder         string
	Seq            int64
	Index          int
	Total          int
	Progress       archive.Progress
	OverallPercent float64
	ArchivePath    string
	Err            error
	Stats          Stats
}

// EmitFunc receives pipeline events. It must be safe for concurrent calls.
type EmitFunc func(Event)

// Engine runs archiving tasks.
type Engine struct {
	Store    *store.Store
	Vault    *vault.Vault // required when a task encrypts
	Archiver archive.Archiver
	Workers  int
	Verify   bool

	// SeqCounter is the counters table key used for archive numbering. When
	// empty it defaults to a per-task counter (store.SeqCounterName).
	SeqCounter string
	// SeqStart is the first sequence number allocated when the counter is new.
	SeqStart int64

	pauseMu  sync.Mutex
	paused   bool
	resumeCh chan struct{}
}

// Pause stops workers from starting new folders. The folder currently being
// compressed finishes first.
func (e *Engine) Pause() {
	e.pauseMu.Lock()
	defer e.pauseMu.Unlock()
	if !e.paused {
		e.paused = true
		e.resumeCh = make(chan struct{})
	}
}

// Resume continues a paused run.
func (e *Engine) Resume() {
	e.pauseMu.Lock()
	defer e.pauseMu.Unlock()
	if e.paused {
		close(e.resumeCh)
		e.resumeCh = nil
		e.paused = false
	}
}

// IsPaused reports whether the engine is paused.
func (e *Engine) IsPaused() bool {
	e.pauseMu.Lock()
	defer e.pauseMu.Unlock()
	return e.paused
}

// waitIfPaused blocks while paused or until the context is cancelled.
func (e *Engine) waitIfPaused(ctx context.Context) error {
	e.pauseMu.Lock()
	ch := e.resumeCh
	paused := e.paused
	e.pauseMu.Unlock()
	if !paused {
		return nil
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type plannedJob struct {
	unit  Unit
	info  TreeInfo
	index int
}

// progressTracker aggregates per-job progress into a single overall percentage.
// Jobs are weighted by input size (falling back to an equal weight per job when
// the total is zero), so the overall bar advances smoothly and monotonically
// even though several workers report concurrently.
type progressTracker struct {
	mu      sync.Mutex
	percent map[int]float64
	bytes   map[int]int64
	total   int64
	byCount bool
}

func newProgressTracker(jobs []plannedJob) *progressTracker {
	t := &progressTracker{percent: map[int]float64{}, bytes: map[int]int64{}}
	var total int64
	for _, j := range jobs {
		t.bytes[j.index] = j.info.Bytes
		total += j.info.Bytes
	}
	if total > 0 {
		t.total = total
	} else {
		t.byCount = true
		t.total = int64(len(jobs))
	}
	return t
}

// set records a job's percentage and returns the aggregate percentage.
func (t *progressTracker) set(index int, pct float64) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.percent[index] = pct
	if t.total == 0 {
		return 0
	}
	var done float64
	if t.byCount {
		for _, p := range t.percent {
			done += p / 100
		}
	} else {
		for idx, p := range t.percent {
			done += p / 100 * float64(t.bytes[idx])
		}
	}
	return done / float64(t.total) * 100
}

type counters struct {
	mu       sync.Mutex
	done     int
	skipped  int
	failed   int
	bytesIn  int64
	bytesOut int64
}

func (c *counters) add(i int, in, out int64, failed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if failed {
		c.failed++
	}
	c.done += i
	c.bytesIn += in
	c.bytesOut += out
}

// Run executes task to completion. It returns the run stats and a non-nil error
// only for setup/fatal problems; per-folder failures are reported in Stats and
// via events.
func (e *Engine) Run(ctx context.Context, task store.Task, emit EmitFunc) (Stats, error) {
	if e.Store == nil {
		return Stats{}, errors.New("job: engine has no store")
	}
	if e.Archiver == nil {
		return Stats{}, errors.New("job: engine has no archiver")
	}
	if emit == nil {
		emit = func(Event) {}
	}
	workers := e.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > 16 {
		workers = 16
	}
	seqCounter := e.SeqCounter
	if seqCounter == "" {
		seqCounter = store.SeqCounterName(task.ID)
	}
	seqStart := e.SeqStart
	if seqStart == 0 {
		seqStart = task.SeqStart
	}
	if seqStart == 0 {
		seqStart = 10000
	}

	started := time.Now()
	run := store.Run{ID: id.New(), TaskID: task.ID, Status: store.RunRunning, Total: 0}
	if err := e.Store.CreateRun(ctx, run); err != nil {
		return Stats{}, fmt.Errorf("job: create run: %w", err)
	}

	units, err := Scan(task.SourceDir, 1)
	if err != nil {
		e.Store.FinishRun(ctx, run.ID, store.RunFailed, 0, 0, err.Error())
		return Stats{}, err
	}
	stats := Stats{Total: len(units)}
	emit(Event{Kind: EventRunStarted, RunID: run.ID, TaskID: task.ID, Total: len(units)})

	// Planning: filter, signature, dedup.
	var jobs []plannedJob
	for _, u := range units {
		if !MatchFilters(u.Name, task.IncludeGlobs, task.ExcludeGlobs) {
			stats.Skipped++
			emit(Event{Kind: EventItemSkipped, RunID: run.ID, TaskID: task.ID, Folder: u.Name, Err: fmt.Errorf("filtered out")})
			continue
		}
		// "folder_name" de-duplication can skip without walking the tree at all,
		// which matters for very large folders.
		if task.DedupRule == "folder_name" {
			existing, found, err := e.Store.ArchiveByFolderName(ctx, task.ID, u.Name)
			if err != nil {
				return stats, fmt.Errorf("job: dedup: %w", err)
			}
			if found {
				stats.Skipped++
				emit(Event{Kind: EventItemSkipped, RunID: run.ID, TaskID: task.ID, Folder: u.Name, ArchivePath: existing.ArchivePath})
				continue
			}
		}
		info, err := ScanTree(u.Path)
		if err != nil {
			stats.Failed++
			emit(Event{Kind: EventItemDone, RunID: run.ID, TaskID: task.ID, Folder: u.Name, Err: err})
			continue
		}
		if task.DedupRule == "" || task.DedupRule == "content_sig" {
			if existing, found, err := e.Store.ArchiveBySig(ctx, info.Signature); err != nil {
				return stats, fmt.Errorf("job: dedup: %w", err)
			} else if found {
				stats.Skipped++
				emit(Event{Kind: EventItemSkipped, RunID: run.ID, TaskID: task.ID, Folder: u.Name, ArchivePath: existing.ArchivePath})
				continue
			}
		}
		jobs = append(jobs, plannedJob{unit: u, info: info, index: len(jobs) + 1})
	}

	tracker := newProgressTracker(jobs)
	var c counters
	jobCh := make(chan plannedJob)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				e.process(ctx, task, run.ID, j, len(jobs), seqCounter, seqStart, tracker, emit, &c)
			}
		}()
	}
	for _, j := range jobs {
		select {
		case <-ctx.Done():
			break
		case jobCh <- j:
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobCh)
	wg.Wait()

	c.mu.Lock()
	stats.Done = c.done
	stats.Skipped += c.skipped
	stats.Failed += c.failed
	stats.BytesIn = c.bytesIn
	stats.BytesOut = c.bytesOut
	c.mu.Unlock()
	stats.Elapsed = time.Since(started)

	status := store.RunDone
	var runErr string
	switch {
	case ctx.Err() != nil:
		status = store.RunCanceled
		runErr = ctx.Err().Error()
	case stats.Failed > 0:
		status = store.RunFailed
		runErr = fmt.Sprintf("%d folder(s) failed", stats.Failed)
	}
	if err := e.Store.FinishRun(ctx, run.ID, status, stats.BytesIn, stats.BytesOut, runErr); err != nil {
		return stats, err
	}
	emit(Event{Kind: EventRunFinished, RunID: run.ID, TaskID: task.ID, Stats: stats})
	return stats, nil
}

func (e *Engine) process(
	ctx context.Context,
	task store.Task,
	runID string,
	j plannedJob,
	total int,
	seqCounter string,
	seqStart int64,
	tracker *progressTracker,
	emit EmitFunc,
	c *counters,
) {
	if ctx.Err() != nil {
		return
	}
	if err := e.waitIfPaused(ctx); err != nil {
		return
	}
	emit(Event{Kind: EventItemStarted, RunID: runID, TaskID: task.ID,
		Folder: j.unit.Name, Index: j.index, Total: total})

	seq, err := e.Store.NextSeq(ctx, seqCounter, seqStart)
	if err != nil {
		e.finishItem(ctx, runID, j, 0, "", fmt.Errorf("seq: %w", err), tracker, emit, c)
		return
	}

	pwd, pwdID, err := e.resolvePassword(task, j.unit.Name)
	if err != nil {
		e.finishItem(ctx, runID, j, seq, "", err, tracker, emit, c)
		return
	}

	format := task.Format
	if format == "" {
		format = "7z"
	}
	outPath := filepath.Join(task.DestDir, task.ArchiveFileName(seq, format))

	opt := archive.Options{
		Format:        archive.Format(format),
		Codec:         archive.Codec(task.Codec),
		Level:         task.Level,
		Encrypt:       task.Encrypt,
		HeaderEncrypt: task.HeaderEncrypt,
		SplitSize:     task.SplitSize,
		Password:      pwd,
		Verify:        e.Verify,
	}
	onProgress := func(p archive.Progress) {
		overall := tracker.set(j.index, p.Percent)
		emit(Event{Kind: EventProgress, RunID: runID, TaskID: task.ID, Folder: j.unit.Name,
			Seq: seq, Index: j.index, Total: total, Progress: p, OverallPercent: overall})
	}
	if err := e.Archiver.Compress(ctx, j.unit.Path, outPath, opt, onProgress); err != nil {
		e.finishItem(ctx, runID, j, seq, outPath, err, tracker, emit, c)
		return
	}

	ref := outPath
	if task.SplitSize > 0 {
		ref = outPath + ".001"
	}
	var size int64
	if fi, statErr := statSize(ref); statErr == nil {
		size = fi
	}
	rec := store.Archive{
		ID:          id.New(),
		RunID:       runID,
		TaskID:      task.ID,
		Seq:         seq,
		FolderName:  j.unit.Name,
		SourcePath:  j.unit.Path,
		ArchivePath: outPath,
		ArchiveSize: size,
		FileCount:   j.info.Files,
		ContentSig:  j.info.Signature,
		PasswordID:  pwdID,
		Status:      store.ArchiveOK,
	}
	if err := e.Store.InsertArchive(ctx, rec); err != nil {
		e.finishItem(ctx, runID, j, seq, outPath, fmt.Errorf("record: %w", err), tracker, emit, c)
		return
	}
	c.add(1, j.info.Bytes, size, false)
	if err := e.Store.UpdateRunProgress(ctx, runID, currentDone(c), currentIn(c), currentOut(c)); err != nil {
		// Non-fatal: the run still completes.
		_ = err
	}
	e.finishItem(ctx, runID, j, seq, outPath, nil, tracker, emit, c)
}

// finishItem emits the terminal event for a job and updates counters.
func (e *Engine) finishItem(ctx context.Context, runID string, j plannedJob, seq int64, archivePath string, err error, tracker *progressTracker, emit EmitFunc, c *counters) {
	if err != nil {
		c.add(0, 0, 0, true)
	}
	tracker.set(j.index, 100)
	emit(Event{Kind: EventItemDone, RunID: runID, Folder: j.unit.Name, Seq: seq,
		ArchivePath: archivePath, Err: err})
}

func (e *Engine) resolvePassword(task store.Task, folderName string) (password, passwordID string, err error) {
	if !task.Encrypt || task.Password.Mode == "none" {
		return "", "", nil
	}
	if e.Vault == nil {
		return "", "", errors.New("job: task encrypts but no vault is available")
	}
	switch task.Password.Mode {
	case "", "random":
		length := task.Password.Length
		if length <= 0 {
			length = 20
		}
		charset := task.Password.Charset
		if charset == "" {
			charset = vault.CharsetAlnum
		}
		password, err = vault.GeneratePasswordWith(length, charset)
		if err != nil {
			return "", "", err
		}
	case "fixed":
		if task.Password.Fixed == "" {
			return "", "", errors.New("job: fixed password policy has no value")
		}
		password = task.Password.Fixed
	default:
		return "", "", fmt.Errorf("job: unknown password mode %q", task.Password.Mode)
	}
	passwordID, err = e.Vault.Put(folderName, password, "")
	if err != nil {
		return "", "", fmt.Errorf("job: store password: %w", err)
	}
	return password, passwordID, nil
}

func currentDone(c *counters) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.done
}

func currentIn(c *counters) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytesIn
}

func currentOut(c *counters) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytesOut
}

func statSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}
