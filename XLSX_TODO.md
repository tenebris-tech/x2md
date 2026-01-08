# XLSX -> MD Commercial Quality TODO

This list tracks the remaining work to reach audit-grade XLSX conversion fidelity.

- [x] Emit explicit MD structure: sheet headings, table/range headings with A1 refs, and row/col indices.
- [x] Use table definitions (`xl/tables/*.xml` + worksheet rels) to drive range extraction.
- [x] Handle formulas (`<f>`) and cached values (`<v>`), emitting `formula => value` when present.
- [x] Expand number format handling (built-in + custom format codes) for percent/currency/accounting/text.
- [x] Handle merged cells (`<mergeCells>`) by repeating value or marking `[merged]` spans deterministically.
- [x] Respect hidden rows/columns from sheet metadata (option to include/exclude with markers).
- [x] Preserve error cell values (`#N/A`, `#VALUE!`, etc.) explicitly in output.
