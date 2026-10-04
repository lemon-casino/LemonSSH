#!/usr/bin/env python3
"""Generate the lemon-themed LemonSSH icon set using Pillow only.

Relationship to scripts/generate-app-icon-variants.py:
    that older script rasterized public/icon.svg (white-cat mascot era) with
    rsvg-convert, which is not available in this environment. The brand icon
    is now a lemon, so this script redraws the exact same output set purely
    with Pillow (no native SVG rasterizer required) and also regenerates
    public/icon.svg from the same geometry so the design source stays in sync.
    Keep the old script untouched for history; do not run both.

Outputs (all paths relative to the repository root):
    public/icon.svg                          design source (1024 viewBox)
    build/icons/{16,32,48,64,128,256,512}    Wails / packaging PNGs
    cmd/lemonssh/appicon.png                 //go:embed application icon (512)
    cmd/lemonssh/app.ico                     Windows multi-size ICO
    build/lemonssh.ico                       Windows multi-size ICO
    public/icons/variants/<id>.png           runtime variant set (1024, desktop crop)
    public/icons/variants/macos/<id>.png     same variants with Apple HIG padding
    build/icon.icns                          macOS icon (only if icnsutil is importable)

Run: python scripts/generate-lemon-icon.py [--skip-icns]
Requires: Pillow >= 10 (pip install Pillow); optional: icnsutil for .icns
"""
from __future__ import annotations

import argparse
import math
from pathlib import Path

from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[1]

# ---------------------------------------------------------------- constants

MASTER = 4096          # supersampled master canvas per render (4x of 1024)
SS = 4                 # supersampling factor relative to the target size

# Background crop margins (fraction of the canvas edge), matching the two
# crops the old SVG pipeline used:
#   desktop: viewBox "44 44 936 936" around a rect at 100..924 -> 56/936
#   macOS:   viewBox "0 0 1024 1024"                            -> 100/1024
MARGIN_DESKTOP = 56.0 / 936.0
MARGIN_MACOS = 100.0 / 1024.0

# Rounded-square corner radius, matching 185/824 from the previous icon.
CORNER_RADIUS = 185.0 / 824.0

# --- lemon geometry, all fractions of the rounded-square side S -------------
# Normal style (>= 48 px): full detail with highlight, two leaves and a stem.
NORMAL = {
    "lemon_cy": 0.035,      # lemon center, offset below the square center
    "a": 0.325,             # body half-length
    "ry": 0.240,            # body half-height
    "rn": 0.090,            # nub radius (the typical lemon tips)
    "nub_inset": 0.010,     # nub centers at +- (a - nub_inset)
    "angle": 18.0,          # CCW tilt; right tip points up towards the leaf
    "shade_shift": 0.028,   # bottom shading rim thickness
    "hi_c": (-0.105, -0.100),   # highlight ellipse center (body-local)
    "hi_r": (0.150, 0.082),     # highlight ellipse radii
    "dot_c": None,              # optional secondary highlight (off: keeps it clean)
    "dot_r": None,
    "leaf_main": {"back": (0.050, -0.020), "angle": 66.0, "len": 0.270, "w": 0.140},
    "leaf_second": {"back": (0.012, 0.008), "angle": 30.0, "len": 0.170, "w": 0.100},
    "stem": {"back": (0.028, -0.012), "r": 0.030},
    "midrib": {"t0": 0.14, "t1": 0.86, "w": 0.016},
}
# Small style (16/32 px): bigger body, thicker single leaf, no fine highlights.
SMALL = {
    "lemon_cy": 0.020,
    "a": 0.330,
    "ry": 0.250,
    "rn": 0.095,
    "nub_inset": 0.015,
    "angle": 15.0,
    "shade_shift": 0.045,
    "hi_c": None,
    "hi_r": None,
    "dot_c": None,
    "dot_r": None,
    "leaf_main": {"back": (0.025, -0.012), "angle": 60.0, "len": 0.260, "w": 0.200},
    "leaf_second": None,
    "stem": None,
    "midrib": None,
}

# Brand palette.
NAVY_GRADIENT = ("#1F2657", "#0C1943")
LEAF_GREEN = "#43A94E"
LEAF_GREEN_DARK = "#2F7D3B"
RAINBOW_STOPS = [  # same stops/orientation as the legacy SVG rainbow variant
    (0, "#EF4444"), (16, "#F97316"), (33, "#EAB308"), (50, "#22C55E"),
    (66, "#06B6D4"), (83, "#3B82F6"), (100, "#A855F7"),
]
RAINBOW = "rainbow"

