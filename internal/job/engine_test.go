package job

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/equals-chan/ElectricHamster/internal/archive"
	"github.com/equals-chan/ElectricHamster/internal/id"
	"github.com/equals-chan/ElectricHamster/internal/store"
	"github.com/equals-chan/ElectricHamster/internal/vault"
)

// fakeArchiver writes a stand-in archive file so pipeline logic can be tested
// without a real 7-Zip binary.
type fakeArchiver struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeArchiver) Compress(_ context.Context, srcDir, outFile string, _ archive.Options, onProgress archive.ProgressFunc) error {
	if err := os.MkdirAll(filepath.Dir(outFile), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outFile, []byte("archive of "+srcDir), 0o644); err != nil {
		return err
	}
	if onProgress != nil {
		onProgress(archive.Progress{Phase: archive.PhaseCompress, Percent: 50, Current: "file"})
		onProgress(archive.Progress{Phase: archive.PhaseDone, Percent: 100})
	}
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return nil
}

func (f *fakeArchiver) Extract(context.Context, string, string, string, archive.ProgressFunc) error {
	return nil
}
func (f *fakeArchiver) List(context.Context, string, string) ([]archive.Entry, error) {
	return nil, nil
}
func (f *fakeArchiver) Verify(context.Context, string, string) error { return nil }
func (f *fakeArchiver) Info(context.Context) (archive.ToolInfo, error) {
	return archive.ToolInfo{Codecs: []string{"zstd"}, HasZstd: true, HasAES: true}, nil
}

func testVaultParams() vault.KDFParams {
	p := vault.DefaultKDFParams()
	p.Time = 1
	p.Memory = 8 * 1024
	p.Parallelism = 1
	return p
}

