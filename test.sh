#!/usr/bin/env bash
# ******************************************************************************
# * Copyright (c) 2026 Tenebris Technologies Inc.                              *
# * Please see LICENSE file for details.                                       *
# ******************************************************************************
#
# Full regression suite for x2md. Run it through "make test".
#
#   0.x  Setup and preconditions
#   1.x  Static checks (formatting and vet, verified but never applied)
#   2.x  Go test suite (race detector on, no cache)
#   3.x  CLI end-to-end: conversions
#   4.x  CLI end-to-end: location contract
#   5.x  CLI end-to-end: error handling
#   6.x  Cleanup and final verification
#
# Exit code: 0 if every test passed, non-zero otherwise.

set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT" || exit 1

usage() {
    cat <<EOF
Usage: $0 [-n] [-k] [-h]

  -n  no colour in the output
  -k  keep the temporary work directory for debugging
  -h  show this help
EOF
}

USE_COLOUR=1
KEEP=0
while getopts "nkh" opt; do
    case "$opt" in
        n) USE_COLOUR=0 ;;
        k) KEEP=1 ;;
        h) usage; exit 0 ;;
        *) usage >&2; exit 2 ;;
    esac
done

if [ ! -t 1 ] || [ -n "${NO_COLOR:-}" ]; then
    USE_COLOUR=0
fi
if [ "$USE_COLOUR" -eq 1 ]; then
    GREEN=$'\033[32m'; RED=$'\033[31m'; YELLOW=$'\033[33m'; BOLD=$'\033[1m'; RESET=$'\033[0m'
else
    GREEN=""; RED=""; YELLOW=""; BOLD=""; RESET=""
fi

TOTAL=0
PASSED=0
FAILED=0
SKIPPED=0
FAILURES=()

WORK=""
cleanup() {
    if [ -n "$WORK" ] && [ -d "$WORK" ]; then
        if [ "$KEEP" -eq 1 ]; then
            echo "Work directory kept: $WORK"
        else
            rm -rf "$WORK"
        fi
    fi
}
trap cleanup EXIT
trap 'exit 130' INT TERM

section() {
    printf '\n%s%s%s\n' "$BOLD" "$1" "$RESET"
}

pass() {
    TOTAL=$((TOTAL + 1)); PASSED=$((PASSED + 1))
    printf '  %sPASS%s  %s\n' "$GREEN" "$RESET" "$1"
}

fail() {
    TOTAL=$((TOTAL + 1)); FAILED=$((FAILED + 1))
    FAILURES+=("$1")
    printf '  %sFAIL%s  %s\n' "$RED" "$RESET" "$1"
    if [ -n "${2:-}" ]; then
        printf '%s\n' "$2" | sed 's/^/        /'
    fi
}

skip() {
    TOTAL=$((TOTAL + 1)); SKIPPED=$((SKIPPED + 1))
    printf '  %sSKIP%s  %s\n' "$YELLOW" "$RESET" "$1"
}

# check NAME COMMAND...: passes if the command exits 0
check() {
    local name="$1"; shift
    local out
    if out="$("$@" 2>&1)"; then
        pass "$name"
    else
        fail "$name" "$out"
    fi
}

summary() {
    echo
    echo "============================================"
    echo "   TEST SUMMARY"
    echo "============================================"
    echo
    echo "Total Tests: $TOTAL"
    echo "Passed:      $PASSED"
    echo "Failed:      $FAILED"
    echo "Skipped:     $SKIPPED"
    echo
    if [ "$FAILED" -gt 0 ]; then
        printf '%sFAILURES DETECTED%s\n' "$RED" "$RESET"
        for f in "${FAILURES[@]}"; do
            echo "  - $f"
        done
    else
        printf '%sAll tests passed!%s\n' "$GREEN" "$RESET"
    fi
}

# A missing prerequisite stops the run: the rest of the suite cannot execute,
# and a partial run must not look like a pass.
require() {
    local name="$1" what="$2"
    if command -v "$what" >/dev/null 2>&1; then
        pass "$name"
    else
        fail "$name" "missing prerequisite: $what"
        summary
        exit 1
    fi
}

# ---------------------------------------------------------------------------
section "0. Setup and preconditions"

require "0.1 Go toolchain is installed" go
require "0.2 gofmt is installed" gofmt
# The race detector needs cgo and a C compiler
CC_BIN="$(go env CC)"
require "0.3 C compiler for the race detector ($CC_BIN)" "$CC_BIN"

TREE_BEFORE="$(git status --porcelain 2>/dev/null)"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/x2md-test.XXXXXX")" || { echo "cannot create work directory" >&2; exit 1; }
BIN="$WORK/x2md"

if out="$(go build -o "$BIN" ./cli 2>&1)"; then
    pass "0.4 CLI builds"
else
    fail "0.4 CLI builds" "$out"
    summary
    exit 1
fi

# ---------------------------------------------------------------------------
section "1. Static checks"

unformatted="$(gofmt -l . 2>&1)"
if [ -z "$unformatted" ]; then
    pass "1.1 gofmt: all files formatted"
