package xlsx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Workbook struct {
	Sheets []*Sheet
}

type Sheet struct {
	Name   string
	Cells  map[int]map[int]string
	MaxRow int
	MaxCol int
	MinCol int
}

type workbook struct {
	Sheets []workbookSheet `xml:"sheets>sheet"`
}

type workbookSheet struct {
	Name string `xml:"name,attr"`
	ID   string `xml:"id,attr"`
}

type relationships struct {
	Relationships []relationship `xml:"Relationship"`
}

type relationship struct {
	ID     string `xml:"Id,attr"`
	Target string `xml:"Target,attr"`
}

type worksheet struct {
	SheetData sheetData `xml:"sheetData"`
}

type sheetData struct {
	Rows []row `xml:"row"`
}

type row struct {
	R     int    `xml:"r,attr"`
	Cells []cell `xml:"c"`
}

type cell struct {
	R         string    `xml:"r,attr"`
	T         string    `xml:"t,attr"`
	S         int       `xml:"s,attr"`
	V         string    `xml:"v"`
	InlineStr inlineStr `xml:"is"`
}

type inlineStr struct {
	T []string   `xml:"t"`
	R []richText `xml:"r"`
}

type sharedStrings struct {
	Strings []sharedString `xml:"si"`
}

type sharedString struct {
	T []string   `xml:"t"`
	R []richText `xml:"r"`
}

type richText struct {
	T string `xml:"t"`
}

type styleSheet struct {
	NumFmts []numFmt `xml:"numFmts>numFmt"`
	CellXfs []cellXf `xml:"cellXfs>xf"`
}

type numFmt struct {
	NumFmtID   int    `xml:"numFmtId,attr"`
	FormatCode string `xml:"formatCode,attr"`
}

type cellXf struct {
	NumFmtID int `xml:"numFmtId,attr"`
}

type styleInfo struct {
	NumFmtByStyle    []int
	FormatCodeByID   map[int]string
	HasCustomFormats bool
}

func Parse(data []byte) (*Workbook, error) {
	reader := bytes.NewReader(data)
	zipReader, err := zip.NewReader(reader, int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("opening XLSX archive: %w", err)
	}

	files := make(map[string]*zip.File)
	for _, f := range zipReader.File {
		files[f.Name] = f
	}

	if _, ok := files["xl/workbook.xml"]; !ok {
		return nil, fmt.Errorf("not a valid XLSX file: missing xl/workbook.xml")
	}

	sharedStrings, err := readSharedStrings(files)
	if err != nil {
		return nil, err
	}

	styles, err := readStyles(files)
	if err != nil {
		return nil, err
	}

	workbookData, err := readWorkbook(files)
	if err != nil {
		return nil, err
	}

	rels, err := readWorkbookRels(files)
	if err != nil {
		return nil, err
	}

	var sheets []*Sheet
	for _, sheet := range workbookData.Sheets {
		target := rels[sheet.ID]
		if target == "" {
			continue
		}

		path := normalizeTargetPath(target)
		file, ok := files[path]
		if !ok {
			continue
		}

		parsedSheet, err := readSheet(file, sharedStrings, styles)
		if err != nil {
			return nil, err
		}
		parsedSheet.Name = sheet.Name
		sheets = append(sheets, parsedSheet)
	}

	return &Workbook{Sheets: sheets}, nil
}

func readSharedStrings(files map[string]*zip.File) ([]string, error) {
	file, ok := files["xl/sharedStrings.xml"]
	if !ok {
		return nil, nil
	}

	data, err := readZipFile(file)
	if err != nil {
		return nil, err
	}

	var sst sharedStrings
	if err := unmarshalXLSXXML(data, &sst); err != nil {
		return nil, fmt.Errorf("parsing shared strings: %w", err)
	}

	stringsOut := make([]string, 0, len(sst.Strings))
	for _, s := range sst.Strings {
		stringsOut = append(stringsOut, extractRichText(s.T, s.R))
	}

	return stringsOut, nil
}

func readWorkbook(files map[string]*zip.File) (*workbook, error) {
	file, ok := files["xl/workbook.xml"]
	if !ok {
		return nil, fmt.Errorf("workbook.xml not found")
	}

	data, err := readZipFile(file)
	if err != nil {
		return nil, err
	}

	var wb workbook
	if err := unmarshalXLSXXML(data, &wb); err != nil {
		return nil, fmt.Errorf("parsing workbook: %w", err)
	}

	return &wb, nil
}

func readWorkbookRels(files map[string]*zip.File) (map[string]string, error) {
	file, ok := files["xl/_rels/workbook.xml.rels"]
	if !ok {
		return nil, fmt.Errorf("workbook relationships not found")
	}

	data, err := readZipFile(file)
	if err != nil {
		return nil, err
	}

	var rels relationships
	if err := unmarshalXLSXXML(data, &rels); err != nil {
		return nil, fmt.Errorf("parsing workbook relationships: %w", err)
	}

	out := make(map[string]string, len(rels.Relationships))
	for _, rel := range rels.Relationships {
		out[rel.ID] = rel.Target
	}
	return out, nil
}

func readStyles(files map[string]*zip.File) (*styleInfo, error) {
	file, ok := files["xl/styles.xml"]
	if !ok {
		return nil, nil
	}

	data, err := readZipFile(file)
	if err != nil {
		return nil, err
	}

	var styles styleSheet
	if err := unmarshalXLSXXML(data, &styles); err != nil {
		return nil, fmt.Errorf("parsing styles: %w", err)
	}

	info := &styleInfo{
		NumFmtByStyle:  make([]int, len(styles.CellXfs)),
		FormatCodeByID: make(map[int]string),
	}

	for _, fmtDef := range styles.NumFmts {
		info.FormatCodeByID[fmtDef.NumFmtID] = fmtDef.FormatCode
		info.HasCustomFormats = true
	}

	for i, xf := range styles.CellXfs {
		info.NumFmtByStyle[i] = xf.NumFmtID
	}

	return info, nil
}

