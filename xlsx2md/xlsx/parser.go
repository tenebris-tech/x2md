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
	Name       string
	Cells      map[int]map[int]Cell
	MaxRow     int
	MaxCol     int
	MinCol     int
	Tables     []Table
	HiddenRows map[int]bool
	HiddenCols map[int]bool
	Merges     []MergeRange
}

type Cell struct {
	Value          string
	Formula        string
	HasFormula     bool
	IsMerged       bool
	IsMergeTopLeft bool
}

type Table struct {
	Name           string
	Ref            string
	StartRow       int
	StartCol       int
	EndRow         int
	EndCol         int
	HeaderRowCount int
}

type MergeRange struct {
	StartRow int
	StartCol int
	EndRow   int
	EndCol   int
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
	Dimension  dimension  `xml:"dimension"`
	Cols       []col      `xml:"cols>col"`
	SheetData  sheetData  `xml:"sheetData"`
	MergeCells mergeCells `xml:"mergeCells"`
	TableParts tableParts `xml:"tableParts"`
}

type dimension struct {
	Ref string `xml:"ref,attr"`
}

type col struct {
	Min    int  `xml:"min,attr"`
	Max    int  `xml:"max,attr"`
	Hidden bool `xml:"hidden,attr"`
}

type sheetData struct {
	Rows []row `xml:"row"`
}

type row struct {
	R      int    `xml:"r,attr"`
	Hidden bool   `xml:"hidden,attr"`
	Cells  []cell `xml:"c"`
}

type cell struct {
	R         string    `xml:"r,attr"`
	T         string    `xml:"t,attr"`
	S         int       `xml:"s,attr"`
	F         *formula  `xml:"f"`
	V         string    `xml:"v"`
	InlineStr inlineStr `xml:"is"`
}

type formula struct {
	Text string `xml:",chardata"`
}

type mergeCells struct {
	Cells []mergeCell `xml:"mergeCell"`
}

type mergeCell struct {
	Ref string `xml:"ref,attr"`
}

type tableParts struct {
	Parts []tablePart `xml:"tablePart"`
}

type tablePart struct {
	RID string `xml:"id,attr"`
}