else
    fail "1.1 gofmt: all files formatted" "not formatted (run 'make fmt'):
$unformatted"
fi

check "1.2 go vet" go vet ./...
check "1.3 go vet (private corpus tests)" go vet -tags private .

# ---------------------------------------------------------------------------
section "2. Go test suite (go test -race -count=1 ./...)"

GO_JSON="$WORK/go-test.json"
go test -race -count=1 -json ./... >"$GO_JSON" 2>"$WORK/go-test.stderr"
GO_EXIT=$?

# One result per package. Packages without test files report "skip".
pkg_results="$(sed -n 's/.*"Action":"\(pass\|fail\|skip\)","Package":"\([^"]*\)","Elapsed".*/\1 \2/p' "$GO_JSON" | sort -k2)"
pkg_count=0
while read -r action pkg; do
    [ -z "$action" ] && continue
    pkg_count=$((pkg_count + 1))
    case "$action" in
        pass) pass "2.$pkg_count $pkg" ;;
        fail) fail "2.$pkg_count $pkg" "$(grep -F "\"Action\":\"fail\",\"Package\":\"$pkg\",\"Test\"" "$GO_JSON" | sed 's/.*"Test":"\([^"]*\)".*/failed: \1/')" ;;
        skip) skip "2.$pkg_count $pkg (no test files)" ;;
    esac
done <<EOF
$pkg_results
EOF

# Individual tests that skipped themselves inside a passing package
test_skips="$(grep '"Action":"skip"' "$GO_JSON" | grep '"Test":' | sed 's/.*"Package":"\([^"]*\)","Test":"\([^"]*\)".*/\1 \2/')"
if [ -n "$test_skips" ]; then
    while read -r line; do
        skip "2.x skipped test: $line"
    done <<EOF
$test_skips
EOF
fi

# go test must never fail without a recorded failure (build errors, panics)
if [ "$GO_EXIT" -ne 0 ] && ! grep -q '"Action":"fail"' "$GO_JSON"; then
    fail "2.0 go test exited $GO_EXIT" "$(cat "$WORK/go-test.stderr")"
elif [ "$pkg_count" -eq 0 ]; then
    fail "2.0 go test reported no packages" "$(cat "$WORK/go-test.stderr")"
fi

# ---------------------------------------------------------------------------
section "3. CLI end-to-end: conversions"

# run_cli NAME: runs the binary, capturing stdout, stderr and the exit code
run_cli() {
    CLI_OUT="$WORK/stdout.txt"; CLI_ERR="$WORK/stderr.txt"
    "$BIN" "$@" >"$CLI_OUT" 2>"$CLI_ERR"
    CLI_EXIT=$?
}

n=0
for fixture in pages.pdf glyphs.pdf procedure.docx register.xlsx; do
    n=$((n + 1))
    out="$WORK/$fixture.md"
    run_cli -output "$out" "testdata/$fixture"
    if [ "$CLI_EXIT" -ne 0 ]; then
        fail "3.$n convert $fixture" "exit $CLI_EXIT: $(cat "$CLI_ERR")"
    elif [ ! -s "$out" ]; then
        fail "3.$n convert $fixture" "no output written"
    elif ! grep -q "Conversion complete" "$CLI_OUT"; then
        fail "3.$n convert $fixture" "stdout: $(cat "$CLI_OUT")"
    elif [ -s "$CLI_ERR" ]; then
        fail "3.$n convert $fixture" "unexpected stderr: $(cat "$CLI_ERR")"
    else
        pass "3.$n convert $fixture"
    fi
done

# Without -output the Markdown is written next to the input file
cp testdata/register.xlsx "$WORK/copy.xlsx"
run_cli "$WORK/copy.xlsx"
if [ "$CLI_EXIT" -eq 0 ] && ls "$WORK"/copy*.md >/dev/null 2>&1; then
    pass "3.5 default output is written next to the input"
else
    fail "3.5 default output is written next to the input" "exit $CLI_EXIT: $(cat "$CLI_ERR")"
fi

# The steps are numbered and the bullet nested only through their Word styles
steps=0
for i in 1 2 3 4 5; do
    grep -q "^$i\. Step $i: notify" "$WORK/procedure.docx.md" && steps=$((steps + 1))
done
if [ "$steps" -eq 5 ] && grep -q "^    - Nested bullet$" "$WORK/procedure.docx.md"; then
    pass "3.6 DOCX list numbered through its paragraph style"
else
    fail "3.6 DOCX list numbered through its paragraph style" "$(grep -E 'Step [1-5]|Nested bullet' "$WORK/procedure.docx.md")"
fi

# ---------------------------------------------------------------------------
section "4. CLI end-to-end: location contract"

# The first line names the original file by absolute path
n=0
for fixture in pages.pdf procedure.docx register.xlsx; do
    n=$((n + 1))
    want="<!-- source: $ROOT/testdata/$fixture -->"
    got="$(head -n 1 "$WORK/$fixture.md" 2>/dev/null)"
    if [ "$got" = "$want" ]; then
        pass "4.$n source path heads the output of $fixture"
    else
        fail "4.$n source path heads the output of $fixture" "got:  $got
