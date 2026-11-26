//go:build score

/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tenebris-tech/x2md/pdf2md"
)

// Corpus score for PDF conversion, run with "make score" and recorded as the
// baseline with "make score-accept". It converts every distinct PDF in
// private/ twice, checks that both outputs are identical, and scores reading
// order against pdftotext. Results are kept in private/.score/, which is not
// in the repository.

const (
	scoreCorpusDir   = "private"
	scoreStateDir    = "private/.score"
	scoreCurrentFile = "current.tsv"
	scoreBaseline    = "baseline.tsv"
	scoreAcceptEnv   = "X2MD_SCORE_ACCEPT"

	// scoreTokenCap is the number of leading tokens of each text compared.
	scoreTokenCap = 6000
	// scoreMinRefTokens is the reference size below which a file is unscored.
	scoreMinRefTokens = 30
	// scoreTolerance is the change in score that counts as better or worse.
	scoreTolerance = 0.005
	// scoreMostImproved is how many of the most improved files are listed.
	scoreMostImproved = 20
	// scorePdftotextTimeout bounds one pdftotext run.
	scorePdftotextTimeout = 2 * time.Minute
)

// Score field values for files that have no numeric score.
const (
	scoreUnscored = "-"
	scoreError    = "error"
)

// scoreRecord is one line of current.tsv or baseline.tsv.
type scoreRecord struct {
	SHA256     string
	Score      string // 4-decimal score, scoreUnscored or scoreError
	Repeatable bool
	Tables     int // output lines starting with "|"
	Headings   int // output lines starting with "#"
	Path       string
}

// scoreDelta is a file whose score changed between baseline and current.
type scoreDelta struct {
	Path     string
	Old, New float64
}

// scoreComparison is the result of comparing current records to a baseline.
type scoreComparison struct {
	Better    []scoreDelta // sorted by improvement, largest first
	Worse     []scoreDelta // sorted by path
	Added     []string
	Removed   []string
	NewErrors []string // files that error now but did not in the baseline
}

var (
	scoreTokenRE    = regexp.MustCompile(`[a-z0-9]{2,}`)
	scoreCommentRE  = regexp.MustCompile(`(?s)<!--.*?-->`)
	scoreFootnoteRE = regexp.MustCompile(`\[\^?[0-9]+\]`)
)

// scoreTokens returns the lowercase runs of [a-z0-9]{2,} in text, at most
// limit of them.
func scoreTokens(text string, limit int) []string {
	return scoreTokenRE.FindAllString(strings.ToLower(text), limit)
}

// scoreMarkdownTokens strips HTML comments and footnote references from
// markdown, then tokenises it like scoreTokens.
func scoreMarkdownTokens(markdown string, limit int) []string {
	s := scoreCommentRE.ReplaceAllString(markdown, " ")
	s = scoreFootnoteRE.ReplaceAllString(s, " ")
	return scoreTokens(s, limit)
}

// scoreLCS returns the length of the longest common subsequence of a and b.
func scoreLCS(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	// Intern tokens so the inner loop compares integers.
	ids := make(map[string]int, len(a))
	ai := make([]int, len(a))
	for i, tok := range a {
		id, ok := ids[tok]
		if !ok {
			id = len(ids)
			ids[tok] = id
		}
		ai[i] = id
	}
	bi := make([]int, len(b))
	for j, tok := range b {
		id, ok := ids[tok]
		if !ok {
			id = -1 // not in a, never matches
		}
		bi[j] = id
	}

	prev := make([]int, len(bi)+1)
	cur := make([]int, len(bi)+1)
	for _, x := range ai {
		for j, y := range bi {
			switch {
			case x == y:
				cur[j+1] = prev[j] + 1
			case prev[j+1] >= cur[j]:
				cur[j+1] = prev[j+1]
			default:
				cur[j+1] = cur[j]
			}
		}
		prev, cur = cur, prev
	}
	return prev[len(bi)]
}

// scoreReadingOrder returns 2*LCS(ref, out) / (len(ref)+len(out)).
func scoreReadingOrder(ref, out []string) float64 {
	total := len(ref) + len(out)
	if total == 0 {
		return 0
	}
	return 2 * float64(scoreLCS(ref, out)) / float64(total)
}

