package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/equals-chan/ElectricHamster/internal/config"
	"github.com/equals-chan/ElectricHamster/internal/export"
	"github.com/equals-chan/ElectricHamster/internal/id"
	"github.com/equals-chan/ElectricHamster/internal/job"
	"github.com/equals-chan/ElectricHamster/internal/legacy"
	"github.com/equals-chan/ElectricHamster/internal/store"
)

// TaskService is the frontend-facing API for tasks, runs and archives.
type TaskService struct{ rt *Runtime }

// List returns all tasks with their archive counts.
func (s *TaskService) List() ([]TaskView, error) {
	ctx := context.Background()
	tasks, err := s.rt.store.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TaskView, 0, len(tasks))
	for _, t := range tasks {
		count, _ := s.rt.store.CountArchivesByTask(ctx, t.ID)
		out = append(out, toTaskView(t, count))
	}
	return out, nil
}

// Get returns a single task by id.
func (s *TaskService) Get(taskID string) (TaskView, error) {
	t, err := s.rt.store.GetTask(context.Background(), taskID)
	if err != nil {
		return TaskView{}, err
	}
	count, _ := s.rt.store.CountArchivesByTask(context.Background(), t.ID)
	return toTaskView(t, count), nil
}

// Save creates or updates a task and returns its id.
func (s *TaskService) Save(in TaskInput) (string, error) {
	if strings.TrimSpace(in.Name) == "" {
		return "", fmt.Errorf("任务名称不能为空")
	}
	if strings.TrimSpace(in.SourceDir) == "" || strings.TrimSpace(in.DestDir) == "" {
		return "", fmt.Errorf("源目录与输出目录不能为空")
	}
	if in.Codec == "" {
		in.Codec = "zstd"
	}
	ctx := context.Background()

	var task store.Task
	isNew := strings.TrimSpace(in.ID) == ""
	if isNew {
		task.ID = id.New()
	} else {
		existing, err := s.rt.store.GetTask(ctx, in.ID)
		if err != nil {
			return "", err
		}
		task = existing
	}
	task.Name = in.Name
	task.SourceDir = in.SourceDir
	task.DestDir = in.DestDir
	task.IncludeGlobs = splitList(in.Include)
	task.ExcludeGlobs = splitList(in.Exclude)
	task.Format = "7z"
	task.Codec = in.Codec
	task.Level = in.Level
	task.Encrypt = in.Encrypt
	task.HeaderEncrypt = in.HeaderEncrypt
	task.SplitSize = in.SplitSize
	task.DedupRule = in.DedupRule
	if task.DedupRule == "" {
		task.DedupRule = "content_sig"
	}
	task.NamePrefix = in.NamePrefix
	task.NameSuffix = in.NameSuffix
	if in.SeqStart <= 0 {
		task.SeqStart = 10000
	} else {
		task.SeqStart = in.SeqStart
	}
	task.Enabled = true
	task.Password = store.PasswordPolicy{
		Mode:   in.PasswordMode,
		Length: in.PasswordLength,
		Fixed:  in.PasswordFixed,
	}
	if task.Password.Mode == "" {
		if task.Encrypt {
			task.Password.Mode = "random"
		} else {
			task.Password.Mode = "none"
		}
	}
	if task.Password.Mode == "random" && task.Password.Length <= 0 {
		task.Password.Length = 20
	}

	if isNew {
		if err := s.rt.store.CreateTask(ctx, task); err != nil {
			return "", err
		}
	} else {
		if err := s.rt.store.UpdateTask(ctx, task); err != nil {
			return "", err
		}
		// Editing the starting number raises the counter (never lowers it) so
		// it cannot collide with archives that were already produced.
		if err := s.rt.store.EnsureSeqAtLeast(ctx, store.SeqCounterName(task.ID), task.SeqStart); err != nil {
			return "", err
		}
	}
	return task.ID, nil
}

// Delete removes a task (its archive ledger is preserved).
func (s *TaskService) Delete(taskID string) error {
	return s.rt.store.DeleteTask(context.Background(), taskID)
}

// PickDirectory opens a native folder chooser.
func (s *TaskService) PickDirectory(title string) (string, error) {
	if s.rt.app == nil {
		return "", fmt.Errorf("application not ready")
	}
	if title == "" {
		title = "选择文件夹"
	}
	return s.rt.app.Dialog.OpenFile().
		SetTitle(title).
		CanChooseDirectories(true).
		CanChooseFiles(false).
		PromptForSingleSelection()
}

// IsRunning reports whether a run is in progress.
func (s *TaskService) IsRunning() bool {
	s.rt.mu.Lock()
	defer s.rt.mu.Unlock()
	return s.rt.running
}

