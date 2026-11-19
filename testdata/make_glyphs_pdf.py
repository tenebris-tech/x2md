# ******************************************************************************
# * Copyright (c) 2026 Tenebris Technologies Inc.                              *
# * Please see LICENSE file for details.                                       *
# ******************************************************************************

"""Generates glyphs.pdf: a page that draws every glyph with its own show-text
operation and no space characters, the way some PDF producers do.

    python3 make_glyphs_pdf.py > glyphs.pdf
"""
import sys

# Helvetica advance widths (1/1000 em) for the characters used below
WIDTHS = {' ': 278, "'": 191, '.': 278}
WIDTHS.update(dict.fromkeys('abdeghnopqu', 556))
WIDTHS.update(dict.fromkeys('cksvxyz', 500))
WIDTHS.update(dict.fromkeys('ijl', 222))
WIDTHS.update({'f': 278, 't': 278, 'r': 333, 'm': 833, 'w': 722, 'T': 611, 'A': 667, 'E': 667, 'R': 722})

LINES = [
    "The dog's vacation was approved by the committee.",
    "Evidence of the review is kept for seven years.",
    "Access rights are removed when a person leaves.",
]
SIZE = 12

ops = []
y = 700
for line in LINES:
    x = 72.0
    for ch in line:
        if ch != ' ':  # spaces are gaps, never glyphs
            ops.append(f"BT /F0 1 Tf {SIZE} 0 0 {SIZE} {x:.3f} {y} Tm ({ch}) Tj ET")
        x += WIDTHS[ch] * SIZE / 1000
    y -= 18
content = "\n".join(ops).encode()

first, last = 32, 122
widths = " ".join(str(WIDTHS.get(chr(c), 500)) for c in range(first, last + 1))
objects = [
    b"<< /Type /Catalog /Pages 2 0 R >>",
    b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
    b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R "
    b"/Resources << /Font << /F0 5 0 R >> >> >>",
    b"<< /Length %d >>\nstream\n" % len(content) + content + b"\nendstream",
    (f"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding "
     f"/FirstChar {first} /LastChar {last} /Widths [{widths}] >>").encode(),
]

out = bytearray(b"%PDF-1.4\n")
offsets = []
for i, obj in enumerate(objects, 1):
    offsets.append(len(out))
    out += b"%d 0 obj\n" % i + obj + b"\nendobj\n"
xref = len(out)
out += b"xref\n0 %d\n0000000000 65535 f \n" % (len(objects) + 1)
for off in offsets:
    out += b"%010d 00000 n \n" % off
out += b"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n" % (len(objects) + 1, xref)
sys.stdout.buffer.write(bytes(out))
