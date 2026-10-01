// Package export writes archive ledgers to spreadsheet files.
package export

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// ArchiveRow is one exported ledger line.
type ArchiveRow struct {
	Seq         int64
	FolderName  string
	ArchivePath string
	Password    string
	Size        int64
	FileCount   int
	CreatedAt   string
}

var headers = []string{"序号", "文件夹", "归档路径", "密码", "大小(字节)", "文件数", "创建时间"}

// WriteArchives writes rows to an .xlsx file at path.
func WriteArchives(path string, rows []ArchiveRow) error {
	f := excelize.NewFile()
	defer f.Close()

	const sheet = "归档记录"
	index, err := f.NewSheet(sheet)
	if err != nil {
		return fmt.Errorf("export: new sheet: %w", err)
	}
	f.SetActiveSheet(index)
	_ = f.DeleteSheet("Sheet1")

	headerStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return err
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return err
		}
	}
	if err := f.SetCellStyle(sheet, "A1", "G1", headerStyle); err != nil {
		return err
	}

	for r, row := range rows {
		line := r + 2
		values := []any{row.Seq, row.FolderName, row.ArchivePath, row.Password, row.Size, row.FileCount, row.CreatedAt}
		for col, v := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, line)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return err
			}
		}
	}

	widths := map[string]float64{"A": 8, "B": 24, "C": 52, "D": 24, "E": 14, "F": 10, "G": 20}
	for col, w := range widths {
		if err := f.SetColWidth(sheet, col, col, w); err != nil {
			return err
		}
	}

	if err := f.SaveAs(path); err != nil {
		return fmt.Errorf("export: save: %w", err)
	}
	return nil
}
