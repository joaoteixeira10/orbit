"""Bake the UBonito proxy crests: replace the <text>U</text> (UnifrakturCook 700)
with the glyph as a vector path, so the SVG renders the same as a favicon or an
<img> (neither loads web fonts). Strips the C2PA manifest (no longer valid once
the file changes).

  docker run --rm -v "$PWD":/src -w /src python:3.12-slim sh -c \\
    'pip -q install fonttools brotli && python scripts/bake-crests.py'
"""
import re
import sys
from fontTools.ttLib import TTFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen

FONT = "web/public/fonts/unifrakturcook-700.woff2"
SRC = "proxy-assets/proxy/ubonito/proxy-ubonito-{}.svg"
DST = "web/public/dev-themes/proxy-ubonito-{}.svg"

font = TTFont(FONT)
upm = font["head"].unitsPerEm
gs = font.getGlyphSet()
glyph = font.getBestCmap()[ord("U")]
adv = font["hmtx"][glyph][0]

TEXT = re.compile(r'<text x="([\d.]+)" y="([\d.]+)"([^>]*)>U</text>')
SIZE = re.compile(r'font-size="([\d.]+)"')
for variant in ["dark", "bone", "blue", "gold", "red"]:
    svg = open(SRC.format(variant)).read()
    svg = re.sub(r"<metadata>.*?</metadata>", "", svg, flags=re.S)
    svg = svg.replace(' xmlns:c2pa="http://c2pa.org/manifest"', "")
    m = TEXT.search(svg)
    if not m:
        sys.exit(f"{variant}: no <text>U</text>")
    x, y, attrs = float(m.group(1)), float(m.group(2)), m.group(3)
    size = float(SIZE.search(attrs).group(1))
    s = size / upm
    x0 = x - adv * s / 2  # text-anchor="middle"
    pen = SVGPathPen(gs, ntos=lambda v: f"{v:.2f}".rstrip("0").rstrip("."))
    gs[glyph].draw(TransformPen(pen, (s, 0, 0, -s, x0, y)))
    keep = re.sub(r'\s(font-family|font-weight|font-size|text-anchor)="[^"]*"', "", attrs)
    svg = svg[: m.start()] + f'<path d="{pen.getCommands()}"{keep}/>' + svg[m.end():]
    open(DST.format(variant), "w").write(svg)
    print(variant, len(svg), "bytes")
