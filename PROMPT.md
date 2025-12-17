# How to Resume This Project with Claude

Copy and paste the following prompt when starting a new Claude Code session:

---

## Prompt to Use

```
I'm resuming work on the x2md project - a pure Go PDF/DOCX to Markdown converter.

Please read these files to get up to speed:
1. STATUS.md - Current project state, known issues, next steps
2. CLAUDE.md - Architecture and coding guidelines
3. TODO.md - Feature checklist and limitations

After reading, run `go test ./...` to verify the build, then tell me:
1. Current project status
2. Any open PRs or uncommitted changes
3. The highest priority task to work on next

The goal is 100% robust conversion from PDF and DOCX to Markdown.
```

---

## What Claude Will Find

### Branch Status
- **Current branch**: `nested-lists`
- **Open PR**: #1 merging `nested-lists` → `alpha`

### Build Status
- All 71 tests passing
- `go build` and `go vet` clean

### Priority Task
Fix header over-detection in `pdf2md/transform/detect_headers.go`:
- **Problem**: Simple PDFs like `basic-text.pdf` have all lines marked as H2
- **Cause**: Algorithm assumes font size variation = headings
- **Test**: `./x2md private/basic-text.pdf && head -30 private/basic-text.md`

### Test Files
Located in `private/`:
- `CPP_ND_V3.0E.pdf` - Complex 245-page document (works well)
- `footnotes.pdf` - Footnote testing (works well)
- `basic-text.pdf` - Simple document (has header detection bug)

---

## Alternative Shorter Prompts

### For continuing development:
```
Resume x2md project. Read STATUS.md, CLAUDE.md, TODO.md. Run tests.
Continue with the next priority task (header over-detection fix).
```

### For a specific task:
```
Resume x2md project. Read STATUS.md and CLAUDE.md.
I want to [describe specific task].
```

### For code review:
```
Resume x2md project. Read STATUS.md and CLAUDE.md.
Review PR #1 (nested-lists → alpha) and check for any issues.
```

---

## Key Documentation Files

| File | Purpose |
|------|---------|
| `STATUS.md` | Full project state, test commands, known issues, next steps |
| `CLAUDE.md` | Architecture, package structure, coding guidelines |
| `TODO.md` | Feature checklist, limitations, QA checklist |
| `NOTES.md` | PDF/DOCX format technical details |
| `test-plan.md` | Manual image extraction test plan |
| `README.md` | User-facing documentation |

---

## Quick Verification Commands

```bash
# Verify build
go build && go vet ./... && go test ./...

# Test conversions
./x2md private/CPP_ND_V3.0E.pdf    # Should produce good output
./x2md private/basic-text.pdf       # Shows the header bug

# Check git status
git status
git log --oneline -5
gh pr list
```