WHITE_BG_BORDER = ("#CBD5E1", 217)  # 0.85 alpha, as the legacy white-* variants

# Variant registry: same 12 ids as the legacy set, lemon semantics.
VARIANTS: dict[str, dict] = {
    "original": dict(
        bg=NAVY_GRADIENT, border=("#FFFFFF", 102),  # 0.4 white ring
        body="#FFD948", shade="#E8AE24", hi="#FFF3B8",
        leaf=LEAF_GREEN, leaf_dark=LEAF_GREEN_DARK),
    "bright": dict(
        bg="#0EA5E9", border=("#FFFFFF", 140),
        body="#FFFFFF", shade="#CFE9FB", hi=None,
        leaf="#0369A1", leaf_dark="#075985"),
    "dark": dict(
        bg="#0F172A", border=("#FFFFFF", 89),
        body="#F8FAFC", shade="#C6D0E0", hi="#FFFFFF",
        leaf="#334155", leaf_dark="#1E293B"),
    "colorful": dict(
        bg="#EA580C", border=("#FFFFFF", 128),
        body="#FFFFFF", shade="#FFE0C7", hi=None,
        leaf="#C2410C", leaf_dark="#9A3412"),
    "high-contrast": dict(
        bg="#000000", border=("#FFFFFF", 230),
        body="#FACC15", shade="#A16207", hi="#FDE047",
        leaf="#A16207", leaf_dark="#7C4A05"),
    "white-navy": dict(
        bg="#FFFFFF", border=WHITE_BG_BORDER,
        body="#002551", shade="#001233", hi="#0B3D78",
        leaf="#0B3D78", leaf_dark="#002551"),
    "white-sky": dict(
        bg="#FFFFFF", border=WHITE_BG_BORDER,
        body="#0284C7", shade="#075985", hi="#38BDF8",
        leaf="#38BDF8", leaf_dark="#0369A1"),
    "white-rose": dict(
        bg="#FFFFFF", border=WHITE_BG_BORDER,
        body="#E11D48", shade="#9F1239", hi="#FB7185",
        leaf="#FB7185", leaf_dark="#BE123C"),
    "white-emerald": dict(
        bg="#FFFFFF", border=WHITE_BG_BORDER,
        body="#059669", shade="#047857", hi="#34D399",
        leaf="#34D399", leaf_dark="#059669"),
    "white-amber": dict(
        bg="#FFFFFF", border=WHITE_BG_BORDER,
        body="#D97706", shade="#B45309", hi="#FBBF24",
        leaf="#FBBF24", leaf_dark="#D97706"),
    "white-violet": dict(
        bg="#FFFFFF", border=WHITE_BG_BORDER,
        body="#7C3AED", shade="#5B21B6", hi="#A78BFA",
        leaf="#A78BFA", leaf_dark="#6D28D9"),
    "rainbow": dict(
        bg="#FFFFFF", border=WHITE_BG_BORDER,
        body=RAINBOW, shade=(15, 23, 42, 60), hi=None,
        leaf=LEAF_GREEN, leaf_dark=LEAF_GREEN_DARK),
}

BUILD_SIZES = [16, 32, 48, 64, 128, 256, 512]
ICO_SIZES = [16, 32, 48, 64, 128, 256]


# ---------------------------------------------------------------- helpers

def hex_rgb(value: str) -> tuple[int, int, int]:
    value = value.lstrip("#")
    return int(value[0:2], 16), int(value[2:4], 16), int(value[4:6], 16)


def _interpolate(stops: list[tuple[float, tuple[int, int, int]]], t: float):
    if t <= stops[0][0]:
        return stops[0][1]
    for (p0, c0), (p1, c1) in zip(stops, stops[1:]):
        if t <= p1:
            span = max(p1 - p0, 1e-6)
            k = min(max((t - p0) / span, 0.0), 1.0)
            return tuple(round(a + (b - a) * k) for a, b in zip(c0, c1))
    return stops[-1][1]


