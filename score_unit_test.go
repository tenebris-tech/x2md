//go:build score

/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// Unit tests for the pure helpers behind TestCorpusScore. They use invented
// inputs only.

func TestScoreTokens(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		limit int
		want  []string
	}{
		{"lowercase and drop short", "The Quick a fox, 42 x9!", -1, []string{"the", "quick", "fox", "42", "x9"}},
		{"cap", "one two three four", 2, []string{"one", "two"}},
		{"empty", "", -1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scoreTokens(tt.text, tt.limit); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("scoreTokens(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestScoreTokenCap(t *testing.T) {
	text := strings.Repeat("word ", scoreTokenCap+100)
	if got := len(scoreTokens(text, scoreTokenCap)); got != scoreTokenCap {
		t.Errorf("got %d tokens, want %d", got, scoreTokenCap)
	}
	if got := len(scoreMarkdownTokens(text, scoreTokenCap)); got != scoreTokenCap {
		t.Errorf("markdown: got %d tokens, want %d", got, scoreTokenCap)
	}
}

func TestScoreMarkdownTokens(t *testing.T) {
	md := "<!-- source: /tmp/alice.pdf -->\nAlice met Bob[^12] at noon[3].\n<!-- a\nmultiline note -->\n## Bob replied"
	want := []string{"alice", "met", "bob", "at", "noon", "bob", "replied"}
	if got := scoreMarkdownTokens(md, -1); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestScoreReadingOrder(t *testing.T) {
	ref := scoreTokens("alice sends the report to bob and bob reads it every morning", -1)
	if got := scoreReadingOrder(ref, ref); got != 1 {
		t.Errorf("identical: got %v, want 1", got)
	}
	half := len(ref) / 2
	swapped := append(append([]string(nil), ref[half:]...), ref[:half]...)
	if got := scoreReadingOrder(ref, swapped); got >= 1 || got <= 0 {
		t.Errorf("reordered halves: got %v, want between 0 and 1", got)
	}
	if got := scoreReadingOrder(ref, nil); got != 0 {
		t.Errorf("empty output: got %v, want 0", got)
	}
	if got := scoreLCS([]string{"aa", "bb", "cc", "dd"}, []string{"bb", "xx", "dd", "aa"}); got != 2 {
		t.Errorf("LCS: got %d, want 2", got)
	}
}

func TestScoreCountLines(t *testing.T) {
	tables, headings := scoreCountLines("# Title\ntext\n| a | b |\n| --- | --- |\n## Sub\n  | indented")
	if tables != 2 || headings != 2 {
		t.Errorf("got tables %d headings %d, want 2 and 2", tables, headings)
	}
}

func TestScoreDedupe(t *testing.T) {
	in := []scoreFile{
		{Path: "z/alice.pdf", SHA256: "aaa"},
		{Path: "b/bob.pdf", SHA256: "bbb"},
		{Path: "a/alice-copy.pdf", SHA256: "aaa"},
	}
	want := []scoreFile{
		{Path: "a/alice-copy.pdf", SHA256: "aaa"},
		{Path: "b/bob.pdf", SHA256: "bbb"},
	}
	if got := scoreDedupe(in); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestScoreTSVRoundTrip(t *testing.T) {
	records := []scoreRecord{
		{SHA256: "bb", Score: scoreError, Repeatable: true, Path: "bob.pdf"},
		{SHA256: "aa", Score: "0.9123", Repeatable: true, Tables: 4, Headings: 7, Path: "alice.pdf"},
		{SHA256: "cc", Score: scoreUnscored, Repeatable: false, Path: "scans/carol.pdf"},
	}
	var buf bytes.Buffer
	if err := writeScoreTSV(&buf, records); err != nil {
		t.Fatal(err)
	}
	wantText := "aa\t0.9123\tyes\t4\t7\talice.pdf\n" +
		"bb\terror\tyes\t0\t0\tbob.pdf\n" +
		"cc\t-\tno\t0\t0\tscans/carol.pdf\n"
	if buf.String() != wantText {
		t.Errorf("written:\n%s\nwant:\n%s", buf.String(), wantText)
	}
	got, err := readScoreTSV(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	want := []scoreRecord{records[1], records[0], records[2]}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("read %v, want %v", got, want)
	}
}

func TestScoreTSVRejectsBadLines(t *testing.T) {
	for _, line := range []string{
		"aa\t0.5\tyes\t1\talice.pdf",      // too few fields
		"aa\tabc\tyes\t1\t2\talice.pdf",   // bad score
		"aa\t0.5\tmaybe\t1\t2\talice.pdf", // bad repeatable
		"aa\t0.5\tyes\tone\t2\talice.pdf", // bad table count
		"aa\t0.5\tyes\t1\ttwo\talice.pdf", // bad heading count
	} {
		if _, err := readScoreTSV(strings.NewReader(line + "\n")); err == nil {
			t.Errorf("no error for %q", line)
		}
	}
}

func TestScoreCompare(t *testing.T) {
	rec := func(path, sha, score string) scoreRecord {
		return scoreRecord{SHA256: sha, Score: score, Repeatable: true, Path: path}
	}
	baseline := []scoreRecord{
		rec("better.pdf", "1", "0.5000"),
		rec("much-better.pdf", "2", "0.1000"),
		rec("worse.pdf", "3", "0.9000"),
		rec("same.pdf", "4", "0.8000"),
		rec("removed.pdf", "5", "0.7000"),
		rec("breaks.pdf", "6", "0.6000"),
		rec("still-broken.pdf", "7", scoreError),
		rec("changed.pdf", "8", "0.9000"),
		rec("scan.pdf", "9", scoreUnscored),
	}
	current := []scoreRecord{
		rec("better.pdf", "1", "0.6000"),
		rec("much-better.pdf", "2", "0.9000"),
		rec("worse.pdf", "3", "0.8000"),
		rec("same.pdf", "4", "0.8040"),
		rec("added.pdf", "10", "0.1000"),
		rec("added-error.pdf", "11", scoreError),
		rec("breaks.pdf", "6", scoreError),
		rec("still-broken.pdf", "7", scoreError),
		rec("changed.pdf", "12", "0.1000"),
		rec("scan.pdf", "9", scoreUnscored),
	}
	got := compareScores(baseline, current)
	want := scoreComparison{
		Better: []scoreDelta{
			{Path: "much-better.pdf", Old: 0.1, New: 0.9},
			{Path: "better.pdf", Old: 0.5, New: 0.6},
		},
		Worse:     []scoreDelta{{Path: "worse.pdf", Old: 0.9, New: 0.8}},
		Added:     []string{"added-error.pdf", "added.pdf", "changed.pdf"},
		Removed:   []string{"changed.pdf", "removed.pdf"},
		NewErrors: []string{"added-error.pdf", "breaks.pdf"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}