want: $want"
    fi
done

# located QUOTE FILE: prints the location marker nearest above the quote
located() {
    awk -v quote="$1" '
        /^<!-- (pages? [0-9-]+|para [0-9]+) -->$/ { marker = $0 }
        index($0, quote) { print marker; exit }
    ' "$2"
}

expect_location() {
    local name="$1" file="$2" quote="$3" want="$4" got
    got="$(located "$quote" "$WORK/$file.md")"
    if [ "$got" = "<!-- $want -->" ]; then
        pass "$name"
    else
        fail "$name" "\"$quote\" located at \"$got\", want \"<!-- $want -->\""
    fi
}

expect_location "4.4 PDF passage on page 2"                pages.pdf      "dog's vacation at the lake"          "page 2"
expect_location "4.5 PDF page after a stripped blank page" pages.pdf      "The fourth page follows a blank page" "page 4"
expect_location "4.6 DOCX heading paragraph"               procedure.docx "# 2 Roles"                            "para 13"
expect_location "4.7 DOCX table paragraph"                 procedure.docx "| Role 17 |"                          "para 14"
expect_location "4.8 DOCX paragraph late in the document"  procedure.docx "UNIQUE-MARKER-ZEBRA final sentence."  "para 130"

if grep -q "^### Range: Access Review!A1:D8$" "$WORK/register.xlsx.md" &&
    grep -q "^| 4 | user3 | Admin | 2026-03-15 | Approved |$" "$WORK/register.xlsx.md"; then
    pass "4.9 XLSX sheet, range and row"
else
    fail "4.9 XLSX sheet, range and row" "$(head -n 12 "$WORK/register.xlsx.md")"
fi

if grep -q "The dog's vacation was approved by the committee." "$WORK/glyphs.pdf.md"; then
    pass "4.10 PDF drawn glyph by glyph keeps its word spaces"
else
    fail "4.10 PDF drawn glyph by glyph keeps its word spaces" "$(cat "$WORK/glyphs.pdf.md")"
fi

run_cli -no-location-markers -output "$WORK/plain.md" testdata/pages.pdf
if [ "$CLI_EXIT" -eq 0 ] && ! grep -Eq '^<!-- (page|para|x2md)' "$WORK/plain.md" &&
    grep -q '^<!-- source: ' "$WORK/plain.md"; then
    pass "4.11 -no-location-markers removes markers and keeps the source path"
else
    fail "4.11 -no-location-markers removes markers and keeps the source path" "exit $CLI_EXIT"
fi

# ---------------------------------------------------------------------------
section "5. CLI end-to-end: error handling"

# expect_error NAME STDERR_PATTERN ARGS...: non-zero exit and a message on stderr
expect_error() {
    local name="$1" pattern="$2"; shift 2
    run_cli "$@"
    if [ "$CLI_EXIT" -eq 0 ]; then
        fail "$name" "expected a non-zero exit, got 0"
    elif ! grep -Eq "$pattern" "$CLI_ERR"; then
        fail "$name" "stderr does not match /$pattern/: $(cat "$CLI_ERR")"
    else
        pass "$name"
    fi
}

run_cli
if [ "$CLI_EXIT" -ne 0 ] && grep -q "^Usage: x2md" "$CLI_OUT"; then
    pass "5.1 no arguments prints usage and exits non-zero"
else
    fail "5.1 no arguments prints usage and exits non-zero" "exit $CLI_EXIT: $(head -n 2 "$CLI_OUT")"
fi

expect_error "5.2 missing input file" "no such file" -output "$WORK/x.md" "$WORK/does-not-exist.pdf"

echo "plain text" >"$WORK/note.txt"
expect_error "5.3 unsupported file type" "unsupported file type" -output "$WORK/x.md" "$WORK/note.txt"

echo "this is not a PDF" >"$WORK/broken.pdf"
expect_error "5.4 corrupt PDF" "." -output "$WORK/x.md" "$WORK/broken.pdf"

echo "this is not a DOCX" >"$WORK/broken.docx"
expect_error "5.5 corrupt DOCX" "." -output "$WORK/x.md" "$WORK/broken.docx"

expect_error "5.6 unknown flag" "flag provided but not defined" -no-such-flag testdata/pages.pdf

expect_error "5.7 -output with a directory" "cannot be used with directories" -output "$WORK/x.md" testdata

expect_error "5.8 unsupported file type without -output" "unsupported file type" "$WORK/note.txt"

# ---------------------------------------------------------------------------
section "6. Cleanup and final verification"

TREE_AFTER="$(git status --porcelain 2>/dev/null)"
if [ "$TREE_BEFORE" = "$TREE_AFTER" ]; then
    pass "6.1 working tree is unchanged by the test run"
else
    fail "6.1 working tree is unchanged by the test run" "$(diff <(echo "$TREE_BEFORE") <(echo "$TREE_AFTER"))"
fi

summary

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
exit 0