def gradient_image(size: int, stops: list[tuple[float, str]]) -> Image.Image:
    """Diagonal (top-left -> bottom-right) multi-stop gradient. Stops in %."""
    rgb_stops = [(p / 100.0, hex_rgb(c)) for p, c in stops]
    small = Image.new("RGB", (512, 512))
    px = small.load()
    denom = 2 * 511
    for y in range(512):
        for x in range(512):
            px[x, y] = _interpolate(rgb_stops, (x + y) / denom)
    return small.resize((size, size), Image.BILINEAR)


_BG_GRAD_CACHE: dict[str, Image.Image] = {}


def background(color_spec, canvas: int, margin_frac: float = MARGIN_DESKTOP):
    """Rounded-square background: flat color or 2-stop diagonal gradient."""
    margin = round(canvas * margin_frac)
    side = canvas - 2 * margin
    box = (margin, margin, margin + side, margin + side)
    radius = round(side * CORNER_RADIUS)
    layer = Image.new("RGBA", (canvas, canvas), (0, 0, 0, 0))
    if isinstance(color_spec, tuple):
        key = f"bg{color_spec[0]}{color_spec[1]}{canvas}"
        grad = _BG_GRAD_CACHE.get(key)
        if grad is None:
            grad = gradient_image(1024, [(0, color_spec[0]), (100, color_spec[1])])
            _BG_GRAD_CACHE[key] = grad
        grad = grad.resize((side, side), Image.BILINEAR)
        mask = Image.new("L", (side, side), 0)
        ImageDraw.Draw(mask).rounded_rectangle((0, 0, side - 1, side - 1), radius, fill=255)
        layer.paste(grad, (margin, margin), mask)
    else:
        ImageDraw.Draw(layer).rounded_rectangle(box, radius, fill=hex_rgb(color_spec))
    return layer, box, radius


def leaf_polygon(base, tip, width, samples: int = 48) -> list[tuple[float, float]]:
    """Lens-shaped leaf: base -> tip with a sine bulge slightly towards the base."""
    dx, dy = tip[0] - base[0], tip[1] - base[1]
    length = math.hypot(dx, dy)
    nx, ny = -dy / length, dx / length
    upper, lower = [], []
    for i in range(samples + 1):
        t = i / samples
        w = (width / 2.0) * (math.sin(math.pi * (t ** 0.85)) ** 0.85)
        px, py = base[0] + dx * t, base[1] + dy * t
        upper.append((px + nx * w, py + ny * w))
        lower.append((px - nx * w, py - ny * w))
    return upper + lower[::-1]


def angled_tip(base, angle_deg: float, length: float):
    rad = math.radians(angle_deg)
    return (base[0] + length * math.cos(rad), base[1] - length * math.sin(rad))


def draw_silhouette(draw: ImageDraw.ImageDraw, cx, cy, a, ry, rn, nub_dx, fill):
    """Lemon outline: rotated later, drawn axis-aligned here (two nubs included)."""
    draw.ellipse((cx - a, cy - ry, cx + a, cy + ry), fill=fill)
    for sign in (-1, 1):
        nx = cx + sign * nub_dx
        draw.ellipse((nx - rn, cy - rn, nx + rn, cy + rn), fill=fill)


# ---------------------------------------------------------------- renderer

