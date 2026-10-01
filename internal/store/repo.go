package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("store: not found")

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

func parseTime(s sql.NullString) time.Time {
	if !s.Valid || s.String == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s.String)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseTimeStr(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func marshalStrings(v []string) string {
	if len(v) == 0 {
		return ""
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func unmarshalStrings(s sql.NullString) []string {
	if !s.Valid || s.String == "" {
		return nil
	}
	var v []string
	_ = json.Unmarshal([]byte(s.String), &v)
	return v
}

// --- tasks ---

// CreateTask inserts a task.
func (s *Store) CreateTask(ctx context.Context, t Task) error {
	policy, _ := json.Marshal(t.Password)
	if t.SeqStart == 0 {
		t.SeqStart = 10000
	}
	_, err := s.conn.ExecContext(ctx, `INSERT INTO tasks
		(id, name, source_dir, dest_dir, include_globs, exclude_globs, format, codec, level,
		 encrypt, header_encrypt, split_size, password_policy, dedup_rule, name_prefix, name_suffix,
		 seq_start, enabled, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Name, t.SourceDir, t.DestDir, marshalStrings(t.IncludeGlobs), marshalStrings(t.ExcludeGlobs),
		t.Format, t.Codec, t.Level, boolToInt(t.Encrypt), boolToInt(t.HeaderEncrypt), t.SplitSize,
		string(policy), t.DedupRule, t.NamePrefix, t.NameSuffix, t.SeqStart,
		boolToInt(t.Enabled), nowUTC(), nowUTC())
	return err
}

// UpdateTask updates a task's mutable fields.
func (s *Store) UpdateTask(ctx context.Context, t Task) error {
	policy, _ := json.Marshal(t.Password)
	if t.SeqStart == 0 {
		t.SeqStart = 10000
	}
	res, err := s.conn.ExecContext(ctx, `UPDATE tasks SET
		name=?, source_dir=?, dest_dir=?, include_globs=?, exclude_globs=?, format=?, codec=?, level=?,
		encrypt=?, header_encrypt=?, split_size=?, password_policy=?, dedup_rule=?,
		name_prefix=?, name_suffix=?, seq_start=?, enabled=?, updated_at=?
		WHERE id=?`,
		t.Name, t.SourceDir, t.DestDir, marshalStrings(t.IncludeGlobs), marshalStrings(t.ExcludeGlobs),
		t.Format, t.Codec, t.Level, boolToInt(t.Encrypt), boolToInt(t.HeaderEncrypt), t.SplitSize,
		string(policy), t.DedupRule, t.NamePrefix, t.NameSuffix, t.SeqStart,
		boolToInt(t.Enabled), nowUTC(), t.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanTask(row interface{ Scan(...any) error }) (Task, error) {
	var (
		t                        Task
		include, exclude         sql.NullString
		policy                   string
		encrypt, header, enabled int
		created, updated         string
	)
	err := row.Scan(&t.ID, &t.Name, &t.SourceDir, &t.DestDir, &include, &exclude,
		&t.Format, &t.Codec, &t.Level, &encrypt, &header, &t.SplitSize, &policy,
		&t.DedupRule, &t.NamePrefix, &t.NameSuffix, &t.SeqStart, &enabled, &created, &updated)
	if err != nil {
		return Task{}, err
	}
	t.IncludeGlobs = unmarshalStrings(include)
	t.ExcludeGlobs = unmarshalStrings(exclude)
	t.Encrypt = encrypt != 0
	t.HeaderEncrypt = header != 0
	t.Enabled = enabled != 0
	_ = json.Unmarshal([]byte(policy), &t.Password)
	t.CreatedAt = parseTimeStr(created)
	t.UpdatedAt = parseTimeStr(updated)
	return t, nil
}

const taskColumns = `id, name, source_dir, dest_dir, include_globs, exclude_globs, format, codec, level,
	encrypt, header_encrypt, split_size, password_policy, dedup_rule, name_prefix, name_suffix,
	seq_start, enabled, created_at, updated_at`

// GetTask returns a task by id.
func (s *Store) GetTask(ctx context.Context, taskID string) (Task, error) {
	row := s.conn.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, taskID)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	return t, err
}

// FindTaskByName returns a task by its unique-ish name.
func (s *Store) FindTaskByName(ctx context.Context, name string) (Task, error) {
	row := s.conn.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE name = ? ORDER BY created_at LIMIT 1`, name)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	return t, err
}

// ListTasks returns all tasks ordered by creation time.
func (s *Store) ListTasks(ctx context.Context) ([]Task, error) {
	rows, err := s.conn.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTask removes a task. Related runs/archives are kept (their task_id is
// set to NULL by the foreign key) so the ledger is not lost.
func (s *Store) DeleteTask(ctx context.Context, taskID string) error {
	res, err := s.conn.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, taskID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountArchivesByTask returns how many archives belong to a task.
func (s *Store) CountArchivesByTask(ctx context.Context, taskID string) (int, error) {
	var n int
	err := s.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM archives WHERE task_id = ?`, taskID).Scan(&n)
	return n, err
}

// --- runs ---

// CreateRun inserts a new run row.
func (s *Store) CreateRun(ctx context.Context, r Run) error {
	_, err := s.conn.ExecContext(ctx, `INSERT INTO runs
		(id, task_id, status, total, done, bytes_in, bytes_out, started_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		r.ID, r.TaskID, r.Status, r.Total, r.Done, r.BytesIn, r.BytesOut, nowUTC())
	return err
}

// UpdateRunProgress updates a run's counters.
func (s *Store) UpdateRunProgress(ctx context.Context, runID string, done int, bytesIn, bytesOut int64) error {
	_, err := s.conn.ExecContext(ctx, `UPDATE runs SET done=?, bytes_in=?, bytes_out=? WHERE id=?`,
		done, bytesIn, bytesOut, runID)
	return err
}

// FinishRun marks a run complete.
func (s *Store) FinishRun(ctx context.Context, runID, status string, bytesIn, bytesOut int64, errMsg string) error {
	_, err := s.conn.ExecContext(ctx, `UPDATE runs SET status=?, bytes_in=?, bytes_out=?, finished_at=?, error=? WHERE id=?`,
		status, bytesIn, bytesOut, nowUTC(), errMsg, runID)
	return err
}

func scanRun(row interface{ Scan(...any) error }) (Run, error) {
	var r Run
	var started, finished sql.NullString
	var errMsg sql.NullString
	if err := row.Scan(&r.ID, &r.TaskID, &r.Status, &r.Total, &r.Done, &r.BytesIn, &r.BytesOut,
		&started, &finished, &errMsg); err != nil {
		return Run{}, err
	}
	r.StartedAt = parseTime(started)
	r.FinishedAt = parseTime(finished)
	r.Error = errMsg.String
	return r, nil
}

// GetRun returns a run by id.
func (s *Store) GetRun(ctx context.Context, runID string) (Run, error) {
	row := s.conn.QueryRowContext(ctx, `SELECT id, task_id, status, total, done, bytes_in, bytes_out,
		started_at, finished_at, error FROM runs WHERE id = ?`, runID)
	r, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	return r, err
}

// --- archives ---

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// InsertArchive records a produced archive.
func (s *Store) InsertArchive(ctx context.Context, a Archive) error {
	_, err := s.conn.ExecContext(ctx, `INSERT INTO archives
		(id, run_id, task_id, seq, folder_name, source_path, archive_path, archive_size,
		 file_count, content_sig, password_id, status, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, nullIfEmpty(a.RunID), nullIfEmpty(a.TaskID), a.Seq, a.FolderName, a.SourcePath, a.ArchivePath, a.ArchiveSize,
		a.FileCount, a.ContentSig, nullIfEmpty(a.PasswordID), a.Status, nowUTC())
	return err
}

const archiveColumns = `id, run_id, task_id, seq, folder_name, source_path, archive_path,
	archive_size, file_count, content_sig, password_id, status, created_at`

func scanArchive(row interface{ Scan(...any) error }) (Archive, error) {
	var a Archive
	var runID, taskID, passwordID sql.NullString
	var created string
	if err := row.Scan(&a.ID, &runID, &taskID, &a.Seq, &a.FolderName, &a.SourcePath, &a.ArchivePath,
		&a.ArchiveSize, &a.FileCount, &a.ContentSig, &passwordID, &a.Status, &created); err != nil {
		return Archive{}, err
	}
	a.RunID = runID.String
	a.TaskID = taskID.String
	a.PasswordID = passwordID.String
	a.CreatedAt = parseTimeStr(created)
	return a, nil
}

// ArchiveBySig finds an OK archive whose content signature matches.
func (s *Store) ArchiveBySig(ctx context.Context, sig string) (Archive, bool, error) {
	row := s.conn.QueryRowContext(ctx, `SELECT `+archiveColumns+`
		FROM archives WHERE content_sig = ? AND status = ? ORDER BY created_at LIMIT 1`, sig, ArchiveOK)
	a, err := scanArchive(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Archive{}, false, nil
	}
	return a, err == nil, err
}

// ArchiveByPath finds an archive by its output path.
func (s *Store) ArchiveByPath(ctx context.Context, path string) (Archive, bool, error) {
	row := s.conn.QueryRowContext(ctx, `SELECT `+archiveColumns+` FROM archives WHERE archive_path = ?`, path)
	a, err := scanArchive(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Archive{}, false, nil
	}
	return a, err == nil, err
}

// ArchiveByFolderName finds an OK archive for the same task whose source folder
// has the given name. Used by the "folder_name" de-duplication rule.
func (s *Store) ArchiveByFolderName(ctx context.Context, taskID, folderName string) (Archive, bool, error) {
	row := s.conn.QueryRowContext(ctx, `SELECT `+archiveColumns+`
		FROM archives WHERE task_id = ? AND folder_name = ? AND status = ?
		ORDER BY created_at LIMIT 1`, taskID, folderName, ArchiveOK)
	a, err := scanArchive(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Archive{}, false, nil
	}
	return a, err == nil, err
}

// ListArchives returns archives, optionally filtered by task id ("" = all).
func (s *Store) ListArchives(ctx context.Context, taskID string) ([]Archive, error) {
	query := `SELECT ` + archiveColumns + ` FROM archives`
	var args []any
	if taskID != "" {
		query += ` WHERE task_id = ?`
		args = append(args, taskID)
	}
	query += ` ORDER BY seq`
	rows, err := s.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Archive
	for rows.Next() {
		a, err := scanArchive(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// --- counters ---

// NextSeq atomically increments the named counter and returns the new value.
// On first use the counter is seeded with start.
func (s *Store) NextSeq(ctx context.Context, name string, start int64) (int64, error) {
	var v int64
	err := s.conn.QueryRowContext(ctx, `INSERT INTO counters (name, value) VALUES (?, ?)
		ON CONFLICT(name) DO UPDATE SET value = value + 1
		RETURNING value`, name, start).Scan(&v)
	if err != nil {
		return 0, fmt.Errorf("store: next seq %q: %w", name, err)
	}
	return v, nil
}

// EnsureSeqAtLeast makes sure the named counter is at least min. Used by the
// legacy importer so imported sequence numbers are not reused.
func (s *Store) EnsureSeqAtLeast(ctx context.Context, name string, min int64) error {
	_, err := s.conn.ExecContext(ctx, `INSERT INTO counters (name, value) VALUES (?, ?)
		ON CONFLICT(name) DO UPDATE SET value = MAX(value, excluded.value)`, name, min)
	return err
}