func writeTree(t *testing.T, root string, folders map[string][]string) {
	t.Helper()
	for folder, files := range folders {
		dir := filepath.Join(root, folder)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(folder+"/"+name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

type harness struct {
	store    *store.Store
	vault    *vault.Vault
	archiver *fakeArchiver
	engine   *Engine
	task     store.Task
	srcDir   string
	dstDir   string
}

func newHarness(t *testing.T, folders map[string][]string, encrypt bool) *harness {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	writeTree(t, src, folders)

	st, err := store.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	h := &harness{store: st, archiver: &fakeArchiver{}, srcDir: src, dstDir: dst}
	task := store.Task{
		ID:            id.New(),
		Name:          "test",
		SourceDir:     src,
		DestDir:       dst,
		Format:        "7z",
		Codec:         "zstd",
		Level:         3,
		Encrypt:       encrypt,
		HeaderEncrypt: encrypt,
		Password:      store.PasswordPolicy{Mode: "random", Length: 16},
		DedupRule:     "content_sig",
		Enabled:       true,
	}
	if encrypt {
		v, err := vault.Create(filepath.Join(dir, "vault.db"), "master", testVaultParams())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { v.Close() })
		h.vault = v
	}
	h.task = task
	h.engine = &Engine{Store: st, Vault: h.vault, Archiver: h.archiver, Workers: 2}
	if err := st.CreateTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *harness) run(t *testing.T, events *[]Event) Stats {
	t.Helper()
	var emit EmitFunc
	if events != nil {
		emit = func(e Event) { *events = append(*events, e) }
	}
	stats, err := h.engine.Run(context.Background(), h.task, emit)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return stats
}

func TestEngineArchivesAndRecords(t *testing.T) {
	h := newHarness(t, map[string][]string{
		"a": {"1.txt", "2.txt"},
		"b": {"x.txt"},
		"c": {"y.txt"},
	}, true)

	var events []Event
	stats := h.run(t, &events)

	if stats.Total != 3 || stats.Done != 3 || stats.Skipped != 0 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want total=3 done=3", stats)
	}
	if h.archiver.calls != 3 {
		t.Errorf("archiver calls = %d, want 3", h.archiver.calls)
	}

	archives, err := h.store.ListArchives(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(archives) != 3 {
		t.Fatalf("archives = %d, want 3", len(archives))
	}
	for i, a := range archives {
		want := int64(10000 + i)
		if a.Seq != want {
			t.Errorf("archive[%d].Seq = %d, want %d", i, a.Seq, want)
		}
		if a.PasswordID == "" {
			t.Errorf("archive %d has no password id", i)
		}
		item, err := h.vault.Get(a.PasswordID)
		if err != nil {
			t.Fatalf("vault.Get: %v", err)
		}
		if len(item.Password) != 16 {
			t.Errorf("password length = %d, want 16", len(item.Password))
		}
	}

	// Events: run started, per-item started/done, progress, run finished.
	var started, itemDone, progress, finished int
	for _, e := range events {
		switch e.Kind {
		case EventRunStarted:
			started++
		case EventItemDone:
			itemDone++
		case EventProgress:
			progress++
		case EventRunFinished:
			finished++
		}
	}
	if started != 1 || finished != 1 || itemDone != 3 || progress == 0 {
		t.Errorf("events started=%d itemDone=%d progress=%d finished=%d", started, itemDone, progress, finished)
	}
}

func TestEngineSkipsAlreadyArchived(t *testing.T) {
	h := newHarness(t, map[string][]string{"a": {"1.txt"}, "b": {"2.txt"}}, true)
	if s := h.run(t, nil); s.Done != 2 {
		t.Fatalf("first run done = %d, want 2", s.Done)
	}
	// Second run over unchanged folders must skip everything.
	s := h.run(t, nil)
	if s.Done != 0 || s.Skipped != 2 {
		t.Fatalf("second run = %+v, want done=0 skipped=2", s)
	}
	if h.archiver.calls != 2 {
		t.Errorf("archiver calls = %d, want 2 (no recompression)", h.archiver.calls)
	}

	// Changing one folder's contents invalidates its signature.
	if err := os.WriteFile(filepath.Join(h.srcDir, "a", "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	s = h.run(t, nil)
	if s.Done != 1 || s.Skipped != 1 {
		t.Fatalf("after change = %+v, want done=1 skipped=1", s)
	}
}

func TestEngineUnencrypted(t *testing.T) {
	h := newHarness(t, map[string][]string{"a": {"1.txt"}}, false)
	s := h.run(t, nil)
	if s.Done != 1 {
		t.Fatalf("done = %d, want 1", s.Done)
	}
	archives, _ := h.store.ListArchives(context.Background(), "")
	if len(archives) != 1 || archives[0].PasswordID != "" {
		t.Errorf("unencrypted archive should have empty password id: %+v", archives)
	}
}

func TestEngineIncludeExclude(t *testing.T) {
	h := newHarness(t, map[string][]string{"keep": {"1.txt"}, "skip": {"2.txt"}}, true)
	h.task.ExcludeGlobs = []string{"skip"}
	s := h.run(t, nil)
	if s.Done != 1 || s.Skipped != 1 {
		t.Fatalf("stats = %+v, want done=1 skipped=1", s)
	}
}

func TestEngineFixedPassword(t *testing.T) {
	h := newHarness(t, map[string][]string{"a": {"1.txt"}}, true)
	h.task.Password = store.PasswordPolicy{Mode: "fixed", Fixed: "SharedPass123"}
	if s := h.run(t, nil); s.Done != 1 {
		t.Fatalf("done = %d", s.Done)
	}
	archives, _ := h.store.ListArchives(context.Background(), "")
	item, err := h.vault.Get(archives[0].PasswordID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Password != "SharedPass123" {
		t.Errorf("password = %q, want SharedPass123", item.Password)
	}
}

func TestEngineFolderNameDedup(t *testing.T) {
	h := newHarness(t, map[string][]string{"game": {"save.dat"}, "other": {"x.txt"}}, true)
	h.task.DedupRule = "folder_name"

	if s := h.run(t, nil); s.Done != 2 {
		t.Fatalf("first run done = %d, want 2", s.Done)
	}
	// Change the contents (e.g. a game save). With folder_name de-dup the
	// folder is still skipped because its name is unchanged.
	if err := os.WriteFile(filepath.Join(h.srcDir, "game", "save.dat"), []byte("changed save"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := h.run(t, nil)
	if s.Done != 0 || s.Skipped != 2 {
		t.Fatalf("second run = %+v, want done=0 skipped=2", s)
	}
	if h.archiver.calls != 2 {
		t.Errorf("archiver calls = %d, want 2", h.archiver.calls)
	}
}

func TestEnginePerTaskNamingAndNumbering(t *testing.T) {
	h := newHarness(t, map[string][]string{"a": {"1.txt"}, "b": {"2.txt"}}, true)
	h.task.NamePrefix = "game_"
	h.task.NameSuffix = "_x"
	h.task.SeqStart = 500
	// The stored task must carry the naming settings too.
	if err := h.store.UpdateTask(context.Background(), h.task); err != nil {
		t.Fatal(err)
	}

	if s := h.run(t, nil); s.Done != 2 {
		t.Fatalf("done = %d, want 2", s.Done)
	}
	archives, _ := h.store.ListArchives(context.Background(), "")

	byName := map[string]store.Archive{}
	for _, a := range archives {
		byName[a.FolderName] = a
	}
	if got := filepath.Base(byName["a"].ArchivePath); got != "game_500_x.7z" {
		t.Errorf("archive a name = %q, want game_500_x.7z", got)
	}
	if got := filepath.Base(byName["b"].ArchivePath); got != "game_501_x.7z" {
		t.Errorf("archive b name = %q, want game_501_x.7z", got)
	}
	if byName["a"].Seq != 500 || byName["b"].Seq != 501 {
		t.Errorf("seqs = %d,%d, want 500,501", byName["a"].Seq, byName["b"].Seq)
	}

	// A second task must have an independent counter starting at its own value.
	h2 := newHarness(t, map[string][]string{"z": {"z.txt"}}, true)
	h2.task.SeqStart = 1
	if err := h2.store.UpdateTask(context.Background(), h2.task); err != nil {
		t.Fatal(err)
	}
	h2.run(t, nil)
	arch2, _ := h2.store.ListArchives(context.Background(), "")
	if len(arch2) != 1 || arch2[0].Seq != 1 {
		t.Fatalf("second task seq = %+v, want 1 (independent counter)", arch2)
	}
}

func TestProgressTrackerAggregate(t *testing.T) {
	jobs := []plannedJob{
		{index: 1, info: TreeInfo{Bytes: 100}},
		{index: 2, info: TreeInfo{Bytes: 300}},
	}
	tr := newProgressTracker(jobs)
	if got := tr.set(1, 100); got != 25 {
		t.Errorf("after job1=100%%: overall = %v, want 25", got)
	}
	if got := tr.set(2, 50); got != 62.5 {
		t.Errorf("after job2=50%%: overall = %v, want 62.5", got)
	}
	// Equal weighting fallback when total bytes is zero.
	tr2 := newProgressTracker([]plannedJob{{index: 1}, {index: 2}, {index: 3}, {index: 4}})
	if got := tr2.set(1, 100); got != 25 {
		t.Errorf("count-weighted overall = %v, want 25", got)
	}
}

func TestEnginePauseResume(t *testing.T) {
	e := &Engine{}
	if e.IsPaused() {
		t.Fatal("engine should start unpaused")
	}
	e.Pause()
	if !e.IsPaused() {
		t.Fatal("engine should be paused")
	}

	done := make(chan error, 1)
	go func() { done <- e.waitIfPaused(context.Background()) }()
	select {
	case <-done:
		t.Fatal("waitIfPaused returned while paused")
	case <-time.After(50 * time.Millisecond):
	}

	e.Resume()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("waitIfPaused after resume: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waitIfPaused did not return after resume")
	}

	// Cancellation must unblock a paused wait.
	e.Pause()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.waitIfPaused(ctx); err == nil {
		t.Fatal("expected context error while paused with cancelled context")
	}
}