type tableDefinition struct {
	Name           string `xml:"name,attr"`
	DisplayName    string `xml:"displayName,attr"`
	Ref            string `xml:"ref,attr"`
	HeaderRowCount *int   `xml:"headerRowCount,attr"`
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
		if isOLEEncrypted(data) {
			return nil, fmt.Errorf("encrypted XLSX files are not supported")
		}
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

		parsedSheet, err := readSheet(file, sharedStrings, styles, files)
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

func readSheet(file *zip.File, sharedStrings []string, styles *styleInfo, files map[string]*zip.File) (*Sheet, error) {
	data, err := readZipFile(file)
	if err != nil {
		return nil, err
	}

	var ws worksheet
	if err := unmarshalXLSXXML(data, &ws); err != nil {
		return nil, fmt.Errorf("parsing worksheet: %w", err)
	}

	sheet := &Sheet{
		Cells:      make(map[int]map[int]Cell),
		HiddenRows: make(map[int]bool),
		HiddenCols: make(map[int]bool),
	}

	for _, col := range ws.Cols {
		if !col.Hidden {
			continue
		}
		for colIndex := col.Min; colIndex <= col.Max; colIndex++ {
			sheet.HiddenCols[colIndex] = true
		}
	}

	for _, row := range ws.SheetData.Rows {
		if row.Hidden && row.R > 0 {
			sheet.HiddenRows[row.R] = true
		}

		for _, cell := range row.Cells {
			rowIndex, colIndex := parseCellRef(cell.R)
			if rowIndex == 0 && row.R > 0 {
				rowIndex = row.R
			}
			if rowIndex == 0 || colIndex == 0 {
				continue
			}

			value := resolveCellValue(cell, sharedStrings, styles)

			if sheet.Cells[rowIndex] == nil {
				sheet.Cells[rowIndex] = make(map[int]Cell)
			}
			sheet.Cells[rowIndex][colIndex] = Cell{
				Value:      value,
				Formula:    extractFormula(cell),
				HasFormula: cell.F != nil && strings.TrimSpace(cell.F.Text) != "",
			}

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

	sheet.Merges = parseMergeRanges(ws.MergeCells)
	applyMerges(sheet)

	if len(ws.TableParts.Parts) > 0 {
		tables, err := readTableDefinitions(file.Name, ws.TableParts.Parts, files)
		if err != nil {
			return nil, err
		}
		sheet.Tables = tables
	}

	return sheet, nil
}

func readZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", file.Name, err)
	}
	defer func() { _ = rc.Close() }()

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
	return filepath.ToSlash(filepath.Clean(path))
}

func sheetRelsPath(sheetPath string) string {
	base := strings.TrimPrefix(sheetPath, "xl/worksheets/")
	return filepath.ToSlash(filepath.Join("xl/worksheets/_rels", base+".rels"))
}

func readSheetRels(sheetPath string, files map[string]*zip.File) (map[string]string, error) {
	relsPath := sheetRelsPath(sheetPath)
	file, ok := files[relsPath]
	if !ok {
		return nil, nil
	}

	data, err := readZipFile(file)
	if err != nil {
		return nil, err
	}

	var rels relationships
	if err := unmarshalXLSXXML(data, &rels); err != nil {
		return nil, fmt.Errorf("parsing sheet relationships: %w", err)
	}

	out := make(map[string]string, len(rels.Relationships))
	for _, rel := range rels.Relationships {
		out[rel.ID] = rel.Target
	}
	return out, nil
}

func readTableDefinitions(sheetPath string, parts []tablePart, files map[string]*zip.File) ([]Table, error) {
	rels, err := readSheetRels(sheetPath, files)
	if err != nil {
		return nil, err
	}

	var tables []Table
	for _, part := range parts {
		target := rels[part.RID]
		if target == "" {
			continue
		}
		path := resolveRelationshipTarget(sheetPath, target)
		file, ok := files[path]
		if !ok {
			continue
		}

		data, err := readZipFile(file)
		if err != nil {
			return nil, err
		}

		var def tableDefinition
		if err := unmarshalXLSXXML(data, &def); err != nil {
			return nil, fmt.Errorf("parsing table definition: %w", err)
		}

		startRow, startCol, endRow, endCol := parseRangeRef(def.Ref)
		if startRow == 0 || startCol == 0 || endRow == 0 || endCol == 0 {
			continue
		}

		headerRowCount := 1
		if def.HeaderRowCount != nil {
			headerRowCount = *def.HeaderRowCount
		}

		name := def.Name
		if name == "" {
			name = def.DisplayName
		}

		tables = append(tables, Table{
			Name:           name,
			Ref:            def.Ref,
			StartRow:       startRow,
			StartCol:       startCol,
			EndRow:         endRow,
			EndCol:         endCol,
			HeaderRowCount: headerRowCount,
		})
	}

	return tables, nil
}

func resolveRelationshipTarget(sheetPath string, target string) string {
	trimmed := strings.TrimPrefix(target, "/")
	if strings.HasPrefix(trimmed, "xl/") {
		return filepath.ToSlash(filepath.Clean(trimmed))
	}
	baseDir := filepath.Dir(sheetPath)
	return filepath.ToSlash(filepath.Clean(filepath.Join(baseDir, trimmed)))
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
	case "e":
		return strings.TrimSpace(cell.V)
	case "str":
		return strings.TrimSpace(cell.V)
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
		if isPercentFormat(numFmtID, formatCode) {
			return formatPercent(value, formatCode)
		}
		if isCurrencyFormat(numFmtID, formatCode) {
			return formatCurrency(value, formatCode)
		}
		if isTextFormat(numFmtID, formatCode) {
			return strconv.FormatFloat(value, 'f', -1, 64)
		}
	}

	return strconv.FormatFloat(value, 'f', -1, 64)
}

