"""生成 build/appicon.png：白色圆角方块上一个黑色字母。

沿用 Wails 默认图标的构图（透明底、白色圆角方块、居中的黑色无衬线粗体字母），
只换字母。用脚本而不是找一张图，是为了字母换了以后还能一比一复现同样的构图。

用法：python tools/appicon/gen.py [字母] [输出路径]
"""

import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont

SIZE = 1024
# 以下比例是从 Wails 默认图标上量出来的：方块占画布 795/1024，字母墨迹高度占方块 0.685。
MARGIN = 112
RADIUS = 88
GLYPH_RATIO = 0.685
SQUARE = (255, 255, 255, 255)
GLYPH = (32, 32, 32, 255)

# Windows 自带字体，从粗到细挑第一个存在的。Arial Black 最接近默认图标那种厚重的字重。
FONT_CANDIDATES = [
    "C:/Windows/Fonts/ariblk.ttf",  # Arial Black
    "C:/Windows/Fonts/arialbd.ttf",  # Arial Bold
    "C:/Windows/Fonts/segoeuib.ttf",  # Segoe UI Bold
]


def pick_font(size: int) -> ImageFont.FreeTypeFont:
    for path in FONT_CANDIDATES:
        if Path(path).exists():
            return ImageFont.truetype(path, size)
    raise SystemExit(f"找不到可用字体，试过：{FONT_CANDIDATES}")


def fit_font(draw: ImageDraw.ImageDraw, letter: str, target_h: float):
    """按墨迹实际高度反推字号。

    直接拿字号当尺寸会让不同字母大小不一：字号描述的是 em 框，而 C 和 W 在 em 框里
    占的比例差很多。所以先在参考字号下量一次墨迹高度，再按比例缩放到目标高度。
    """
    probe = 600
    font = pick_font(probe)
    _, top, _, bottom = draw.textbbox((0, 0), letter, font=font)
    ink = bottom - top
    if ink <= 0:
        raise SystemExit(f"字母 {letter!r} 量不到墨迹范围")
    return pick_font(max(1, round(probe * target_h / ink)))


def render(letter: str, out: Path) -> None:
    inner = SIZE - 2 * MARGIN
    box = [MARGIN, MARGIN, SIZE - MARGIN, SIZE - MARGIN]

    # 先画一层模糊的深色方块当投影，白色方块才能从浅色桌面背景里分离出来，
    # 否则透明底上的纯白方块看起来就是一整块白。偏移量对齐默认图标的光源方向。
    img = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
    shadow = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
    ImageDraw.Draw(shadow).rounded_rectangle(
        [box[0], box[1] + 10, box[2], box[3] + 10], radius=RADIUS, fill=(0, 0, 0, 60)
    )
    img.alpha_composite(shadow.filter(ImageFilter.GaussianBlur(12)))

    draw = ImageDraw.Draw(img)
    draw.rounded_rectangle(box, radius=RADIUS, fill=SQUARE)

    # 用实际墨迹范围做居中：靠 anchor 居中会因为基线和行高的留白而整体偏上。
    font = fit_font(draw, letter, inner * GLYPH_RATIO)
    left, top, right, bottom = draw.textbbox((0, 0), letter, font=font)
    x = (SIZE - (right - left)) / 2 - left
    y = (SIZE - (bottom - top)) / 2 - top
    draw.text((x, y), letter, font=font, fill=GLYPH)

    out.parent.mkdir(parents=True, exist_ok=True)
    img.save(out, "PNG")
    print(f"已生成 {out}（字母 {letter}，{SIZE}x{SIZE}，墨迹高 {bottom - top}px）")

    # exe 上真正显示的是 build/windows/icon.ico。wails 在这个文件缺失时会自己从
    # appicon.png 转，但转出来的尺寸档位不受控；这里直接写全，任务栏和资源管理器
    # 各档缩略图都清晰。
    ico = out.parent / "windows" / "icon.ico"
    if ico.parent.exists():
        img.save(ico, "ICO", sizes=[(n, n) for n in (16, 24, 32, 48, 64, 128, 256)])
        print(f"已生成 {ico}")


if __name__ == "__main__":
    letter = sys.argv[1] if len(sys.argv) > 1 else "C"
    out = Path(sys.argv[2]) if len(sys.argv) > 2 else Path("build/appicon.png")
    render(letter, out)
