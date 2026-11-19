"""Generates site/wall.svg: a seamless tile of rough dungeon stonework.

Run from the repo root: python3 scripts/site-wall.py > site/wall.svg. A fixed seed keeps the output stable.
"""
import random

R = random.Random(7)
W, H = 720, 440
MORTAR = 2.6  # half the gap between stones


def jit(v, a):
    return v + R.uniform(-a, a)


def stone(x, y, w, h):
    """An irregular, worn polygon inside the cell (x, y, w, h)."""
    g = MORTAR
    x0, y0, x1, y1 = x + g, y + g, x + w - g, y + h - g
    pts = []

    def edge(ax, ay, bx, by, n):
        for i in range(n):
            t = i / n
            pts.append((jit(ax + (bx - ax) * t, 1.4), jit(ay + (by - ay) * t, 1.4)))

    # Chamfer each corner by a random amount, as if worn or chipped.
    c = [R.uniform(1.5, 7) for _ in range(4)]
    nx, ny = max(2, int(w / 28)), max(1, int(h / 22))
    edge(x0 + c[0], y0, x1 - c[1], y0, nx)
    edge(x1, y0 + c[1], x1, y1 - c[2], ny)
    edge(x1 - c[2], y1, x0 + c[3], y1, nx)
    edge(x0, y1 - c[3], x0, y0 + c[0], ny)
    return pts


def path(pts):
    return "M" + " L".join(f"{px:.1f} {py:.1f}" for px, py in pts) + "Z"


def crack(x, y, w, h):
    px, py = R.uniform(x + w * 0.2, x + w * 0.8), y + MORTAR + R.uniform(0, 3)
    pts = [(px, py)]
    for _ in range(R.randint(3, 6)):
        px += R.uniform(-9, 9)
        py += R.uniform(4, h / 4)
        if py > y + h - MORTAR:
            break
        pts.append((px, py))
    return pts


def shift(pts, dx):
    return [(a + dx, b) for a, b in pts]


# Rows of uneven height that add up to the tile's height exactly.
heights, total = [], 0
while total < H - 70:
    hgt = R.randint(46, 66)
    heights.append(hgt)
    total += hgt
heights.append(H - total)

stones, cracks = [], []
y = 0
for hgt in heights:
    # Widths that add up to the tile's width exactly, so the row wraps around
    # with no stone overlapping another. A leftover too narrow for a stone of
    # its own goes to the last one.
    widths, left = [], W
    while left > 0:
        w = R.choice([R.uniform(70, 110), R.uniform(100, 160), R.uniform(140, 200)])
        if left - w < 45:
            w = left
        widths.append(w)
        left -= w
    x = R.uniform(-80, 0)  # start before 0 so the joints don't line up with the tile edge
    for w in widths:
        pts = stone(x, y, w, hgt)
        lines = [crack(x, y, w, hgt)] if R.random() < 0.16 else []
        lightness = R.uniform(0.0, 1.0)
        # A stone crossing the tile's edge is drawn again, the same shape, on the other side.
        for dx in (0, W, -W):
            if x + dx < W and x + dx + w > 0:
                stones.append(([(a + dx, b) for a, b in pts], lightness))
                cracks.extend(shift(c, dx) for c in lines)
        x += w
    y += hgt

out = [f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" viewBox="0 0 {W} {H}">']
out.append("""<defs>
<filter id="soft" x="-5%" y="-5%" width="110%" height="110%">
  <feGaussianBlur stdDeviation="2.2"/>
</filter>
<filter id="grit" x="0" y="0" width="100%" height="100%">
  <feTurbulence type="fractalNoise" baseFrequency=".9" numOctaves="2" seed="3" stitchTiles="stitch"/>
  <feColorMatrix values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 -1.6 1.05"/>
</filter>
<filter id="damp" x="0" y="0" width="100%" height="100%">
  <feTurbulence type="fractalNoise" baseFrequency=".008 .02" numOctaves="3" seed="11" stitchTiles="stitch"/>
  <feColorMatrix values="0 0 0 0 .02  0 0 0 0 .01  0 0 0 0 0  0 0 0 -2.4 1.2"/>
</filter>
</defs>""")
# Mortar: the dark gaps between the stones.
out.append(f'<rect width="{W}" height="{H}" fill="#000" opacity=".42"/>')
for pts, l in stones:
    r, g, b = int(52 + 26 * l), int(39 + 19 * l), int(29 + 13 * l)
    d = path(pts)
    out.append(f'<path d="{d}" fill="rgb({r} {g} {b})" fill-opacity=".62"/>')
# Each stone darkens evenly toward its edges: a blurred dark outline, the inner half
# of which falls on the stone (the outer half is lost in the mortar). Every stone
# gets the same treatment, so they sit flush and only the mortar reads as recessed.
out.append('<g filter="url(#soft)" fill="none" stroke="#000" stroke-opacity=".38" stroke-width="7" stroke-linejoin="round">')
for pts, _ in stones:
    out.append(f'<path d="{path(pts)}"/>')
out.append('</g>')
# Grime and damp patches over everything, then fine grit.
out.append(f'<rect width="{W}" height="{H}" filter="url(#damp)" opacity=".55"/>')
out.append(f'<rect width="{W}" height="{H}" filter="url(#grit)" opacity=".5"/>')
for c in cracks:
    d = "M" + " L".join(f"{a:.1f} {b:.1f}" for a, b in c)
    out.append(f'<path d="{d}" fill="none" stroke="#000" stroke-opacity=".55" stroke-width="1.1" stroke-linecap="round" stroke-linejoin="round"/>')
out.append("</svg>")
print("\n".join(out))
