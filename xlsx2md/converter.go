// Package xlsx2md provides a pure Go library to convert XLSX files to Markdown
package xlsx2md

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tenebris-tech/x2md/xlsx2md/xlsx"
)

// Converter is the main XLSX to Markdown converter
type Converter struct {
	options *Options
}

// Options holds configuration for the converter
type Options struct {
	// IncludeSheetNames adds a heading before each sheet
	IncludeSheetNames bool

	// SheetSeparator is inserted between sheets
	SheetSeparator string

	// SkipEmptyRows removes rows with no cell values
	SkipEmptyRows bool

	// IncludeHidden controls whether hidden rows/columns are emitted
	IncludeHidden bool

	// MarkHidden appends a marker to hidden row/column labels
	MarkHidden bool

	// ShowFormulas displays cell formulas alongside values when true
	// When false, only the calculated values are shown
	ShowFormulas bool

	// Compact removes excessive blank lines from the output
	Compact bool

	// OnSheetParsed is called when a sheet is parsed
	OnSheetParsed func(name string, rows, cols int)
}

// Option is a functional option for configuring the converter
type Option func(*Options)

// DefaultOptions returns the default options
func DefaultOptions() *Options {
	return &Options{
		IncludeSheetNames: true,
		SheetSeparator:    "\n\n",
		SkipEmptyRows:     true,
		IncludeHidden:     true,
		MarkHidden:        true,
		ShowFormulas:      true,
	}
}

// WithIncludeSheetNames sets whether to include sheet headings
func WithIncludeSheetNames(include bool) Option {
	return func(o *Options) {
		o.IncludeSheetNames = include
	}
}

// WithIncludeHidden sets whether to include hidden rows/columns
func WithIncludeHidden(include bool) Option {
	return func(o *Options) {
		o.IncludeHidden = include
	}
}

// WithOnSheetParsed sets the callback for sheet parsing
func WithOnSheetParsed(callback func(name string, rows, cols int)) Option {
	return func(o *Options) {
		o.OnSheetParsed = callback
	}
}

// WithShowFormulas sets whether to display formulas alongside cell values
func WithShowFormulas(show bool) Option {
	return func(o *Options) {
		o.ShowFormulas = show
	}
}

// WithCompact removes excessive blank lines from the output.
func WithCompact(compact bool) Option {
	return func(o *Options) {
		o.Compact = compact
	}
}

// New creates a new Converter with the given options
func New(opts ...Option) *Converter {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	return &Converter{options: options}
}

// ConvertFile converts an XLSX file to Markdown
func (c *Converter) ConvertFile(inputPath string) (string, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}
	return c.Convert(data)
}

// ConvertFileToFile converts an XLSX file and writes the result to a file
func (c *Converter) ConvertFileToFile(inputPath, outputPath string) error {
	markdown, err := c.ConvertFile(inputPath)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(markdown), 0644)
}

// Convert converts XLSX data to Markdown
func (c *Converter) Convert(data []byte) (string, error) {
	workbook, err := xlsx.Parse(data)
	if err != nil {
		return "", err
	}

	var output strings.Builder
	for sheetIndex, sheet := range workbook.Sheets {
		if sheetIndex > 0 {
			output.WriteString(c.options.SheetSeparator)
		}

		if c.options.IncludeSheetNames && sheet.Name != "" {
			output.WriteString(fmt.Sprintf("## %s\n\n", sheet.Name))
		}

		if sheet.MaxRow == 0 || sheet.MaxCol == 0 {
			continue
		}

		if c.options.OnSheetParsed != nil {
			outputCols := sheet.MaxCol
			if sheet.MinCol > 0 {
				outputCols = sheet.MaxCol - sheet.MinCol + 1
			}
			c.options.OnSheetParsed(sheet.Name, sheet.MaxRow, outputCols)
		}

		blocks := buildRangeBlocks(sheet)
		for blockIndex, block := range blocks {
			if blockIndex > 0 {
				output.WriteString("\n")
			}

			blockTitle := formatBlockTitle(sheet.Name, block)
			if blockTitle != "" {
				output.WriteString(blockTitle)
				output.WriteString("\n")
			}

			cols := visibleColumns(block, sheet, c.options.IncludeHidden)
			rows := visibleRows(block, sheet, c.options.IncludeHidden, cols, c.options.SkipEmptyRows)
			if len(cols) == 0 || len(rows) == 0 {
				continue
			}

			headers := append([]string{"Row"}, columnLabels(cols, sheet, c.options.MarkHidden)...)
			writeMarkdownRow(&output, headers)
			writeSeparatorRow(&output, len(headers))

			for _, row := range rows {
				rowLabel := formatRowLabel(row, sheet, c.options.MarkHidden)
				values := []string{rowLabel}
				for _, col := range cols {
					values = append(values, cellDisplay(sheet, row, col, c.options.ShowFormulas))
				}
				writeMarkdownRow(&output, values)
			}
		}
	}

	markdown := output.String()

	// Apply compact formatting if enabled
	if c.options.Compact {
		markdown = compactMarkdown(markdown)
	}

	return markdown, nil
}