// Run starts a task asynchronously. Progress is streamed via "job:event".
func (s *TaskService) Run(taskID string) error {
	if s.rt.archiverErr != nil {
		return s.rt.archiverErr
	}
	task, err := s.rt.store.GetTask(context.Background(), taskID)
	if err != nil {
		return err
	}
	if task.Encrypt {
		v := s.rt.currentVault()
		if v == nil || !v.IsUnlocked() {
			return fmt.Errorf("该任务需要加密，请先在“密码库”中解锁")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	if !s.rt.beginRun(cancel) {
		return fmt.Errorf("已有任务正在运行")
	}
	engine := &job.Engine{
		Store:    s.rt.store,
		Vault:    s.rt.currentVault(),
		Archiver: s.rt.archiver,
		Verify:   true,
	}
	s.rt.setEngine(engine)
	go func() {
		defer s.rt.endRun()
		emit := func(e job.Event) { s.rt.emit("job:event", toRunEvent(e)) }
		if _, err := engine.Run(ctx, task, emit); err != nil {
			s.rt.emit("job:event", RunEvent{Kind: "job:error", TaskID: taskID, Error: err.Error()})
		}
	}()
	return nil
}

// Pause stops the running task after the current folder finishes.
func (s *TaskService) Pause() error {
	e := s.rt.currentEngine()
	if e == nil {
		return fmt.Errorf("没有正在运行的任务")
	}
	e.Pause()
	s.rt.emit("job:event", RunEvent{Kind: "job:paused"})
	return nil
}

// Resume continues a paused task.
func (s *TaskService) Resume() error {
	e := s.rt.currentEngine()
	if e == nil {
		return fmt.Errorf("没有正在运行的任务")
	}
	e.Resume()
	s.rt.emit("job:event", RunEvent{Kind: "job:resumed"})
	return nil
}

// IsPaused reports whether the running task is paused.
func (s *TaskService) IsPaused() bool {
	e := s.rt.currentEngine()
	return e != nil && e.IsPaused()
}

// Cancel stops the running task.
func (s *TaskService) Cancel() error {
	s.rt.stopRun()
	return nil
}

// Archives returns the ledger for a task ("" = all tasks).
func (s *TaskService) Archives(taskID string) ([]ArchiveView, error) {
	archives, err := s.rt.store.ListArchives(context.Background(), taskID)
	if err != nil {
		return nil, err
	}
	out := make([]ArchiveView, 0, len(archives))
	for _, a := range archives {
		out = append(out, ArchiveView{
			ID:          a.ID,
			Seq:         a.Seq,
			FolderName:  a.FolderName,
			ArchivePath: a.ArchivePath,
			SourcePath:  a.SourcePath,
			Size:        a.ArchiveSize,
			FileCount:   a.FileCount,
			Status:      a.Status,
			PasswordID:  a.PasswordID,
			CreatedAt:   a.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return out, nil
}

// PickSaveFile opens a native save dialog for an .xlsx file.
func (s *TaskService) PickSaveFile(title, defaultName string) (string, error) {
	if s.rt.app == nil {
		return "", fmt.Errorf("application not ready")
	}
	if title == "" {
		title = "导出 Excel"
	}
	return s.rt.app.Dialog.SaveFile().
		SetMessage(title).
		SetFilename(defaultName).
		AddFilter("Excel", "*.xlsx").
		PromptForSingleSelection()
}

// ExportExcel writes the archive ledger (with passwords) to an .xlsx file.
// taskID "" exports all tasks. The vault must be unlocked when any archive has
// a stored password. Returns the number of rows written.
func (s *TaskService) ExportExcel(taskID, outPath string) (int, error) {
	archives, err := s.rt.store.ListArchives(context.Background(), taskID)
	if err != nil {
		return 0, err
	}
	v := s.rt.currentVault()
	var rows []export.ArchiveRow
	for _, a := range archives {
		pw := ""
		if a.PasswordID != "" {
			if v == nil || !v.IsUnlocked() {
				return 0, fmt.Errorf("请先解锁密码库，以便导出密码")
			}
			item, err := v.Get(a.PasswordID)
			if err != nil {
				return 0, err
			}
			pw = item.Password
		}
		rows = append(rows, export.ArchiveRow{
			Seq:         a.Seq,
			FolderName:  a.FolderName,
			ArchivePath: a.ArchivePath,
			Password:    pw,
			Size:        a.ArchiveSize,
			FileCount:   a.FileCount,
			CreatedAt:   a.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	if outPath == "" {
		dir, err := config.Dir()
		if err != nil {
			return 0, err
		}
		outPath = filepath.Join(dir, "passwords_"+time.Now().Format("20060102_150405")+".xlsx")
	}
	if err := export.WriteArchives(outPath, rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// ImportLegacy imports a legacy config.properties + db.sqlite3. The vault must
// be unlocked. Returns a short summary string.
func (s *TaskService) ImportLegacy(configPath string) (string, error) {
	v := s.rt.currentVault()
	if v == nil || !v.IsUnlocked() {
		return "", fmt.Errorf("请先解锁密码库，以便导入旧密码")
	}
	res, err := legacy.Import(context.Background(), legacy.ImportOptions{
		ConfigPath: configPath,
		Store:      s.rt.store,
		Vault:      v,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("已导入 %d 条归档、%d 个密码（跳过 %d 条已存在）", res.Archives, res.PasswordsIn, res.Skipped), nil
}

// PickLegacyConfig opens a chooser for the legacy config.properties file.
func (s *TaskService) PickLegacyConfig() (string, error) {
	if s.rt.app == nil {
		return "", fmt.Errorf("application not ready")
	}
	return s.rt.app.Dialog.OpenFile().
		SetTitle("选择旧的 config.properties").
		AddFilter("Properties", "*.properties").
		PromptForSingleSelection()
}

// Verify tests an archive's integrity using its stored password.
func (s *TaskService) Verify(archivePath, passwordID string) error {
	if s.rt.archiverErr != nil {
		return s.rt.archiverErr
	}
	pw, err := s.passwordFor(passwordID)
	if err != nil {
		return err
	}
	return s.rt.archiver.Verify(context.Background(), archivePath, pw)
}

// Extract unpacks an archive into outDir using its stored password.
func (s *TaskService) Extract(archivePath, outDir, passwordID string) error {
	if s.rt.archiverErr != nil {
		return s.rt.archiverErr
	}
	pw, err := s.passwordFor(passwordID)
	if err != nil {
		return err
	}
	return s.rt.archiver.Extract(context.Background(), archivePath, outDir, pw, nil)
}

// ToolStatus reports the 7-Zip engine availability.
func (s *TaskService) ToolStatus() ToolStatus {
	if s.rt.archiverErr != nil {
		return ToolStatus{Error: s.rt.archiverErr.Error()}
	}
	info, err := s.rt.archiver.Info(context.Background())
	if err != nil {
		return ToolStatus{Error: err.Error()}
	}
	return ToolStatus{Available: true, Path: info.Path, Version: info.Version, Codecs: info.Codecs}
}

// AppVersion returns the application version.
func (s *TaskService) AppVersion() string { return Version }

func (s *TaskService) passwordFor(passwordID string) (string, error) {
	if passwordID == "" {
		return "", nil
	}
	v := s.rt.currentVault()
	if v == nil || !v.IsUnlocked() {
		return "", fmt.Errorf("请先解锁密码库")
	}
	item, err := v.Get(passwordID)
	if err != nil {
		return "", err
	}
	return item.Password, nil
}

func splitList(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';' || r == ' '
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func toTaskView(t store.Task, count int) TaskView {
	return TaskView{
		ID:            t.ID,
		Name:          t.Name,
		SourceDir:     t.SourceDir,
		DestDir:       t.DestDir,
		Codec:         t.Codec,
		Level:         t.Level,
		Encrypt:       t.Encrypt,
		HeaderEncrypt: t.HeaderEncrypt,
		SplitSize:     t.SplitSize,
		PasswordMode:  t.Password.Mode,
		PasswordLen:   t.Password.Length,
		DedupRule:     t.DedupRule,
		NamePrefix:    t.NamePrefix,
		NameSuffix:    t.NameSuffix,
		SeqStart:      t.SeqStart,
		Include:       strings.Join(t.IncludeGlobs, ", "),
		Exclude:       strings.Join(t.ExcludeGlobs, ", "),
		ArchiveCount:  count,
		Enabled:       t.Enabled,
	}
}

func toRunEvent(e job.Event) RunEvent {
	ev := RunEvent{
		Kind:   string(e.Kind),
		RunID:  e.RunID,
		TaskID: e.TaskID,
		Folder: e.Folder,
		Seq:    e.Seq,
		Index:  e.Index,
		Total:  e.Total,
	}
	if e.Progress.Phase != "" {
		ev.Phase = string(e.Progress.Phase)
	}
	ev.Percent = e.Progress.Percent
	ev.Overall = e.OverallPercent
	ev.Current = e.Progress.Current
	ev.BytesOut = e.Progress.BytesOut
	if e.Err != nil {
		ev.Error = e.Err.Error()
	}
	ev.ArchivePath = e.ArchivePath
	if e.Kind == job.EventRunFinished {
		ev.BytesIn = e.Stats.BytesIn
		ev.BytesOut = e.Stats.BytesOut
		ev.Done = e.Stats.Done
		ev.Skipped = e.Stats.Skipped
		ev.Failed = e.Stats.Failed
		ev.ElapsedMS = e.Stats.Elapsed.Milliseconds()
	}
	return ev
}
