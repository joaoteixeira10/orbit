"""SHARED — from ../blueprint/shared/dev-themes (copied by blueprint/sync-themes.sh).

Bake a project's UBonito crests (the developer themes' marks) from the owner's
design pack (../assets, out of git — the icons are the owner's; this only
converts them) into web/public/dev-themes/crest-<ink>…:

- the SVG: a <text>U</text> (UnifrakturCook 700), if the pack still has one,
  becomes the glyph as a vector path, so the SVG renders the same as a favicon
  or an <img> (neither loads fonts); packs with the U already in outlines are
  taken as they are. Any metadata (the C2PA manifest) is dropped;
- the PNGs are copied as delivered: -192 and -512 (install icons), -touch-180
  (iPhone / iPad home screen) and -maskable-512 (Android).

  make crests     (the project's Makefile passes its pack, e.g.
                   ../assets/orbit/ubonito-orbit)
  python scripts/bake-crests.py <pack prefix> [inks…]
"""
import os
import re
import shutil
import sys
from fontTools.ttLib import TTFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen

FONT = "web/public/fonts/unifrakturcook-700.woff2"
DST = "web/public/dev-themes/crest-{}"
INKS = ["dark", "bone", "blue", "gold", "red", "green"]
PNGS = ["192", "512", "touch-180", "maskable-512"]

if len(sys.argv) < 2:
    sys.exit(__doc__)
src = sys.argv[1]
inks = sys.argv[2:] or INKS

os.makedirs(os.path.dirname(DST), exist_ok=True)
font = TTFont(FONT)
upm = font["head"].unitsPerEm
gs = font.getGlyphSet()
glyph = font.getBestCmap()[ord("U")]
adv = font["hmtx"][glyph][0]

TEXT = re.compile(r'<text x="([\d.]+)" y="([\d.]+)"([^>]*)>U</text>')
SIZE = re.compile(r'font-size="([\d.]+)"')
for ink in inks:
    svg = open(f"{src}-{ink}.svg").read()
    svg = re.sub(r"<metadata>.*?</metadata>", "", svg, flags=re.S)
    svg = svg.replace(' xmlns:c2pa="http://c2pa.org/manifest"', "")
    m = TEXT.search(svg)
    if not m:
        if "<text" in svg:
            sys.exit(f"{ink}: a <text> that isn't the U — ask the owner for outlines")
        open(DST.format(ink) + ".svg", "w").write(svg)  # already in outlines
        for png in PNGS:
            shutil.copyfile(f"{src}-{ink}-{png}.png", DST.format(ink) + f"-{png}.png")
        print(ink, len(svg), "bytes (outlines)")
        continue
    x, y, attrs = float(m.group(1)), float(m.group(2)), m.group(3)
    size = float(SIZE.search(attrs).group(1))
    s = size / upm
    x0 = x - adv * s / 2  # text-anchor="middle"
    pen = SVGPathPen(gs, ntos=lambda v: f"{v:.2f}".rstrip("0").rstrip("."))
    gs[glyph].draw(TransformPen(pen, (s, 0, 0, -s, x0, y)))
    keep = re.sub(r'\s(font-family|font-weight|font-size|text-anchor)="[^"]*"', "", attrs)
    svg = svg[: m.start()] + f'<path d="{pen.getCommands()}"{keep}/>' + svg[m.end():]
    open(DST.format(ink) + ".svg", "w").write(svg)
    for png in PNGS:
        shutil.copyfile(f"{src}-{ink}-{png}.png", DST.format(ink) + f"-{png}.png")
    print(ink, len(svg), "bytes")