// compactMarkdown reduces excessive blank lines in markdown.
func compactMarkdown(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for {
		newS := strings.ReplaceAll(s, "\n\n\n", "\n\n")
		if newS == s {
			break
		}
		s = newS
	}
	return strings.TrimSpace(s) + "\n"
}

type rangeBlock struct {
	Kind     string
	Name     string
	StartRow int
	StartCol int
	EndRow   int
	EndCol   int
}

func buildRangeBlocks(sheet *xlsx.Sheet) []rangeBlock {
	var blocks []rangeBlock
	for _, table := range sheet.Tables {
		blocks = append(blocks, rangeBlock{
			Kind:     "table",
			Name:     table.Name,
			StartRow: table.StartRow,
			StartCol: table.StartCol,
			EndRow:   table.EndRow,
			EndCol:   table.EndCol,
		})
	}

	tableRows := map[int]bool{}
	for _, table := range sheet.Tables {
		for row := table.StartRow; row <= table.EndRow; row++ {
			tableRows[row] = true
		}
	}

	var inBlock bool
	startRow := 0
	for row := 1; row <= sheet.MaxRow; row++ {
		if tableRows[row] || !rowHasAnyData(sheet, row) {
			if inBlock {
				minCol, maxCol := rangeColumnBounds(sheet, startRow, row-1)
				if minCol != 0 && maxCol != 0 {
					blocks = append(blocks, rangeBlock{
						Kind:     "range",
						StartRow: startRow,
						StartCol: minCol,
						EndRow:   row - 1,
						EndCol:   maxCol,
					})
				}
				inBlock = false
			}
			continue
		}

		if !inBlock {
			startRow = row
			inBlock = true
		}
	}

	if inBlock {
		minCol, maxCol := rangeColumnBounds(sheet, startRow, sheet.MaxRow)
		if minCol != 0 && maxCol != 0 {
			blocks = append(blocks, rangeBlock{
				Kind:     "range",
				StartRow: startRow,
				StartCol: minCol,
				EndRow:   sheet.MaxRow,
				EndCol:   maxCol,
			})
		}
	}

	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].StartRow != blocks[j].StartRow {
			return blocks[i].StartRow < blocks[j].StartRow
		}
		return blocks[i].StartCol < blocks[j].StartCol
	})

	return blocks
}

func formatBlockTitle(sheetName string, block rangeBlock) string {
	rangeRef := formatRangeRef(block.StartRow, block.StartCol, block.EndRow, block.EndCol)
	if block.Kind == "table" {
		if block.Name != "" {
			return fmt.Sprintf("### Table: %s!%s (%s)", sheetName, rangeRef, block.Name)
		}
		return fmt.Sprintf("### Table: %s!%s", sheetName, rangeRef)
	}
	return fmt.Sprintf("### Range: %s!%s", sheetName, rangeRef)
}

func visibleColumns(block rangeBlock, sheet *xlsx.Sheet, includeHidden bool) []int {
	var cols []int
	for col := block.StartCol; col <= block.EndCol; col++ {
		if !includeHidden && sheet.HiddenCols[col] {
			continue
		}
		cols = append(cols, col)
	}
	return cols
}

