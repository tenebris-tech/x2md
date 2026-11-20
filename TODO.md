# TODO

Handover notes for continuing the source-location work on another machine.
Written 2026-11-20, after pull request #3 was merged to `main`.

Delete this file when the items below are done or moved elsewhere.

## Goal

x2md converts documents to Markdown so an LLM can search and read them. The
application that embeds x2md is used for audits: an AI assistant assesses
evidence and writes findings. The auditor should be able to click a finding and
have the **original** evidence open, in a browser pane beside the report, at
the passage the finding cites.

Requirements the owner set along the way:

- Show the original file, never a converted rendition of it.
- The viewer runs in a browser.
- x2md is primarily a library embedded in the application. No external
  dependencies, and nothing that is not pure Go.
- Follow the development standards in https://github.com/securityguy/coding
  (see "Development Standards" in `CLAUDE.md`).

## How a citation is meant to work

Each finding stores three things: the source path, the nearest location marker
above the passage, and a short verbatim quote. The viewer opens the original,
uses the marker to get close, then finds and highlights the quote.

| Format | What x2md emits | How the viewer uses it |
|--------|-----------------|------------------------|
| All | `<!-- source: /absolute/path -->` as the first line (file-based conversions only) | Identifies the original file |
| PDF | `<!-- page N -->` before each page; `<!-- pages N-M -->` before a table spanning pages | Open at physical page N, highlight the quote |
| DOCX | `<!-- para N -->` before each heading and table, and at least every 10 paragraphs | Scroll to the Nth paragraph of the rendered document, then find the quote from there |
| XLSX | `### Range: Sheet!A1:D8` headings and a `Row` column (already existed) | Select the sheet and range |

The quote has to be matched loosely: ignore whitespace and strip Markdown marks
(`**bold**`, `_italic_`, table pipes), because x2md adds those.

## What is done (merged in pull request #3)

- Source path header for PDF, DOCX and XLSX (`internal/source/`).
- PDF page markers, using the physical page number. Stripped blank pages do
  not shift the numbering.
- DOCX paragraph markers. The count is every `w:p` in document order,
  including table cells and empty paragraphs, skipping `mc:Fallback`.
- CLI flag `-no-location-markers`; library options `pdf2md.WithPageMarkers`
  and `docx2md.WithParagraphMarkers`.
- Fix: PDFs mixing scanned and text pages now emit scanned pages in page order.
- Fix: PDFs that draw one glyph at a time with no space characters no longer
  lose their word spaces (`mergeGlyphRuns` in `pdf2md/pdf/extractor.go`).
- Fix: DOCX decimal list numbering past item 9 (item 10 was rendered as `0.`).
- Removed the `PageSeparator` options, which had no effect.
- `make test` gate (`Makefile`, `test.sh`), tests for every package, fixtures
  in `testdata/`, copyright headers, whole tree `gofmt`-formatted.
- Last run of `make test` before the merge: 44 checks, 44 passed, 0 failed,
  0 skipped.

## Decisions already made (do not reopen)

- No `app/` identity package and no `--version`: x2md is primarily a library.
- No `make install` target, for the same reason.
- No GitHub Actions workflow: the owner's build system runs `make test`.
- DOCX is not converted to PDF. A pure-Go renderer (omnidoc) was tried and
  dropped headers, footers, list numbers and non-Latin text, and the original
  must be shown anyway.
- Only omnidoc was run; UniOffice was looked at and not tested. No rendering
  library is needed now.
- DOCX gets paragraph numbers, not page numbers: a browser renderer does not
  paginate like Word.

## What is left

### 1. Verify on real documents (do this first)

Everything was verified on generated documents only. The `private/` corpus was
not available, so nothing here has been run against real Word-authored files
or the owner's real PDFs.

1. `make test` - confirm the gate passes on this machine. It needs Go and a C
   compiler (for the race detector).
2. `make test-private` - runs `integration_test.go` against `private/`. It
   fails, naming the file, if a document it expects is missing. The file names
   it expects are listed in `integration_test.go`.
3. Convert a spread of real DOCX files and check the paragraph markers. Things
   most likely to break the count: text boxes, content controls, tracked
   changes, footnotes, nested tables, documents not last saved by Word.
   Appendix A has a script that checks every marker against a browser
   rendering.