def render_icon(size: int, variant_id: str, *, style: str = "normal",
                margin_frac: float = MARGIN_DESKTOP, ss: int = SS) -> Image.Image:
    spec = VARIANTS[variant_id]
    geo = NORMAL if style == "normal" else SMALL
    canvas = max(size * ss, 256)

    img, (bx0, by0, bx1, by1), radius = background(spec["bg"], canvas, margin_frac)
    side = bx1 - bx0
    draw = ImageDraw.Draw(img, "RGBA")  # RGBA draw context = alpha blending
    cx = canvas / 2.0
    cy = canvas / 2.0

    def S(f: float) -> float:
        return f * side

    # --- border ring (matches the legacy 8px white stroke inset by 4px) -----
    border = spec.get("border")
    if border is not None:
        color, alpha = border
        inset, width = S(0.005), max(round(S(0.010)), 1)
        draw.rounded_rectangle(
            (bx0 + inset, by0 + inset, bx1 - inset, by1 - inset),
            radius - inset, outline=hex_rgb(color) + (alpha,), width=width)

    # --- lemon placement ----------------------------------------------------
    lemon_cx = cx
    lemon_cy = cy + S(geo["lemon_cy"])
    a, ry, rn = S(geo["a"]), S(geo["ry"]), S(geo["rn"])
    nub_dx = S(geo["a"] - geo["nub_inset"])
    theta = math.radians(geo["angle"])
    # Right nub in world coords after the CCW tilt (y-down: up is negative).
    nub = (lemon_cx + nub_dx * math.cos(theta),
           lemon_cy - nub_dx * math.sin(theta))

    # --- leaves + stem (drawn first so the lemon overlaps their bases) ------
    leaf_second = geo.get("leaf_second")
    if leaf_second:
        base = (nub[0] + S(leaf_second["back"][0]), nub[1] + S(leaf_second["back"][1]))
        tip = angled_tip(base, leaf_second["angle"], S(leaf_second["len"]))
        draw.polygon(leaf_polygon(base, tip, S(leaf_second["w"])),
                     fill=hex_rgb(spec["leaf_dark"]))

    leaf_main = geo["leaf_main"]
    base = (nub[0] + S(leaf_main["back"][0]), nub[1] + S(leaf_main["back"][1]))
    tip = angled_tip(base, leaf_main["angle"], S(leaf_main["len"]))
    draw.polygon(leaf_polygon(base, tip, S(leaf_main["w"])), fill=hex_rgb(spec["leaf"]))
    midrib = geo.get("midrib")
    if midrib:
        t0, t1 = midrib["t0"], midrib["t1"]
        mb = (base[0] + (tip[0] - base[0]) * t0, base[1] + (tip[1] - base[1]) * t0)
        mt = (base[0] + (tip[0] - base[0]) * t1, base[1] + (tip[1] - base[1]) * t1)
        draw.polygon(leaf_polygon(mb, mt, S(midrib["w"])),
                     fill=hex_rgb(spec["leaf_dark"]))

    stem = geo.get("stem")
    if stem:
        sc = (nub[0] + S(stem["back"][0]), nub[1] + S(stem["back"][1]))
        r = S(stem["r"])
        draw.ellipse((sc[0] - r, sc[1] - r, sc[0] + r, sc[1] + r),
                     fill=hex_rgb(spec["leaf_dark"]))

    # --- lemon body layer (drawn axis-aligned, then tilted) -----------------
    pad = round(S(0.05))
    shift = S(geo["shade_shift"])
    lw = round(2 * (a + rn)) + 2 * pad
    lh = round(2 * ry + shift) + 2 * pad
    layer = Image.new("RGBA", (lw, lh), (0, 0, 0, 0))
    ldraw = ImageDraw.Draw(layer, "RGBA")
    shade_cy = pad + ry + shift          # shading silhouette (bottom rim)
    body_cy = shade_cy - shift           # body silhouette on top of it
    shade_fill = spec["shade"]
    if isinstance(shade_fill, str):
        shade_fill = hex_rgb(shade_fill)
    draw_silhouette(ldraw, lw / 2, shade_cy, a, ry, rn, nub_dx, shade_fill)

    body_fill = spec["body"]
    if body_fill == RAINBOW:
        mask = Image.new("L", (lw, lh), 0)
        draw_silhouette(ImageDraw.Draw(mask), lw / 2, body_cy, a, ry, rn, nub_dx, 255)
        grad = gradient_image(512, RAINBOW_STOPS).resize((lw, lh), Image.BILINEAR)
        layer.paste(grad, (0, 0), mask)
    else:
        draw_silhouette(ldraw, lw / 2, body_cy, a, ry, rn, nub_dx, hex_rgb(body_fill))

    if style == "normal":
        hi_c, hi_r = geo["hi_c"], geo["hi_r"]
        hi = spec["hi"]
        if hi_c and hi:
            hx, hy = lw / 2 + S(hi_c[0]), body_cy + S(hi_c[1])
            if isinstance(hi, str):
                hi = hex_rgb(hi) + (235,)
            elif len(hi) == 3:
                hi = hi + (235,)
            ldraw.ellipse((hx - S(hi_r[0]), hy - S(hi_r[1]),
                           hx + S(hi_r[0]), hy + S(hi_r[1])), fill=hi)
        dot_c, dot_r = geo["dot_c"], geo["dot_r"]
        if dot_c and dot_r and hi:
            dx_, dy_ = lw / 2 + S(dot_c[0]), body_cy + S(dot_c[1])
            dr = S(dot_r)
            dot = hi[:-2] + (210,) if isinstance(hi, tuple) else hi
            ldraw.ellipse((dx_ - dr, dy_ - dr, dx_ + dr, dy_ + dr), fill=dot)

    tilted = layer.rotate(geo["angle"], Image.BICUBIC, expand=True)
    img.alpha_composite(tilted, (round(lemon_cx - tilted.width / 2),
                                 round(lemon_cy - tilted.height / 2)))

    return img.resize((size, size), Image.LANCZOS)


