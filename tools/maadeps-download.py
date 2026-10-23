#!/usr/bin/env python3
import os
import sys
from pathlib import Path

sub_path = Path(__file__).parent.parent / "agent" / "cpp-algo" / "MaaUtils" / "tools"
sys.path.append(str(sub_path))

# sys.path 上面才补上子模块路径，这两行 import 只能放在这里。
from maadeps_download import detect_host_triplet  # noqa: E402
from maadeps_download import main as download_main  # noqa: E402

REPO = "MaaXYZ/MaaDeps"

# 版本必须与链接期那份 MaaFramework 预编译产物同源，不能只追 MaaDeps 的最新版
#
# 桌面端看不出问题：MaaDeps 在 Windows 上把那份库改名成 `onnxruntime_maa.dll`，
# 与 MaaFramework 自带的 `onnxruntime.dll` 能在同一进程里并存。Android 没有这层改名，
# SONAME 都是 `libonnxruntime.so`，而 agent 与 app 共用同一个 native lib 目录，
# 那个目录里只能有一份；bionic 又严格校验 ELF 符号版本，于是引用 VERS_1.29.0 的
# `libcpp-algo.so` 撞上只导出 VERS_1.19.2 的那份，直接
# `CANNOT LINK EXECUTABLE …: cannot locate symbol "OrtGetApiBase"`
#
# MaaFramework v5.14.0 用的是 MaaDeps v2.12.6——那份 runtime 包里的
# `libfastdeploy_ppocr.so` 与 `deps-android/arm64-v8a/bin/` 下同名文件 md5 完全一致，
# 可以拿这条当校验。v3.0.0 起 ORT 才升到 1.29
MAADEPS_VERSION_BY_MAAFW = {
    "v5.14.0": "v2.12.6",
    "v5.14.1": "v2.12.6",
    "v5.14.2": "v2.12.6",
    "v5.14.3": "v2.12.6",
}
DEFAULT_MAAFW_TAG = "v5.14.0"

maafw_tag = os.environ.get("MAAFW_VERSION", DEFAULT_MAAFW_TAG)
VERSION = os.environ.get("MAADEPS_VERSION") or MAADEPS_VERSION_BY_MAAFW.get(maafw_tag)
if not VERSION:
    raise SystemExit(
        f"没有 {maafw_tag} 对应的 MaaDeps 版本：给 MAADEPS_VERSION_BY_MAAFW 补一条，"
        "或用 MAADEPS_VERSION 显式指定"
    )

if __name__ == "__main__":
    target_triplet = sys.argv[1] if len(sys.argv) == 2 else detect_host_triplet()

    download_main(target_triplet, REPO, VERSION)
