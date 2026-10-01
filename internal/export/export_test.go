package export

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestWriteArchives(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.xlsx")
	rows := []ArchiveRow{
		{Seq: 10000, FolderName: "资料", ArchivePath: "D:/out/10000.7z", Password: "abc123", Size: 1234, FileCount: 5, CreatedAt: "2026-10-01 12:00:00"},
		{Seq: 10001, FolderName: "game", ArchivePath: "D:/out/10001.7z", Password: "xyz", Size: 999, FileCount: 2, CreatedAt: "2026-10-01 12:01:00"},
	}
	if err := WriteArchives(path, rows); err != nil {
		t.Fatalf("WriteArchives: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()

	const sheet = "归档记录"
	if got, _ := f.GetCellValue(sheet, "A1"); got != "序号" {
		t.Errorf("A1 = %q, want 序号", got)
	}
	if got, _ := f.GetCellValue(sheet, "D2"); got != "abc123" {
		t.Errorf("D2 = %q, want abc123", got)
	}
	if got, _ := f.GetCellValue(sheet, "B3"); got != "game" {
		t.Errorf("B3 = %q, want game", got)
	}
	if got, _ := f.GetCellValue(sheet, "C3"); got != "D:/out/10001.7z" {
		t.Errorf("C3 = %q", got)
	}
}
