# Android 构建环境准备

> **这页写给谁？**
>
> 你第一次要编 Android 客户端，或者 `uv run tools/build_android_agents.py --check-env` 报了一堆 MISS。
>
> 这页只讲**装依赖、配环境变量**。装完之后怎么构建、怎么出 APK，看 [Android/README.md](../../../Android/README.md)。

---

## 先看结论

第一次配环境，按顺序做三件事：

1. 装 6 样东西 → [第 1 节](#1-装这-6-样)
2. 配好 Android SDK 路径 → [第 2 节](#2-配-android-sdk-路径)
3. 跑一次体检 → [第 3 节](#3-跑一次体检)

**照着命令抄就不会出错。** 万一报错，去 [第 4 节](#4-装依赖时的报错) 对号入座。

配完**务必重开终端**，已经开着的窗口读不到新变量。

---

## 1. 装这 6 样

| 要装的东西 | 干什么用的 | 怎么装 |
| ---------------- | -------------------------------- | ----------------------------------------------------------------------------------- |
| JDK 17 及以上 | 编译 App，Gradle 要用 | Windows `winget install Microsoft.OpenJDK.21`；macOS `brew install openjdk@21`，再把 `$(brew --prefix openjdk@21)/bin` 加到 `PATH` 最前面（brew 不会自动加） |
| Android SDK | 提供各种 Android 打包工具 | [1.1](#11-装-android-sdk) |
| Android NDK | 交叉编译 C++ agent（cpp-algo），go-service 的 cgo 也用它的 clang | `sdkmanager --install "ndk;29.0.13599879"` |
| CMake ≥ 3.28 | 配置并构建 cpp-algo | [1.2](#12-选一份-cmake) |
| Go 1.25.6+ | 交叉编译 go-service | [超基础入门](./super-basic-introduction.md) 2.6 |
| uv | 跑仓库里的 Python 脚本 | [快速开始](./getting-started.md) |

> [!NOTE]
>
> **这些到底是什么？** 一句话版：JDK 是跑 Gradle 的 Java 环境；Android SDK 是官方的打包工具集合；NDK 是"给 Android 编 C++ 的编译器"；CMake 是 C++ 项目的构建配置工具。装齐就行，不用深入。
>
> **Android Studio 不需要装**，命令行工具就够（想装也行）。

### 1.1 装 Android SDK

1. 去 [Android 命令行工具下载页](https://developer.android.com/studio#command-line-tools-only) 下载对应系统的 zip。zip 里是一个 `cmdline-tools` 文件夹，解压后把它改名为 `latest`，放到 `<SDK>/cmdline-tools/` 下（`<SDK>` 是你选的 SDK 目录），让 `sdkmanager` 位于 `<SDK>/cmdline-tools/latest/bin`。
2. 把 `<SDK>/cmdline-tools/latest/bin` 和 `<SDK>/platform-tools` 加进 `PATH`。
3. 装组件：

```bash
sdkmanager --licenses
sdkmanager --install "platform-tools" "platforms;android-37.0" "build-tools;37.0.0"
sdkmanager --install "ndk;29.0.13599879"
```

> [!NOTE]
>
> **版本号里的那个点不能省：是 `platforms;android-37.0`。** API 36 以前是整数版本（`android-35`、`android-36`），从 36.1 起改成"主版本.次版本"（`android-36.1`、`android-37.0`、`android-37.1`），所以按"下一个整数"猜会猜错。写成 `android-37` 报的是 `Failed to find package`，别误判成网络问题。
>
> 版本号来自 `Android/MaaFwApp/build-logic/` 里的 `COMPILE_SDK`（当前 37）。

> [!NOTE]
>
> **NDK 版本跟 CI 保持一致**：上面写的 `29.0.13599879` 就是 CI 用的那个（见 `.github/workflows/android.yml` 的 `AGENT_NDK_VERSION`）。版本不同一般也能编过，但本地出问题时，先换回 CI 的版本排除干扰。

### 1.2 选一份 CMake

系统里会存在**两个** CMake，别搞混：

- **外壳自己的**：MaaFwApp 在 `build.gradle.kts` 里写死了 SDK 的 `cmake;3.22.1`。**这个你不用管**，Gradle 首次构建时自动下载。
- **agent 交叉编译用的**：`tools/build_android_agents.py` 要求 **CMake ≥ 3.28**。**这个要你装**。

装法二选一，**推荐第一种**：

| 装法 | 命令 | 说明 |
| ------------------ | ------------------------------------------- | ---------------------------------------------------- |
| **用 SDK 的**（推荐） | `sdkmanager --install "cmake;3.31.6"` | 它 ≥ 3.28 够用，**而且包里自带 `ninja`，不用再单独装** |
| 装系统 CMake | Windows `winget install Kitware.CMake`；macOS `brew install cmake` | 也行，但 Ninja 要自己保证在 `PATH` 上 |

脚本会先翻 `<SDK>/cmake/*`，再找 `PATH`，所以上面两种都认。

### 1.3 检查点

不确定齐没齐？**先跑体检**，它会逐项列出缺什么、怎么修：

```bash
uv run tools/build_android_agents.py --check-env
```

看到 `OK  environment ready` 就可以去 [Android/README.md](../../../Android/README.md) 构建了。

> [!TIP]
>
> 体检过了就**不用**再手动敲 `cmake --version` / `ninja --version` 复核。如果你走的是 [1.2](#12-选一份-cmake) 里"用 SDK 的 CMake"那条路，这两个命令本来就可能不在 `PATH` 里，手动敲反而会误判成没装。

---

## 2. 配 Android SDK 路径

**只有 SDK 路径是必须的**，另两个设了更稳：

| 环境变量 | 必需？ | 指向 | 说明 |
| ---------------------------------------- | ---------------- | --------------------------------------------- | -------------------------------------------------- |
| `ANDROID_HOME` 或 `ANDROID_SDK_ROOT` | **必需**（二选一） | Android SDK 根目录 | 脚本靠它找 NDK / CMake / Ninja |
| `ANDROID_NDK_ROOT` | 推荐 | NDK 根目录（例如 `.../ndk/29.0.13599879`） | 没设时脚本自己挑 `<SDK>/ndk` 里最新的 |
| `JAVA_HOME` | 推荐 | JDK 根目录 | 没设时用 `PATH` 上的 `java`；装了多个 JDK 时建议设 |

> [!WARNING]
>
> **光装好还不够，SDK 路径必须配上。** 不配的话体检的 Android SDK 一项就会报 `ANDROID_HOME / ANDROID_SDK_ROOT is not set`。

---

## 3. 跑一次体检

```bash
uv run tools/build_android_agents.py --check-env
```

它只报告、不安装，也不会去扫描磁盘找依赖；Go 只检查 `PATH` 上有没有，不核对版本。全绿就可以开始构建；有 MISS 就照着提示逐条修，每条都写了现状和修法。

<details>
<summary>体检过了应该长什么样</summary>

```text
Android build environment check
  Windows AMD64, python 3.14.2

OK  environment ready: JDK 17+, Android SDK, Android NDK, CMake >= 3.28, Ninja, Go, MaaUtils submodule
run: uv run tools/build_android_agents.py
```

第二行是你自己的系统和 Python 版本（macOS 上会是 `Darwin arm64`），**不用和上面完全一致**。

判断标准只有一条：出现 `OK  environment ready`。这一行里列的项目对所有人都一样。

</details>

---

## 4. 装依赖时的报错

### `Warning: Failed to find package 'platforms;android-37'`

包名少了小数位。API 36 以后版本号是 `主版本.次版本`，写 `platforms;android-37.0`，见 [1.1](#11-装-android-sdk)。

> [!NOTE]
>
> 它被标成 `Warning:` 而不是 `Error:`，很容易一眼扫过去，但退出码是 **1**，而且只要命令里有一个包名写错，**整条命令都算失败**（同一条命令里的其它包也一个都不会装）。

### `Java version 17 or higher is required`

`sdkmanager` 找到的 Java 低于 17（`JAVA_HOME` 或 `PATH` 上的 `java` 太旧）。装一个 JDK 17+ 并设置 `JAVA_HOME`，然后重开一个终端再试。

### `CMake was unable to find a build program corresponding to "Ninja Multi-Config"`

配置阶段找不到 Ninja。按 [1.2](#12-选一份-cmake) 装 SDK 的那份 CMake（自带 `ninja`）即可，脚本会自动找到它。

> [!NOTE]
>
> 体检说 Ninja OK、你手敲 `ninja --version` 却没输出，是正常的——脚本找的是 `<SDK>/cmake/*/bin` 里那份，不在 `PATH` 上，以体检结果为准。真要自己加就**追加到末尾**（放前面会让那份 CMake 3.22.1 遮蔽新装的版本）。

### `CMake >= 3.28 required, but the newest one found is 3.22.1`

系统里只有旧版 CMake，装法见 [1.2](#12-选一份-cmake)。

### `CANNOT LINK EXECUTABLE "…libcpp-algo.so": cannot locate symbol "OrtGetApiBase"`

报的是**符号**找不到，不是 `library not found`——加载器已经摸到 `libonnxruntime.so` 了，只是那份导出的符号版本对不上。

Android 上 agent 与 app 共用同一个 native lib 目录（`/data/app/…/lib/arm64`；PI 解包目录落在 noexec 的外部存储上，exec 不了，所以 agent 只能从前者起），`libonnxruntime.so` 这个 SONAME 那儿只能有一份，而 bionic 严格校验 ELF 符号版本：`libcpp-algo.so` 里记的 `OrtGetApiBase@VERS_x.y.z` 必须与那份库导出的 `@@VERS_x.y.z` 完全一致。

> [!NOTE]
>
> 这条只炸 Android。MaaDeps 在 Windows 上把库改名成 `onnxruntime_maa.dll`，与 MaaFramework 自带的 `onnxruntime.dll` 能在同一进程里并存，版本对不上也不会当场报链接错误，所以桌面端看不出问题。

真机上的症状顺序：`MaaRunner: agent[0] connected` → `ExecAgentHost: launching …libcpp-algo.so` → linker 这句 fatal → `agent exited unexpectedly … exitCode=1` → 三十秒后 `prepare failed: agent[1]: agent 连接超时：agent/cpp-algo`，整个任务不起。

原因几乎都是 **MaaDeps 与 MaaFramework 不同源**：MaaDeps 会追着 ONNX Runtime 升（v3.0.0 起 ORT 到 1.29），而下载来的那份 MaaFramework 预编译产物还是旧 ORT。不用下载就能判断同不同源——比 md5：MaaDeps 的 `MaaDeps-<triplet>-runtime` 包里的 `libfastdeploy_ppocr.so`，与 `deps-android/<abi>/bin/` 下同名文件一致即同源。

修法是在 `tools/maadeps-download.py` 的 `MAADEPS_VERSION_BY_MAAFW` 里给所用 MaaFramework tag 补一条对应版本（v5.14.x ↔ v2.12.6：那份 runtime 里 `libonnxruntime.so` 是 27,905,128 字节、导出 `VERS_1.19.2`），删掉 `agent/cpp-algo/MaaUtils/MaaDeps/vcpkg/installed/maa-arm64-android` 重下重编。

> [!WARNING]
>
> 换完版本旧 object 不会自动重编：tar 解出来的头 mtime 是打包那天，比上次编出来的 `.o` 还早，Ninja 会判定依赖没变。要么删掉 `agent/cpp-algo/build/android/<abi>`，要么把新 `include/` 整棵 `touch` 一遍。编完先验再出包：
>
> ```
> llvm-readelf --dyn-syms Android/agent-dist/<abi>/jniLibs/libcpp-algo.so | grep OrtGetApiBase
> ```

---

## 5. Windows ARM64 上的额外坑

> [!NOTE]
>
> 这一段只在 ARM64 Windows（骁龙平板、部分轻薄本）上会遇到，x64 机器可以直接跳过。全部是 2026-10-23 在一台 Snapdragon 8cx Gen 2 平板上实测踩出来的。

### 解压 zip 别用 `Expand-Archive`

Windows PowerShell 5.1 的 `Expand-Archive` 解 JDK / Go / Node 这种几百 MB 的包会长时间零输出（实测烧了 433 秒 CPU 还没落第一个文件）。换系统自带的 bsdtar，同一份包 2 秒解完：

```powershell
& "$env:SystemRoot\System32\tar.exe" -xf $zip -C $dest
```

**一定要写绝对路径**：Git Bash 的 `/usr/bin/tar` 是 GNU tar，既排在 `PATH` 前面，又把 `C:` 当成远程主机名，报 `/usr/bin/tar: Cannot connect to C: resolve failed`。

### `cmdline-tools` 解压后要自己补一层 `latest`

`commandlinetools-win-13114758_latest.zip` 解出来直接是 `cmdline-tools\bin|lib`，没有 `latest` 这一层，于是 `sdkmanager.bat` 找不到。`sdkmanager` 只认 `<SDK>\cmdline-tools\latest\bin\sdkmanager.bat`，所以把解压出来的 `cmdline-tools` 再往里挪一层。

### `sdkmanager --licenses` 要一次喂够 `y`

它每个协议问一次，管道里只给一个 `y` 会停在 `8 licenses not accepted`，而且**退出码仍然是 0**；紧接着的 `--install` 于是把每个包都跳过，看起来像"装完了"。一次喂满：

```powershell
(1..40 | ForEach-Object { 'y' }) | & $sdkmanager --sdk_root=$sdk --licenses
```

### GitHub 的 release 下载老是断

`api.github.com` 一直通，但 `github.com/.../releases/download/...`（实际落在 objects.githubusercontent.com）频繁 `Could not connect` / `Connection was reset` / `handshake operation timed out`。`build_android_agents.py` 把包缓存在 `.cache/android-agents/<包名>.zip`，**文件在就跳过下载**，所以用 `curl -C -` 把它灌满比重跑整条构建划算：

```bash
curl.exe -sSL --fail -C - --retry 3 \
  "https://github.com/MaaXYZ/MaaFramework/releases/download/<tag>/MAA-android-aarch64-<tag>.zip" \
  -o .cache/android-agents/MAA-android-aarch64-<tag>.zip
```

> [!NOTE]
>
> 脚本中断时留在缓存里的是**从头写的半截文件**（`download()` 直接 `open(dest, "wb")` 流式写），所以 `-C -` 续传是安全的。半截文件若是别处来的，先删干净再下。

### Go 模块代理走不通

`proxy.golang.org` 在这个网络里是 IPv6 直连超时（`connectex: The connected party did not properly respond`），交叉编译 go-service 时 `go: downloading ...` 会直接失败。指到可达的镜像即可：

```powershell
$env:GOPROXY = 'https://goproxy.cn,direct'
```

> [!NOTE]
>
> 换镜像不影响完整性：仓库带 `go.sum`，镜像给出的包哈希对不上就会报错。

### 写 `.ps1` 就只用 ASCII

5.1 读没有 BOM 的 UTF-8 脚本会按本地代码页解析，中文注释直接变成语法错误（`意外的标记')'`）。要么存成带 BOM 的 UTF-8，要么注释写英文。

### `uv run` 十几分钟没输出

`uv run tools/build_android_agents.py` 有可能卡在解析环境 / 拉包上。这个脚本只用标准库，直接调 uv 自带的那个 python 就行：

```powershell
& "$env:USERPROFILE\AppData\Roaming\uv\python\cpython-3.<版本>-windows-aarch64-none\python.exe" tools\build_android_agents.py
```

---

## 相关

- [Android/README.md](../../../Android/README.md) —— 装完之后怎么构建、出包、发版
- [快速开始](./getting-started.md) —— 桌面端环境准备与 Pipeline 开发
- [超基础入门](./super-basic-introduction.md) —— Git、终端、Node、Go 从零装起