4. Convert a spread of real PDFs and compare with the output from before this
   work (commit `2abdd91`, the last one on `main` before pull request #3)
   using `-no-location-markers`, so the only differences are real ones.
   Appendix B has a script that scores word recovery against `pdftotext`.
5. Check the PDF reference cases in `CLAUDE.md` ("PDF Testing") still hold for
   `CPP_ND_V3.0E.pdf`. On the copy downloaded during this work the output was
   byte-identical before and after the glyph fix.

Not verified at all on real files: the `<!-- pages N-M -->` range marker for
tables that continue across pages, and scanned-page ordering. Both have unit
tests only.

### 2. Lists numbered through a Word style lose their numbers

A paragraph is treated as a list item only when it carries `w:numPr` itself.
When the numbering comes from the paragraph style ("List Number", "List
Bullet"), the output is plain paragraphs with no number or bullet.

- Reproduce: `testdata/procedure.docx` has "Step 1" to "Step 5" in style
  "List Number" and "Nested bullet" in "List Bullet 2".
- Fix: when a paragraph has no direct `numPr`, resolve it from the style,
  following `basedOn`. Code is in `docx2md/docx/extractor.go`
  (`parseParagraphElement`, `createLineItem`) and `docx2md/docx/styles.go`.
- Do not change paragraph counting. `cli/main_test.go` and `test.sh` assert
  exact paragraph numbers for this fixture.
- Options: fix it (recommended), or leave it if real evidence mostly numbers
  lists directly. Item 1 will show which.

### 3. An unsupported file is silently ignored

`x2md note.txt` without `-output` exits 0 and prints nothing. With `-output`
it reports `unsupported file type`. `test.sh` check 5.3 covers only the
`-output` form.

- Options: make a single unsupported file an error in both cases
  (recommended; directory conversions keep skipping unsupported files), or
  leave it if x2md is only ever called as a library.

### 4. Headers and footers

`README.md` lists a DOCX option `WithExtractHeadersFooters` that does not
exist. `docx2md/converter.go` has an `ExtractHeadersFooters` field and the
code to use it, but nothing can set it, so header and footer text never
reaches the Markdown and cannot be cited.

- Option A: add the option, off by default. Worth it if headers carry things
  an auditor cites (document ID, version, classification).
- Option B: delete the README line and the unused code.
- Undecided. Item 1 will show whether real evidence has meaningful headers.

### 5. The viewer (in the application, not in x2md)

Nothing is built. x2md only supplies the markers.

- PDF: an embedded viewer such as PDF.js can open at a page and highlight a
  search hit. Whether Chrome's built-in viewer supports search-by-link was
  not verified.
- DOCX: `docx-preview` 0.4.1 (Apache-2.0, needs `jszip`) rendered the fixture
  in about 170 ms. The Nth `section.docx > article p` element was the Nth
  paragraph x2md counted, on both fixtures tried. Header, footer, list
  numbers, table borders and Japanese text all appeared. It does not paginate
  like Word and substitutes fonts, so an "Open in Word" button is still worth
  having. Only one library was tried.
- XLSX: not looked at.
- A browser cannot open `file://` paths from a page served over http. The
  viewer has to serve the evidence files itself.
- Suggested for audit use, not done: put a SHA-256 of the original in the
  header next to the source path, so the viewer can tell if evidence changed.

Options for x2md's part:

- A: write a short integration note in `README.md` describing how a consumer
  resolves a citation, including the quote-matching rules above.
- B: add a small reference viewer under `examples/`. This brings JavaScript
  dependencies into the repository and needs the owner's approval.
- C: nothing; the README sections "Source Path", "Page Markers" and
  "Paragraph Markers" already describe the output.

## Smaller things noticed, not confirmed as problems

- DOCX output has doubled spaces around formatted runs, for example
  `defines  **mandatory**  steps`. Harmless if quotes are matched ignoring
  whitespace.
- `multicolumn.pdf` (a third-party test file) once produced different output
  on two runs of the same build. Seen once and not reproduced.
- In `pdf2md/pdf/extractor.go`, text shown with the array operator (`TJ`)
  computes its font size from the page transform only, while single strings
  (`Tj`) also apply the text matrix. This looks inconsistent but no wrong
  output was traced to it.
- A paragraph nested inside another paragraph (a text box) ends the outer
  paragraph early in `parseParagraphElement`. Counting is still correct; the
  text after the box may be lost. Not tested on a real document.

## How to continue

```bash
git checkout main && git pull
make test                 # must pass before and after any change
make test-private         # needs private/
make                      # test, then build bin/x2md
bin/x2md -output out.md some.docx
bin/x2md -no-location-markers -output plain.md some.pdf
```

Standards to follow are summarised in `CLAUDE.md`. The ones that bite:

- New test sections go in `test.sh` using its numbering. `make test` must
  never modify the working tree.
- No silent skips; a missing prerequisite is a failure.
- Copyright header on every source file. `gofmt`-clean.
- No new dependencies without approval.
- The project is unreleased: no legacy code, no backward compatibility.

## Appendix A: check DOCX markers against a browser rendering

Throwaway script, not part of the repository's tooling. Needs Node, with
`npm install docx-preview jszip playwright` in an empty directory outside the
repository. Save as `check.js` there and run:

```bash
bin/x2md -output out.md some.docx
node check.js some.docx out.md
```

It prints how many markers landed on the right on-screen paragraph and lists
any that did not.

```js
// For every "para N" marker, check that paragraph N in the browser rendering
// starts with the text of the Markdown block that follows the marker.
const { chromium } = require('playwright'); const fs = require('fs');
(async () => {
  const md = fs.readFileSync(process.argv[3], 'utf8').split('\n');
  const b = await chromium.launch(); const p = await b.newPage();
  await p.setContent('<div id=c></div>');
  await p.addScriptTag({ path: 'node_modules/jszip/dist/jszip.min.js' });
  await p.addScriptTag({ path: 'node_modules/docx-preview/dist/docx-preview.min.js' });
  const paras = await p.evaluate(async (b64) => {
    await docx.renderAsync(Uint8Array.from(atob(b64), c => c.charCodeAt(0)), document.getElementById('c'));
    return [...document.querySelectorAll('section.docx > article p')].map(e => e.textContent);
  }, fs.readFileSync(process.argv[2]).toString('base64'));
  const norm = s => s.replace(/[\s*_#|\\-]+/g, '').replace(/^\d+\./, '');
  let ok = 0, bad = 0;
  for (let i = 0; i < md.length; i++) {
    const m = md[i].match(/^<!-- para (\d+) -->$/); if (!m) continue;
    let j = i + 1; while (md[j] !== undefined && md[j].trim() === '') j++;
    const want = norm(md[j]), got = norm(paras[m[1] - 1] || '');
    if (got && want.startsWith(got.slice(0, 12))) ok++; else { bad++; console.log('MISMATCH para', m[1], JSON.stringify(md[j].slice(0, 40)), 'vs', JSON.stringify((paras[m[1] - 1] || '').slice(0, 40))); }
  }
  console.log('browser paragraphs:', paras.length, 'markers ok:', ok, 'mismatched:', bad);
  await b.close();
})();
```

## Appendix B: score PDF word recovery

Throwaway script. Needs `pdftotext` (poppler). Put PDFs in a directory,
convert each to `<outdir>/<name>.pdf.md` with `-no-location-markers
-no-images`, then run `python3 score.py <outdir>` from that directory. It
reports the share of words of four or more letters from `pdftotext` that also
appear in the x2md output.

Scores recorded before and after the glyph fix: two glyph-by-glyph PDFs went
from 10.5% and 34.1% to 100% and 95.2%; the other files scored between 81%
and 100% and were byte-identical before and after.

```python
import sys,re,subprocess,glob,os
out=sys.argv[1]
tok=lambda s:[w for w in re.findall(r"[A-Za-z]{4,}",s)]
tot_h=tot_n=0
for f in sorted(glob.glob('*.pdf')):
    ref=tok(subprocess.run(['pdftotext',f,'-'],capture_output=True,text=True,errors='ignore').stdout)
    try: got=set(tok(open(f'{out}/{f}.md',errors='ignore').read()))
    except FileNotFoundError: print(f'{f:32} MISSING'); continue
    if not ref: print(f'{f:32} no reference text'); continue
    hit=sum(w in got for w in ref); tot_h+=hit; tot_n+=len(ref)
    print(f'{f:32} {100*hit/len(ref):5.1f}%  ({len(ref)} words)')
print(f'{"TOTAL":32} {100*tot_h/tot_n:5.1f}%')
```
