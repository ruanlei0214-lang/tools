"""把 exe 里的图标资源全部导出，用来确认构建产物到底带了哪个图标。

不走 Shell API（ExtractAssociatedIcon 之类），直接读 PE 的资源目录，
结果不受 Windows 图标缓存影响。

用法：python tools/appicon/dump_exe_icons.py [exe路径] [输出目录]
"""

import struct
import sys
from pathlib import Path

import pefile

RT_ICON = 3
RT_GROUP_ICON = 14


def entries(pe, type_id):
    for entry in pe.DIRECTORY_ENTRY_RESOURCE.entries:
        if entry.id != type_id:
            continue
        for named in entry.directory.entries:
            for lang in named.directory.entries:
                data = pe.get_data(
                    lang.data.struct.OffsetToData, lang.data.struct.Size
                )
                yield named.id, data


def main(exe: Path, out: Path) -> None:
    pe = pefile.PE(str(exe))
    icons = dict(entries(pe, RT_ICON))
    groups = dict(entries(pe, RT_GROUP_ICON))

    print(f"{exe.name}: {len(groups)} 个图标组，{len(icons)} 张图")
    if not groups:
        print("没有图标资源——exe 会显示系统默认图标")
        return

    out.mkdir(parents=True, exist_ok=True)
    for gid in sorted(groups):
        raw = groups[gid]
        count = struct.unpack_from("<H", raw, 4)[0]
        print(f"\n图标组 {gid}（Explorer 用编号最小的那个组）：{count} 个尺寸")
        for i in range(count):
            w, h, _, _, _, _, _, icon_id = struct.unpack_from("<BBBBHHIH", raw, 6 + i * 14)
            w, h = w or 256, h or 256
            blob = icons.get(icon_id)
            if blob is None:
                print(f"  {w}x{h} -> 资源 {icon_id} 缺失")
                continue
            # 单张 RT_ICON 是「没有文件头的 ICO」，补一个头就能当 .ico 打开。
            header = struct.pack(
                "<HHHBBBBHHII", 0, 1, 1, w % 256, h % 256, 0, 0, 1, 32, len(blob), 22
            )
            path = out / f"group{gid}_{w}x{h}.ico"
            path.write_bytes(header + blob)
            print(f"  {w}x{h} -> {path.name}")


def default_exe() -> Path:
    """构建产物的文件名带版本号，会随发版变，所以在 build/bin 里找而不是写死。"""
    found = sorted(Path("build/bin").glob("*.exe"))
    if not found:
        sys.exit("build/bin 下没有 exe，先构建，或把路径作为第一个参数传进来")
    if len(found) > 1:
        sys.exit(f"build/bin 下有多个 exe，请指定一个：{', '.join(p.name for p in found)}")
    return found[0]


if __name__ == "__main__":
    exe = Path(sys.argv[1]) if len(sys.argv) > 1 else default_exe()
    out = Path(sys.argv[2]) if len(sys.argv) > 2 else Path("build/_icons_dump")
    main(exe, out)