func visibleRows(block rangeBlock, sheet *xlsx.Sheet, includeHidden bool, cols []int, skipEmpty bool) []int {
	var rows []int
	for row := block.StartRow; row <= block.EndRow; row++ {
		if !includeHidden && sheet.HiddenRows[row] {
			continue
		}
		if skipEmpty && rowIsEmpty(sheet, row, cols) {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func rowHasAnyData(sheet *xlsx.Sheet, row int) bool {
	if sheet.Cells[row] == nil {
		return false
	}
	for _, cell := range sheet.Cells[row] {
		if strings.TrimSpace(cell.Value) != "" || cell.HasFormula {
			return true
		}
	}
	return false
}

func rangeColumnBounds(sheet *xlsx.Sheet, startRow int, endRow int) (int, int) {
	minCol := 0
	maxCol := 0
	for row := startRow; row <= endRow; row++ {
		for col, cell := range sheet.Cells[row] {
			if strings.TrimSpace(cell.Value) == "" && !cell.HasFormula {
				if !cell.IsMerged {
					continue
				}
			}
			if minCol == 0 || col < minCol {
				minCol = col
			}
			if col > maxCol {
				maxCol = col
			}
		}
	}
	for _, merge := range sheet.Merges {
		if merge.EndRow < startRow || merge.StartRow > endRow {
			continue
		}
		if minCol == 0 || merge.StartCol < minCol {
			minCol = merge.StartCol
		}
		if merge.EndCol > maxCol {
			maxCol = merge.EndCol
		}
	}
	return minCol, maxCol
}

func rowIsEmpty(sheet *xlsx.Sheet, row int, cols []int) bool {
	for _, col := range cols {
		cell := sheet.Cells[row][col]
		if strings.TrimSpace(cell.Value) != "" || cell.HasFormula || cell.IsMerged {
			return false
		}
	}
	return true
}

func columnLabels(cols []int, sheet *xlsx.Sheet, markHidden bool) []string {
	labels := make([]string, 0, len(cols))
	for _, col := range cols {
		label := columnIndexToLetters(col)
		if markHidden && sheet.HiddenCols[col] {
			label = fmt.Sprintf("%s [hidden]", label)
		}
		labels = append(labels, label)
	}
	return labels
}

func formatRowLabel(row int, sheet *xlsx.Sheet, markHidden bool) string {
	label := fmt.Sprintf("%d", row)
	if markHidden && sheet.HiddenRows[row] {
		label = fmt.Sprintf("%s [hidden]", label)
	}
	return label
}

func cellDisplay(sheet *xlsx.Sheet, row int, col int, showFormulas bool) string {
	cell, ok := sheet.Cells[row][col]
	if !ok {
		return ""
	}

	value := strings.TrimSpace(cell.Value)
	if showFormulas && cell.HasFormula {
		formula := strings.TrimSpace(cell.Formula)
		if formula != "" {
			formula = "=" + formula
			if value != "" {
				value = fmt.Sprintf("%s (%s)", value, formula)
			} else {
				value = formula
			}
		}
	}

	if value == "" && cell.IsMerged && !cell.IsMergeTopLeft {
		value = "[merged]"
	}

	return sanitizeCell(value)
}

func formatRangeRef(startRow int, startCol int, endRow int, endCol int) string {
	start := fmt.Sprintf("%s%d", columnIndexToLetters(startCol), startRow)
	end := fmt.Sprintf("%s%d", columnIndexToLetters(endCol), endRow)
	if start == end {
		return start
	}
	return fmt.Sprintf("%s:%s", start, end)
}

func columnIndexToLetters(col int) string {
	if col <= 0 {
		return ""
	}
	var b strings.Builder
	for col > 0 {
		col--
		b.WriteByte(byte('A' + (col % 26)))
		col /= 26
	}

	runes := []rune(b.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func writeMarkdownRow(output *strings.Builder, values []string) {
	output.WriteString("|")
	for _, value := range values {
		output.WriteString(" ")
		output.WriteString(value)
		output.WriteString(" |")
	}
	output.WriteString("\n")
}

func writeSeparatorRow(output *strings.Builder, cols int) {
	output.WriteString("|")
	for i := 0; i < cols; i++ {
		output.WriteString(" --- |")
	}
	output.WriteString("\n")
}

func sanitizeCell(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\r", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\n", "<br>")
	trimmed = strings.ReplaceAll(trimmed, "|", "\\|")
	return trimmed
}