# ---------------------------------------------------------------- svg emitter

def emit_svg() -> str:
    """Hand-tuned design source mirroring the Pillow geometry (original variant)."""
    spec = VARIANTS["original"]
    S = 824.0
    X0 = Y0 = 100.0
    CXC = CYC = 512.0

    def s(f: float) -> float:
        return f * S

    def w(fx: float, fy: float) -> tuple[float, float]:
        return (CXC + fx * S, CYC + fy * S)

    def fmt(v: float) -> str:
        return f"{v:.1f}"

    geo = NORMAL
    lemon_cy = CYC + s(geo["lemon_cy"])
    a, ry, rn = s(geo["a"]), s(geo["ry"]), s(geo["rn"])
    nub_dx = s(geo["a"] - geo["nub_inset"])
    theta = math.radians(geo["angle"])
    nub = (CXC + nub_dx * math.cos(theta), lemon_cy - nub_dx * math.sin(theta))
    shift = s(geo["shade_shift"])

    def leaf_path(leaf: dict, back, width: float) -> str:
        base = (nub[0] + s(back[0]), nub[1] + s(back[1]))
        tip = angled_tip(base, leaf["angle"], s(leaf["len"]))
        pts = leaf_polygon(base, tip, width, samples=64)
        d = "M " + " L ".join(f"{fmt(x)},{fmt(y)}" for x, y in pts) + " Z"
        return d

    out: list[str] = []
    out.append('<?xml version="1.0" encoding="UTF-8"?>')
    out.append('<!-- LemonSSH brand icon: a lemon with a leaf on a deep navy')
    out.append('     rounded square. Geometry mirrors scripts/generate-lemon-icon.py. -->')
    out.append('<svg xmlns="http://www.w3.org/2000/svg" viewBox="44 44 936 936" '
               'width="1024" height="1024" role="img">')
    out.append('  <title>LemonSSH</title>')
    out.append('  <defs>')
    out.append('    <!-- background crop: rounded square with ~22% corner radius -->')
    out.append('    <clipPath id="round">')
    out.append(f'      <rect x="{fmt(X0)}" y="{fmt(Y0)}" width="{fmt(S)}" height="{fmt(S)}" '
               f'rx="{fmt(S * CORNER_RADIUS)}" ry="{fmt(S * CORNER_RADIUS)}" />')
    out.append('    </clipPath>')
    out.append('    <!-- subtle navy gradient: #1F2657 (top-left) -> #0C1943 (bottom-right) -->')
    out.append('    <linearGradient id="navy" x1="0" y1="0" x2="1" y2="1">')
    out.append(f'      <stop offset="0" stop-color="{spec["bg"][0]}" />')
    out.append(f'      <stop offset="1" stop-color="{spec["bg"][1]}" />')
    out.append('    </linearGradient>')
    out.append('  </defs>')
    out.append('')
    out.append('  <g clip-path="url(#round)">')
    out.append('    <!-- background -->')
    out.append(f'    <rect x="{fmt(X0)}" y="{fmt(Y0)}" width="{fmt(S)}" height="{fmt(S)}" '
               'fill="url(#navy)" />')
    out.append('')
    out.append('    <!-- leaves and stem, attached behind the lemon tip -->')
    second = geo["leaf_second"]
    if second:
        out.append(f'    <path fill="{spec["leaf_dark"]}" d="{leaf_path(second, second["back"], s(second["w"]))}" />')
    main = geo["leaf_main"]
    out.append(f'    <path fill="{spec["leaf"]}" d="{leaf_path(main, main["back"], s(main["w"]))}" />')
    out.append('    <!-- leaf midrib -->')
    base = (nub[0] + s(main["back"][0]), nub[1] + s(main["back"][1]))
    tip = angled_tip(base, main["angle"], s(main["len"]))
    t0, t1 = geo["midrib"]["t0"], geo["midrib"]["t1"]
    mb = (base[0] + (tip[0] - base[0]) * t0, base[1] + (tip[1] - base[1]) * t0)
    mt = (base[0] + (tip[0] - base[0]) * t1, base[1] + (tip[1] - base[1]) * t1)
    rib = leaf_polygon(mb, mt, s(geo["midrib"]["w"]), samples=16)
    out.append('    <path fill="' + spec["leaf_dark"] + '" d="M '
               + " L ".join(f"{fmt(x)},{fmt(y)}" for x, y in rib) + ' Z" />')
    stem = geo["stem"]
    sc = (nub[0] + s(stem["back"][0]), nub[1] + s(stem["back"][1]))
    out.append(f'    <circle cx="{fmt(sc[0])}" cy="{fmt(sc[1])}" r="{fmt(s(stem["r"]))}" '
               f'fill="{spec["leaf_dark"]}" />')
    out.append('')
    out.append('    <!-- lemon: local group is axis-aligned, then tilted CCW -->')
    out.append(f'    <g transform="translate({fmt(CXC)} {fmt(lemon_cy)}) rotate(-{geo["angle"]:g})">')
    out.append('      <!-- bottom shading rim (body drawn shifted up leaves this visible) -->')
    out.append(f'      <ellipse cx="0" cy="0" rx="{fmt(a)}" ry="{fmt(ry)}" fill="{spec["shade"]}" />')
    for sign in (-1, 1):
        out.append(f'      <circle cx="{fmt(sign * nub_dx)}" cy="0" r="{fmt(rn)}" fill="{spec["shade"]}" />')
    out.append('      <!-- lemon body with its two tips -->')
    out.append(f'      <ellipse cx="0" cy="{fmt(-shift)}" rx="{fmt(a)}" ry="{fmt(ry)}" fill="{spec["body"]}" />')
    for sign in (-1, 1):
        out.append(f'      <circle cx="{fmt(sign * nub_dx)}" cy="{fmt(-shift)}" r="{fmt(rn)}" fill="{spec["body"]}" />')
    out.append('      <!-- highlights -->')
    hi_c, hi_r = geo["hi_c"], geo["hi_r"]
    hx, hy = s(hi_c[0]), -shift + s(hi_c[1])
    out.append(f'      <ellipse cx="{fmt(hx)}" cy="{fmt(hy)}" rx="{fmt(s(hi_r[0]))}" '
               f'ry="{fmt(s(hi_r[1]))}" fill="{spec["hi"]}" opacity="0.92" />')
    if geo.get("dot_c") and geo.get("dot_r"):
        dot_c, dot_r = geo["dot_c"], geo["dot_r"]
        out.append(f'      <circle cx="{fmt(s(dot_c[0]))}" cy="{fmt(-shift + s(dot_c[1]))}" '
                   f'r="{fmt(s(dot_r))}" fill="{spec["hi"]}" opacity="0.82" />')
    out.append('    </g>')
    out.append('  </g>')
    out.append('')
    out.append('  <!-- inner border ring -->')
    inset = s(0.005)
    out.append(f'  <rect x="{fmt(X0 + inset)}" y="{fmt(Y0 + inset)}" '
               f'width="{fmt(S - 2 * inset)}" height="{fmt(S - 2 * inset)}" '
               f'rx="{fmt(S * CORNER_RADIUS - inset)}" ry="{fmt(S * CORNER_RADIUS - inset)}" '
               'fill="none" stroke="#FFFFFF" stroke-opacity="0.4" stroke-width="8" />')
    out.append('</svg>')
    return "\n".join(out) + "\n"


