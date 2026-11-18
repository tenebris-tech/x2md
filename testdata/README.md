# Test fixtures

Small synthetic documents used by the end-to-end tests in `cli/`. None of them
contains real data.

| File | What it exercises | How it was made |
|------|-------------------|-----------------|
| `pages.pdf` | Page markers: 7 pages, page 3 blank, a table running over pages 5-7 | HTML printed to PDF with headless Chromium |
| `glyphs.pdf` | Word spacing when every glyph is drawn separately with no space characters | `python3 make_glyphs_pdf.py > glyphs.pdf` |
| `procedure.docx` | Paragraph markers: headings, a list, an image, a 32-row table, a header and footer, a landscape section | python-docx |
| `register.xlsx` | Sheet, range and row locators: two sheets, two blocks on the first | openpyxl |
