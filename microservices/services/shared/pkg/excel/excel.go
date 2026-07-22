// Package excel 提供 xlsx 导出 / 导入的轻量封装（excelize StreamWriter，
// 内存占用有界），供各服务的列表导出、批量导入端点复用。
//
// 约定：
//   - 导出：NewSheet 建表头 → AppendRow 逐行 → WriteHTTP 落响应（自动设
//     Content-Type / Content-Disposition，文件名做 RFC 5987 转义）。
//   - 导入：ReadFirstSheet 读第一张工作表为字符串矩阵（含表头行），
//     行数上限由调用方给定，防御异常大文件。
package excel

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/xuri/excelize/v2"
)

// Sheet 导出中的单表工作簿。
type Sheet struct {
	file  *excelize.File
	sw    *excelize.StreamWriter
	sheet string
	row   int
}

// NewSheet 新建单表工作簿并写入表头（widths 可为 nil=默认列宽；
// 长度与 headers 不一致时多余部分忽略）。
func NewSheet(sheetName string, headers []string, widths []float64) (*Sheet, error) {
	if sheetName == "" {
		sheetName = "Sheet1"
	}
	if len(headers) == 0 {
		return nil, errors.New("表头不能为空")
	}
	f := excelize.NewFile()
	if sheetName != "Sheet1" {
		if err := f.SetSheetName("Sheet1", sheetName); err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	sw, err := f.NewStreamWriter(sheetName)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	for i, w := range widths {
		if i >= len(headers) {
			break
		}
		if w > 0 {
			if err := sw.SetColWidth(i+1, i+1, w); err != nil {
				_ = f.Close()
				return nil, err
			}
		}
	}
	styleID, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	headerCells := make([]any, len(headers))
	for i, h := range headers {
		headerCells[i] = excelize.Cell{Value: h, StyleID: styleID}
	}
	s := &Sheet{file: f, sw: sw, sheet: sheetName, row: 1}
	if err := s.appendCells(headerCells); err != nil {
		_ = f.Close()
		return nil, err
	}
	return s, nil
}

// AppendRow 追加一行数据（值类型由 excelize 自行判别：字符串/数字/时间等）。
func (s *Sheet) AppendRow(values ...any) error {
	return s.appendCells(values)
}

func (s *Sheet) appendCells(values []any) error {
	cell, err := excelize.CoordinatesToCellName(1, s.row)
	if err != nil {
		return err
	}
	if err := s.sw.SetRow(cell, values); err != nil {
		return err
	}
	s.row++
	return nil
}

// WriteHTTP 收尾并把工作簿写入 HTTP 响应（附下载头）。调用后 Sheet 不可复用。
func (s *Sheet) WriteHTTP(w http.ResponseWriter, filename string) error {
	defer func() { _ = s.file.Close() }()
	if err := s.sw.Flush(); err != nil {
		return err
	}
	if filename == "" {
		filename = fmt.Sprintf("export_%s.xlsx", time.Now().Format("20060102150405"))
	}
	w.Header().Set("Content-Type",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition",
		mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	return s.file.Write(w)
}

// ReadFirstSheet 读第一张工作表为字符串矩阵（含表头行）。
// maxRows 为数据行上限（<=0 取 2000）；超限返回错误而非静默截断。
func ReadFirstSheet(r io.Reader, maxRows int) ([][]string, error) {
	if maxRows <= 0 {
		maxRows = 2000
	}
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("解析 Excel 失败（仅支持 .xlsx）: %w", err)
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("文件中没有工作表")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, err
	}
	if len(rows) > maxRows+1 { // +1 表头行
		return nil, fmt.Errorf("数据超过 %d 行上限，请分批导入", maxRows)
	}
	return rows, nil
}

// Cell 读单元格（越界返回空串），导入解析用的防御取值。
func Cell(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return row[idx]
}