// scoreCountLines returns the number of lines starting with "|" and with "#".
func scoreCountLines(markdown string) (tables, headings int) {
	for line := range strings.SplitSeq(markdown, "\n") {
		switch {
		case strings.HasPrefix(line, "|"):
			tables++
		case strings.HasPrefix(line, "#"):
			headings++
		}
	}
	return tables, headings
}

// scoreFile is a candidate corpus file: its path relative to the corpus root
// (with forward slashes) and the SHA-256 of its content.
type scoreFile struct {
	Path   string
	SHA256 string
}

// scoreDedupe sorts files by path and drops every file whose content hash was
// already seen, so the lexically first path of each content is kept.
func scoreDedupe(files []scoreFile) []scoreFile {
	sorted := append([]scoreFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	seen := make(map[string]bool, len(sorted))
	out := sorted[:0]
	for _, f := range sorted {
		if seen[f.SHA256] {
			continue
		}
		seen[f.SHA256] = true
		out = append(out, f)
	}
	return out
}

// writeScoreTSV writes records sorted by path.
func writeScoreTSV(w io.Writer, records []scoreRecord) error {
	sorted := append([]scoreRecord(nil), records...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	bw := bufio.NewWriter(w)
	for _, r := range sorted {
		rep := "no"
		if r.Repeatable {
			rep = "yes"
		}
		if _, err := fmt.Fprintf(bw, "%s\t%s\t%s\t%d\t%d\t%s\n",
			r.SHA256, r.Score, rep, r.Tables, r.Headings, r.Path); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// readScoreTSV parses records written by writeScoreTSV.
func readScoreTSV(r io.Reader) ([]scoreRecord, error) {
	var records []scoreRecord
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\t", 6)
		if len(f) != 6 {
			return nil, fmt.Errorf("line %d: want 6 fields, got %d", lineNo, len(f))
		}
		if f[1] != scoreUnscored && f[1] != scoreError {
			if _, err := strconv.ParseFloat(f[1], 64); err != nil {
				return nil, fmt.Errorf("line %d: bad score %q", lineNo, f[1])
			}
		}
		if f[2] != "yes" && f[2] != "no" {
			return nil, fmt.Errorf("line %d: bad repeatable value %q", lineNo, f[2])
		}
		tables, err := strconv.Atoi(f[3])
		if err != nil {
			return nil, fmt.Errorf("line %d: bad table count %q", lineNo, f[3])
		}
		headings, err := strconv.Atoi(f[4])
		if err != nil {
			return nil, fmt.Errorf("line %d: bad heading count %q", lineNo, f[4])
		}
		records = append(records, scoreRecord{
			SHA256: f[0], Score: f[1], Repeatable: f[2] == "yes",
			Tables: tables, Headings: headings, Path: f[5],
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// scoreValue returns the numeric score of a record, if it has one.
func scoreValue(r scoreRecord) (float64, bool) {
	if r.Score == scoreUnscored || r.Score == scoreError {
		return 0, false
	}
	v, err := strconv.ParseFloat(r.Score, 64)
	return v, err == nil
}

// scoreKey identifies a file across runs: a file whose content changed at the
// same path counts as removed and added.
func scoreKey(r scoreRecord) string {
	return r.Path + "\x00" + r.SHA256
}

// compareScores classifies current records against a baseline.
func compareScores(baseline, current []scoreRecord) scoreComparison {
	var c scoreComparison
	base := make(map[string]scoreRecord, len(baseline))
	for _, r := range baseline {
		base[scoreKey(r)] = r
	}
	cur := make(map[string]bool, len(current))
	for _, r := range current {
		k := scoreKey(r)
		cur[k] = true
		old, ok := base[k]
		if !ok {
			c.Added = append(c.Added, r.Path)
			if r.Score == scoreError {
				c.NewErrors = append(c.NewErrors, r.Path)
			}
			continue
		}
		if r.Score == scoreError {
			if old.Score != scoreError {
				c.NewErrors = append(c.NewErrors, r.Path)
			}
			continue
		}
		ov, ok1 := scoreValue(old)
		nv, ok2 := scoreValue(r)
		if !ok1 || !ok2 {
			continue
		}
		switch {
		case nv < ov-scoreTolerance:
			c.Worse = append(c.Worse, scoreDelta{Path: r.Path, Old: ov, New: nv})
		case nv > ov+scoreTolerance:
			c.Better = append(c.Better, scoreDelta{Path: r.Path, Old: ov, New: nv})
		}
	}
	for _, r := range baseline {
		if !cur[scoreKey(r)] {
			c.Removed = append(c.Removed, r.Path)
		}
	}
	sort.Strings(c.Added)
	sort.Strings(c.Removed)
	sort.Strings(c.NewErrors)
	sort.Slice(c.Worse, func(i, j int) bool { return c.Worse[i].Path < c.Worse[j].Path })
	sort.SliceStable(c.Better, func(i, j int) bool {
		di, dj := c.Better[i].New-c.Better[i].Old, c.Better[j].New-c.Better[j].Old
		if di != dj {
			return di > dj
		}
		return c.Better[i].Path < c.Better[j].Path
	})
	return c
}

// scoreMean returns the mean numeric score of records and how many have one.
func scoreMean(records []scoreRecord) (float64, int) {
	sum, n := 0.0, 0
	for _, r := range records {
		if v, ok := scoreValue(r); ok {
			sum += v
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	return sum / float64(n), n
}

// --- corpus run -------------------------------------------------------------

// findCorpusPDFs returns the distinct PDFs under root, by content, in path order.
func findCorpusPDFs(root string) ([]scoreFile, error) {
	var files []scoreFile
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p == filepath.Clean(scoreStateDir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(p), ".pdf") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, scoreFile{Path: filepath.ToSlash(rel), SHA256: hex.EncodeToString(sum[:])})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return scoreDedupe(files), nil
}

// convertForScore converts one PDF with the CLI defaults, without images or
// page markers. A panic is returned as an error.
func convertForScore(data []byte) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	conv := pdf2md.New(pdf2md.WithExtractImages(false), pdf2md.WithPageMarkers(false))
	return conv.Convert(data)
}

// pdftotextReference returns the plain-text reference for a PDF.
func pdftotextReference(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), scorePdftotextTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "pdftotext", path, "-")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// scoreResult is the outcome for one file, with diagnostics for the log.
type scoreResult struct {
	Record  scoreRecord
	ConvErr error
	RefErr  error
}

func scoreOneFile(f scoreFile) scoreResult {
	res := scoreResult{Record: scoreRecord{SHA256: f.SHA256, Path: f.Path, Repeatable: true}}
	full := filepath.Join(scoreCorpusDir, filepath.FromSlash(f.Path))
	data, err := os.ReadFile(full)
	if err != nil {
		res.ConvErr = err
		res.Record.Score = scoreError
		return res
	}

	out1, err1 := convertForScore(data)
	out2, err2 := convertForScore(data)
	if (err1 == nil) != (err2 == nil) || out1 != out2 {
		res.Record.Repeatable = false
	}
	if err1 != nil || err2 != nil {
		res.ConvErr = errors.Join(err1, err2)
		res.Record.Score = scoreError
		return res
	}
	res.Record.Tables, res.Record.Headings = scoreCountLines(out1)

	ref, err := pdftotextReference(full)
	if err != nil {
		// No usable reference: the file is unscored, like a scanned one.
		res.RefErr = err
	}
	refTokens := scoreTokens(ref, scoreTokenCap)
	if len(refTokens) < scoreMinRefTokens {
		res.Record.Score = scoreUnscored
		return res
	}
	outTokens := scoreMarkdownTokens(out1, scoreTokenCap)
	res.Record.Score = strconv.FormatFloat(scoreReadingOrder(refTokens, outTokens), 'f', 4, 64)
	return res
}

func readScoreFile(path string) ([]scoreRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readScoreTSV(f)
}

func writeScoreFile(path string, records []scoreRecord) error {
	var buf bytes.Buffer
	if err := writeScoreTSV(&buf, records); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func TestCorpusScore(t *testing.T) {
	start := time.Now()
	accept := os.Getenv(scoreAcceptEnv) != ""

	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Fatalf("pdftotext is not on PATH; install poppler-utils (it gives the reference text)")
	}
	if st, err := os.Stat(scoreCorpusDir); err != nil || !st.IsDir() {
		t.Fatalf("the corpus directory %s/ is missing", scoreCorpusDir)
	}
	files, err := findCorpusPDFs(scoreCorpusDir)
	if err != nil {
		t.Fatalf("scanning %s/: %v", scoreCorpusDir, err)
	}
	if len(files) == 0 {
		t.Fatalf("no PDF files found under %s/", scoreCorpusDir)
	}
	t.Logf("scoring %d distinct PDFs with %d workers", len(files), runtime.NumCPU())

	results := make([]scoreResult, len(files))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Go(func() {
			for i := range jobs {
				results[i] = scoreOneFile(files[i])
			}
		})
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	records := make([]scoreRecord, len(results))
	var unscored, errored, notRepeatable, tables, headings int
	for i, r := range results {
		records[i] = r.Record
		tables += r.Record.Tables
		headings += r.Record.Headings
		switch r.Record.Score {
		case scoreUnscored:
			unscored++
		case scoreError:
			errored++
			t.Logf("error: %s: %v", r.Record.Path, r.ConvErr)
		}
		if r.RefErr != nil {
			t.Logf("no reference text: %s: %v", r.Record.Path, r.RefErr)
		}
		if !r.Record.Repeatable {
			notRepeatable++
			t.Errorf("not repeatable: %s (two conversions gave different output)", r.Record.Path)
		}
	}

	if err := os.MkdirAll(scoreStateDir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", scoreStateDir, err)
	}
	if err := writeScoreFile(filepath.Join(scoreStateDir, scoreCurrentFile), records); err != nil {
		t.Fatalf("writing %s: %v", scoreCurrentFile, err)
	}

	mean, scored := scoreMean(records)
	t.Logf("files %d, scored %d, unscored %d, errors %d, not repeatable %d",
		len(records), scored, unscored, errored, notRepeatable)
	t.Logf("mean score %.4f; table lines %d, heading lines %d", mean, tables, headings)

	baselinePath := filepath.Join(scoreStateDir, scoreBaseline)
	baseline, err := readScoreFile(baselinePath)
	haveBaseline := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("reading %s: %v", baselinePath, err)
	}

	if haveBaseline {
		c := compareScores(baseline, records)
		oldMean, _ := scoreMean(baseline)
		t.Logf("mean before %.4f, after %.4f; better %d, worse %d (by more than %.3f)",
			oldMean, mean, len(c.Better), len(c.Worse), scoreTolerance)
		for _, p := range c.Added {
			t.Logf("added since baseline: %s", p)
		}
		for _, p := range c.Removed {
			t.Logf("removed since baseline: %s", p)
		}
		for i, d := range c.Better {
			if i == scoreMostImproved {
				break
			}
			t.Logf("better: %s %.4f -> %.4f", d.Path, d.Old, d.New)
		}
		for _, d := range c.Worse {
			t.Logf("worse: %s %.4f -> %.4f", d.Path, d.Old, d.New)
		}
		if !accept {
			if len(c.Worse) > 0 {
				t.Errorf("%d files scored worse than the baseline", len(c.Worse))
			}
			for _, p := range c.NewErrors {
				t.Errorf("new conversion error: %s", p)
			}
		}
	} else if !accept {
		t.Logf("no baseline exists; run \"make score-accept\" to record this run as the baseline")
		if errored > 0 {
			t.Errorf("%d files failed to convert", errored)
		}
	}

	if accept {
		if err := writeScoreFile(baselinePath, records); err != nil {
			t.Fatalf("writing %s: %v", baselinePath, err)
		}
		t.Logf("baseline written to %s", baselinePath)
	}
	t.Logf("wall time %s", time.Since(start).Round(time.Second))
}
