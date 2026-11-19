/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// End-to-end tests: build the CLI, run it on the fixtures in ../testdata and
// check that a passage in the Markdown can be traced back to its place in
// the original document. That is the contract a viewer relies on to open
// the original at the right spot.

var x2mdBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "x2md-cli-test")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	x2mdBinary = filepath.Join(dir, "x2md")
	if out, err := exec.Command("go", "build", "-o", x2mdBinary, ".").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building CLI: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// runCLI runs the CLI on a fixture and returns the Markdown it wrote
func runCLI(t *testing.T, fixture string, flags ...string) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "out.md")
	args := append(append([]string{}, flags...), "-output", output, filepath.Join("..", "testdata", fixture))
	if out, err := exec.Command(x2mdBinary, args...).CombinedOutput(); err != nil {
		t.Fatalf("x2md %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	return string(data)
}

var locationMarker = regexp.MustCompile(`(?m)^<!-- (pages? [\d-]+|para \d+) -->$`)

// locate returns the location marker nearest above the first occurrence of
// quote, which is how a reader of the Markdown finds where a passage is
func locate(t *testing.T, markdown, quote string) string {
	t.Helper()
	idx := strings.Index(markdown, quote)
	if idx < 0 {
		t.Fatalf("quote %q not found in output:\n%s", quote, markdown)
	}
	matches := locationMarker.FindAllStringSubmatch(markdown[:idx], -1)
	if len(matches) == 0 {
		t.Fatalf("no location marker above %q in output:\n%s", quote, markdown)
	}
	return matches[len(matches)-1][1]
}

func checkLocations(t *testing.T, markdown string, want map[string]string) {
	t.Helper()
	for quote, location := range want {
		if got := locate(t, markdown, quote); got != location {
			t.Errorf("%q located at %q, want %q", quote, got, location)
		}
	}
}

// checkSourceHeader verifies the first line names the original file by its
// absolute path
func checkSourceHeader(t *testing.T, markdown, fixture string) {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	want := "<!-- source: " + abs + " -->\n"
	if !strings.HasPrefix(markdown, want) {
		t.Errorf("output does not start with %q:\n%.300s", want, markdown)
	}
}

func TestCLI_PDFPageMarkers(t *testing.T) {
	md := runCLI(t, "pages.pdf")
	checkSourceHeader(t, md, "pages.pdf")

	if !strings.Contains(md, "converted from a 7-page PDF") {
		t.Errorf("expected the marker convention comment with the page count:\n%.400s", md)
	}
	checkLocations(t, md, map[string]string{
		"The first page talks about the weather":   "page 1",
		"dog's vacation at the lake":               "page 2",
		"The fourth page follows a blank page":     "page 4", // page 3 is blank and stripped
		"Inventory":                                "page 5",
		"Closing remarks after the long table end": "page 7",
	})
	if strings.Contains(md, "<!-- page 3 -->") {
		t.Error("blank page 3 should have no marker")
	}
}

func TestCLI_PDFGlyphByGlyphTextKeepsSpaces(t *testing.T) {
	md := runCLI(t, "glyphs.pdf")
	for _, sentence := range []string{
		"The dog's vacation was approved by the committee.",
		"Evidence of the review is kept for seven years.",
		"Access rights are removed when a person leaves.",
	} {
		if !strings.Contains(md, sentence) {
			t.Errorf("expected %q in output:\n%s", sentence, md)
		}
	}
}

func TestCLI_DOCXParagraphMarkers(t *testing.T) {
	md := runCLI(t, "procedure.docx")
	checkSourceHeader(t, md, "procedure.docx")

	if !strings.Contains(md, "converted from a DOCX") {
		t.Errorf("expected the marker convention comment:\n%.400s", md)
	}
	// Paragraph numbers were checked against a browser rendering of this
	// file: paragraph N in the document is the Nth paragraph on screen.
	checkLocations(t, md, map[string]string{
		"# Incident Response Procedure":       "para 1",
		"# 1 Purpose":                         "para 2",
		"## 1.1 Scope":                        "para 4",
		"Step 3: notify the incident manager": "para 4",
		"# 2 Roles":                           "para 13",
		"| Role | Responsibility | Contact |": "para 14",
		"| Role 17 |":                         "para 14",
		"# 3 Wide appendix (landscape)":       "para 110",
		"UNIQUE-MARKER-ZEBRA final sentence.": "para 130",
	})
}

func TestCLI_XLSXSheetAndRangeLocators(t *testing.T) {
	md := runCLI(t, "register.xlsx")
	checkSourceHeader(t, md, "register.xlsx")

	for _, want := range []string{
		"## Access Review\n",
		"### Range: Access Review!A1:D8\n| Row | A | B | C | D |",
		"| 4 | user3 | Admin | 2026-03-15 | Approved |",
		"### Range: Access Review!A12:B14\n",
		"## Exceptions\n",
		"### Range: Exceptions!A1:B2\n",
		"| 2 | EX-1 | The dog's vacation exception |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("expected %q in output:\n%s", want, md)
		}
	}
}

func TestCLI_NoLocationMarkersFlag(t *testing.T) {
	for _, fixture := range []string{"pages.pdf", "procedure.docx"} {
		md := runCLI(t, fixture, "-no-location-markers")
		checkSourceHeader(t, md, fixture)
		if locationMarker.MatchString(md) || strings.Contains(md, "<!-- x2md:") {
			t.Errorf("%s: expected no location markers:\n%.600s", fixture, md)
		}
	}
}

// Every marker must be on a line of its own with a blank line either side,
// or Markdown renderers and line-based searches will not treat it as one
func TestCLI_MarkersStandAlone(t *testing.T) {
	for _, fixture := range []string{"pages.pdf", "procedure.docx"} {
		lines := strings.Split(runCLI(t, fixture), "\n")
		for i, line := range lines {
			if !strings.Contains(line, "<!-- pa") {
				continue
			}
			if !locationMarker.MatchString(line) {
				t.Errorf("%s line %d: marker shares a line with other text: %q", fixture, i+1, line)
			}
			if i == 0 || lines[i-1] != "" || i+1 >= len(lines) || lines[i+1] != "" {
				t.Errorf("%s line %d: marker %q is not surrounded by blank lines", fixture, i+1, line)
			}
		}
	}
}