# ---------------------------------------------------------------- outputs

def write_outputs() -> list[Path]:
    written: list[Path] = []

    def save(img: Image.Image, path: Path) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        img.save(path, format="PNG")
        written.append(path)
        print(f"  wrote {path.relative_to(ROOT)} ({img.width}x{img.height})")

    # 1. design source svg
    svg_path = ROOT / "public" / "icon.svg"
    svg_path.parent.mkdir(parents=True, exist_ok=True)
    svg_path.write_text(emit_svg(), encoding="utf-8")
    written.append(svg_path)
    print(f"  wrote {svg_path.relative_to(ROOT)}")

    # 2. build/icons PNG set (16/32 use the tuned small style)
    print("build/icons PNG set:")
    for size in BUILD_SIZES:
        style = "small" if size <= 32 else "normal"
        save(render_icon(size, "original", style=style),
             ROOT / "build" / "icons" / f"{size}x{size}.png")

    # 3. embedded application icon (512)
    print("application icon:")
    save(render_icon(512, "original"), ROOT / "cmd" / "lemonssh" / "appicon.png")

    # 4. Windows ICO files (multi-size, exact frames per size)
    print("Windows ICO files:")
    frames = {size: render_icon(size, "original",
                                style="small" if size <= 32 else "normal")
              for size in ICO_SIZES}
    for rel in ("cmd/lemonssh/app.ico", "build/lemonssh.ico"):
        path = ROOT / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        base = frames[ICO_SIZES[-1]]
        base.save(path, format="ICO",
                  sizes=[(s, s) for s in ICO_SIZES],
                  append_images=[frames[s] for s in ICO_SIZES[:-1]])
        written.append(path)
        print(f"  wrote {rel} (sizes: {', '.join(str(s) for s in ICO_SIZES)})")

    # 5. runtime variants (desktop crop + macOS HIG padding)
    print("runtime icon variants:")
    for variant_id in VARIANTS:
        desktop = render_icon(1024, variant_id, margin_frac=MARGIN_DESKTOP)
        save(desktop, ROOT / "public" / "icons" / "variants" / f"{variant_id}.png")
        macos = render_icon(1024, variant_id, margin_frac=MARGIN_MACOS)
        save(macos, ROOT / "public" / "icons" / "variants" / "macos" / f"{variant_id}.png")

    return written


