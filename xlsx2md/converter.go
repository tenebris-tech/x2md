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

	// SkipEmptyRows removes rows with no cell values (excluding header)
	SkipEmptyRows bool

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
	}
}

// WithIncludeSheetNames sets whether to include sheet headings
func WithIncludeSheetNames(include bool) Option {
	return func(o *Options) {
		o.IncludeSheetNames = include
	}
}

// WithSheetSeparator sets the separator between sheets
func WithSheetSeparator(separator string) Option {
	return func(o *Options) {
		o.SheetSeparator = separator
	}
}

// WithSkipEmptyRows sets whether to omit empty rows
func WithSkipEmptyRows(skip bool) Option {
	return func(o *Options) {
		o.SkipEmptyRows = skip
	}
}

// WithOnSheetParsed sets the callback for sheet parsing
func WithOnSheetParsed(callback func(name string, rows, cols int)) Option {
	return func(o *Options) {
		o.OnSheetParsed = callback
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

		if sheet.MaxRow == 0 || sheet.MaxCol == 0 || sheet.MinCol == 0 {
			continue
		}

		if c.options.OnSheetParsed != nil {
			c.options.OnSheetParsed(sheet.Name, sheet.MaxRow, sheet.MaxCol-sheet.MinCol+1)
		}

		blocks := rowBlocks(sheet)
		for blockIndex, block := range blocks {
			if blockIndex > 0 {
				output.WriteString("\n")
			}

			headerRow := headerRowWithMostData(sheet, block)
			if headerRow == 0 {
				continue
			}

			blockMinCol, blockMaxCol := blockColumnRange(sheet, block)
			if blockMinCol == 0 || blockMaxCol == 0 {
				continue
			}

			if block.MaxRow == block.MinRow && rowDataCount(sheet, block.MinRow) < 2 {
				text := rowText(sheet, block.MinRow)
				if text != "" {
					output.WriteString(text)
					output.WriteString("\n")
				}
				continue
			}

			for row := block.MinRow; row < headerRow; row++ {
				text := rowText(sheet, row)
				if text == "" {
					continue
				}
				output.WriteString(text)
				output.WriteString("\n")
			}

			headerValues := rowValues(sheet, headerRow, blockMinCol, blockMaxCol)
			writeMarkdownRow(&output, headerValues)
			writeSeparatorRow(&output, len(headerValues))

			for row := headerRow + 1; row <= block.MaxRow; row++ {
				values := rowValues(sheet, row, blockMinCol, blockMaxCol)
				if c.options.SkipEmptyRows && isRowEmpty(values) {
					continue
				}
				writeMarkdownRow(&output, values)
			}
		}
	}

	return output.String(), nil
}

type rowBlock struct {
	MinRow int
	MaxRow int
}

func rowBlocks(sheet *xlsx.Sheet) []rowBlock {
	var blocks []rowBlock
	inBlock := false
	startRow := 0

	for row := 1; row <= sheet.MaxRow; row++ {
		if rowHasData(sheet, row) {
			if !inBlock {
				inBlock = true
				startRow = row
			}
			continue
		}

		if inBlock {
			blocks = append(blocks, rowBlock{MinRow: startRow, MaxRow: row - 1})
			inBlock = false
		}
	}

	if inBlock {
		blocks = append(blocks, rowBlock{MinRow: startRow, MaxRow: sheet.MaxRow})
	}

	return blocks
}

func rowHasData(sheet *xlsx.Sheet, row int) bool {
	return rowDataCount(sheet, row) > 0
}

func rowDataCount(sheet *xlsx.Sheet, row int) int {
	if sheet.Cells[row] == nil {
		return 0
	}
	return len(sheet.Cells[row])
}

func headerRowWithMostData(sheet *xlsx.Sheet, block rowBlock) int {
	bestRow := 0
	bestCount := 0
	for row := block.MinRow; row <= block.MaxRow; row++ {
		count := rowDataCount(sheet, row)
		if count == 0 {
			continue
		}
		if count > bestCount || (count == bestCount && (bestRow == 0 || row < bestRow)) {
			bestRow = row
			bestCount = count
		}
	}
	return bestRow
}

func blockColumnRange(sheet *xlsx.Sheet, block rowBlock) (int, int) {
	minCol := 0
	maxCol := 0
	for row := block.MinRow; row <= block.MaxRow; row++ {
		for col := range sheet.Cells[row] {
			if minCol == 0 || col < minCol {
				minCol = col
			}
			if col > maxCol {
				maxCol = col
			}
		}
	}
	return minCol, maxCol
}

func rowValues(sheet *xlsx.Sheet, row int, minCol int, maxCol int) []string {
	values := make([]string, maxCol-minCol+1)
	for col := minCol; col <= maxCol; col++ {
		if sheet.Cells[row] != nil {
			values[col-minCol] = sanitizeCell(sheet.Cells[row][col])
		}
	}
	return values
}

func rowText(sheet *xlsx.Sheet, row int) string {
	if sheet.Cells[row] == nil {
		return ""
	}
	var cols []int
	for col := range sheet.Cells[row] {
		cols = append(cols, col)
	}
	sort.Ints(cols)

	var parts []string
	for _, col := range cols {
		value := sanitizeCell(sheet.Cells[row][col])
		if value == "" {
			continue
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, " ")
}

func isRowEmpty(values []string) bool {
	for _, v := range values {
		if v != "" {
			return false
		}
	}
	return true
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