func readSheet(file *zip.File, sharedStrings []string, styles *styleInfo) (*Sheet, error) {
	data, err := readZipFile(file)
	if err != nil {
		return nil, err
	}

	var ws worksheet
	if err := unmarshalXLSXXML(data, &ws); err != nil {
		return nil, fmt.Errorf("parsing worksheet: %w", err)
	}

	sheet := &Sheet{
		Cells: make(map[int]map[int]string),
	}

	for _, row := range ws.SheetData.Rows {
		for _, cell := range row.Cells {
			rowIndex, colIndex := parseCellRef(cell.R)
			if rowIndex == 0 && row.R > 0 {
				rowIndex = row.R
			}
			if rowIndex == 0 || colIndex == 0 {
				continue
			}

			value := resolveCellValue(cell, sharedStrings, styles)
			if value == "" {
				continue
			}

			if sheet.Cells[rowIndex] == nil {
				sheet.Cells[rowIndex] = make(map[int]string)
			}
			sheet.Cells[rowIndex][colIndex] = value

			if rowIndex > sheet.MaxRow {
				sheet.MaxRow = rowIndex
			}
			if colIndex > sheet.MaxCol {
				sheet.MaxCol = colIndex
			}
			if sheet.MinCol == 0 || colIndex < sheet.MinCol {
				sheet.MinCol = colIndex
			}
		}
	}

	return sheet, nil
}

func readZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", file.Name, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file.Name, err)
	}

	return data, nil
}

func normalizeTargetPath(target string) string {
	path := strings.TrimPrefix(target, "/")
	if !strings.HasPrefix(path, "xl/") {
		path = filepath.ToSlash(filepath.Join("xl", path))
	}
	return path
}

func resolveCellValue(cell cell, sharedStrings []string, styles *styleInfo) string {
	switch cell.T {
	case "s":
		idx, err := strconv.Atoi(strings.TrimSpace(cell.V))
		if err == nil && idx >= 0 && idx < len(sharedStrings) {
			return sharedStrings[idx]
		}
	case "inlineStr":
		if inline := extractRichText(cell.InlineStr.T, cell.InlineStr.R); inline != "" {
			return inline
		}
	case "b":
		if strings.TrimSpace(cell.V) == "1" {
			return "TRUE"
		}
		return "FALSE"
	}

	if cell.V != "" {
		return formatNumericValue(cell.V, cell.S, styles)
	}

	if inline := extractRichText(cell.InlineStr.T, cell.InlineStr.R); inline != "" {
		return inline
	}

	return ""
}

func formatNumericValue(raw string, styleIndex int, styles *styleInfo) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return trimmed
	}

	if styles != nil && styleIndex >= 0 && styleIndex < len(styles.NumFmtByStyle) {
		numFmtID := styles.NumFmtByStyle[styleIndex]
		formatCode := styles.FormatCodeByID[numFmtID]
		if isDateFormat(numFmtID, formatCode) {
			return formatExcelDate(value, formatCode)
		}
	}

	return strconv.FormatFloat(value, 'f', -1, 64)
}

func isDateFormat(numFmtID int, formatCode string) bool {
	switch numFmtID {
	case 14, 15, 16, 17, 18, 19, 20, 21, 22,
		27, 28, 29, 30, 31, 32, 33, 34, 35, 36,
		45, 46, 47, 50, 51, 52, 53, 54, 55, 56, 57, 58:
		return true
	}

	if formatCode == "" {
		return false
	}

	code := strings.ToLower(formatCode)
	code = strings.ReplaceAll(code, "\\", "")
	code = strings.ReplaceAll(code, "\"", "")
	return strings.ContainsAny(code, "ymdhis")
}

func formatExcelDate(serial float64, formatCode string) string {
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	days := int(serial)
	frac := serial - float64(days)
	if frac < 0 {
		frac = 0
	}

	timestamp := base.AddDate(0, 0, days).Add(time.Duration(frac * 24 * float64(time.Hour)))

	if hasTimeComponent(serial, formatCode) {
		return timestamp.Format("2006-01-02 15:04:05")
	}
	return timestamp.Format("2006-01-02")
}

func hasTimeComponent(serial float64, formatCode string) bool {
	if serial != float64(int(serial)) {
		return true
	}
	if formatCode == "" {
		return false
	}
	code := strings.ToLower(formatCode)
	code = strings.ReplaceAll(code, "\\", "")
	code = strings.ReplaceAll(code, "\"", "")
	return strings.ContainsAny(code, "hs")
}

func extractRichText(values []string, runs []richText) string {
	if len(runs) > 0 {
		var b strings.Builder
		for _, run := range runs {
			b.WriteString(run.T)
		}
		return b.String()
	}

	if len(values) > 0 {
		return strings.Join(values, "")
	}

	return ""
}

func parseCellRef(ref string) (row int, col int) {
	if ref == "" {
		return 0, 0
	}

	for i := 0; i < len(ref); i++ {
		if ref[i] >= '0' && ref[i] <= '9' {
			col = columnLettersToIndex(ref[:i])
			row, _ = strconv.Atoi(ref[i:])
			return row, col
		}
	}

	return 0, 0
}

func columnLettersToIndex(letters string) int {
	letters = strings.ToUpper(letters)
	col := 0
	for i := 0; i < len(letters); i++ {
		ch := letters[i]
		if ch < 'A' || ch > 'Z' {
			continue
		}
		col = col*26 + int(ch-'A'+1)
	}
	return col
}