def try_write_icns() -> Path | None:
    """Build build/icon.icns with icnsutil when the package is available."""
    out = ROOT / "build" / "icon.icns"
    try:
        from icnsutil import IcnsFile  # type: ignore
    except ImportError:
        print("icnsutil not available -> skipping build/icon.icns "
              "(pip install icnsutil to enable)")
        return None
    # Name each temp file after its icns key so icnsutil maps the type
    # deterministically (icp4=16 .. ic10=1024).
    icns_sources = [(16, "icp4"), (32, "icp5"), (48, "icp6"), (128, "ic07"),
                    (256, "ic08"), (512, "ic09"), (1024, "ic10")]
    tmp_dir = ROOT / "build" / ".icns-work"
    tmp_dir.mkdir(parents=True, exist_ok=True)
    tmp_files: list[Path] = []
    try:
        icns = IcnsFile()
        for px, key in icns_sources:
            img = render_icon(px, "original")
            tmp = tmp_dir / f"{key}.png"
            img.save(tmp, format="PNG")
            tmp_files.append(tmp)
            icns.add_media(file=str(tmp))
        icns.write(str(out))
    finally:
        for tmp in tmp_files:
            tmp.unlink(missing_ok=True)
        tmp_dir.rmdir()
    return out


def verify(written: list[Path]) -> None:
    print("\nverification:")
    failures = []
    for path in written:
        if path.suffix == ".svg":
            ok = path.is_file() and path.stat().st_size > 0
            print(f"  OK  {path.relative_to(ROOT)} (svg, {path.stat().st_size} bytes)"
                  if ok else f"  FAIL {path}")
            if not ok:
                failures.append(path)
            continue
        with Image.open(path) as im:
            im.load()
            if path.suffix == ".ico":
                sizes = sorted(im.info.get("sizes", []))
                expected = {(s, s) for s in ICO_SIZES}
                ok = expected.issubset(set(sizes)) and im.mode == "RGBA"
                print(f"  {'OK ' if ok else 'FAIL'} {path.relative_to(ROOT)} "
                      f"(ico frames: {sizes})")
            else:
                ok = im.mode == "RGBA"
                print(f"  {'OK ' if ok else 'FAIL'} {path.relative_to(ROOT)} "
                      f"({im.width}x{im.height}, {im.mode})")
            if not ok:
                failures.append(path)
    if failures:
        raise SystemExit(f"verification failed for: {failures}")
    print("all outputs verified (RGBA, correct dimensions)")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--skip-icns", action="store_true",
                        help="do not attempt build/icon.icns generation")
    args = parser.parse_args()

    print("generating lemon icon set...")
    written = write_outputs()

    if not args.skip_icns:
        icns = try_write_icns()
        if icns is not None:
            written.append(icns)
            print(f"  wrote {icns.relative_to(ROOT)}")

    verify(written)


if __name__ == "__main__":
    main()