func isTextFormat(numFmtID int, formatCode string) bool {
	if numFmtID == 49 {
		return true
	}
	return strings.Contains(formatCode, "@")
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

func isPercentFormat(numFmtID int, formatCode string) bool {
	if numFmtID == 9 || numFmtID == 10 {
		return true
	}
	return strings.Contains(formatCode, "%")
}

func isCurrencyFormat(numFmtID int, formatCode string) bool {
	switch numFmtID {
	case 5, 6, 7, 8, 41, 42, 44:
		return true
	}
	return strings.ContainsAny(formatCode, "$€£¥")
}

func formatPercent(value float64, formatCode string) string {
	decimals := countDecimals(formatCode, "%")
	if decimals < 0 {
		decimals = 0
	}
	return fmt.Sprintf("%.*f%%", decimals, value*100)
}

func formatCurrency(value float64, formatCode string) string {
	decimals := countDecimals(formatCode, "")
	if decimals < 0 {
		decimals = 2
	}
	symbol := detectCurrencySymbol(formatCode)
	if symbol == "" {
		symbol = "$"
	}
	return fmt.Sprintf("%s%.*f", symbol, decimals, value)
}

func detectCurrencySymbol(formatCode string) string {
	for _, symbol := range []string{"$", "€", "£", "¥"} {
		if strings.Contains(formatCode, symbol) {
			return symbol
		}
	}
	return ""
}

func countDecimals(formatCode string, stopAt string) int {
	code := formatCode
	if stopAt != "" {
		if idx := strings.Index(code, stopAt); idx >= 0 {
			code = code[:idx]
		}
	}
	if dot := strings.Index(code, "."); dot >= 0 {
		count := 0
		for i := dot + 1; i < len(code); i++ {
			ch := code[i]
			if ch == '0' || ch == '#' {
				count++
				continue
			}
			break
		}
		return count
	}
	return 0
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

func extractFormula(cell cell) string {
	if cell.F == nil {
		return ""
	}
	return strings.TrimSpace(cell.F.Text)
}

func isOLEEncrypted(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	oleHeader := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	return bytes.Equal(data[:8], oleHeader)
}

func parseMergeRanges(mergeCells mergeCells) []MergeRange {
	var ranges []MergeRange
	for _, cell := range mergeCells.Cells {
		startRow, startCol, endRow, endCol := parseRangeRef(cell.Ref)
		if startRow == 0 || startCol == 0 || endRow == 0 || endCol == 0 {
			continue
		}
		ranges = append(ranges, MergeRange{
			StartRow: startRow,
			StartCol: startCol,
			EndRow:   endRow,
			EndCol:   endCol,
		})
	}
	return ranges
}

func applyMerges(sheet *Sheet) {
	for _, merge := range sheet.Merges {
		for row := merge.StartRow; row <= merge.EndRow; row++ {
			if sheet.Cells[row] == nil {
				sheet.Cells[row] = make(map[int]Cell)
			}
			for col := merge.StartCol; col <= merge.EndCol; col++ {
				cell := sheet.Cells[row][col]
				cell.IsMerged = true
				if row == merge.StartRow && col == merge.StartCol {
					cell.IsMergeTopLeft = true
				}
				sheet.Cells[row][col] = cell
			}
		}
	}
}

func parseRangeRef(ref string) (startRow, startCol, endRow, endCol int) {
	parts := strings.Split(ref, ":")
	if len(parts) == 0 {
		return 0, 0, 0, 0
	}
	startRow, startCol = parseCellRef(parts[0])
	if len(parts) == 1 {
		return startRow, startCol, startRow, startCol
	}
	endRow, endCol = parseCellRef(parts[1])
	return startRow, startCol, endRow, endCol
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
