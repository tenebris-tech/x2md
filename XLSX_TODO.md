# XLSX -> MD Commercial Quality TODO

This list tracks the remaining work to reach audit-grade XLSX conversion fidelity.

- [ ] Emit explicit MD structure: sheet headings, table/range headings with A1 refs, and row/col indices.
- [ ] Use table definitions (`xl/tables/*.xml` + worksheet rels) to drive range extraction.
- [ ] Handle formulas (`<f>`) and cached values (`<v>`), emitting `formula => value` when present.
- [ ] Expand number format handling (built-in + custom format codes) for percent/currency/accounting/text.
- [ ] Handle merged cells (`<mergeCells>`) by repeating value or marking `[merged]` spans deterministically.
- [ ] Respect hidden rows/columns from sheet metadata (option to include/exclude with markers).
- [ ] Preserve error cell values (`#N/A`, `#VALUE!`, etc.) explicitly in output.
