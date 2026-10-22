# 云浏览器（CloudBrowser）控制器

在本地 Chrome / Edge 里打开网页版「云·终末地」（<https://endfield.hypergryph.com/cloud>），
用 Win32 控制器接管这个浏览器窗口。用户不需要下载安卓端云·终末地客户端，也不需要 APK。

网页云实例里跑的就是**国服 Windows 客户端**（实测为 `CN_WIN_REL_1.6.3`，X86 实例），
所以游戏内的全部 pipeline 直接复用桌面端 `resource`，本控制器只额外负责「网页门户 → 进入游戏」这一段。

## 资源分层

| 目录 | 作用 |
| --- | --- |
| `assets/resource` | 桌面端基线，游戏内节点全部来自这里 |
| `assets/resource_cloud_browser` | 只有门户层节点，以及 `OpenGame.next` 的同名覆盖 |

`interface.json` 中 `CloudBrowser` 只挂 `./resource_cloud_browser`，不像 `CloudADB` 那样再挂
`./resource_adb`——因为网页云里不是安卓客户端。

ProjectInterface v2 的 `controller[].type` 没有浏览器这一类，所以 `CloudBrowser` 借用 `Win32`。
**任何需要把它和真实桌面客户端区分开的代码，必须比较控制器名而不是类型**，
常量在 `pkg/pienv`：`pienv.ControllerNameCloudBrowser`。

## Android 内嵌浏览器（MaaFwApp）

MaaFwApp 另有两条不依赖外置浏览器的控制器，直接在应用内的虚拟屏 WebView 里打开门户：

| 控制器 | 云平台 | 附加资源 |
| --- | --- | --- |
| `CloudBrowserPC` | 电脑版（云端跑 Windows 客户端） | `./resource_cloud_browser` |
| `CloudBrowserMobile` | 手机版（云端跑安卓客户端） | `./resource_adb`、`./resource_cloud_browser`、`./resource_cloud_browser_mobile` |

门户按「开始游戏」那次点击的 `pointerType` 选择云平台：鼠标 → 电脑版，触摸 → 手机版。
pc 模式由 `InputControlUtils` 注入原生 `SOURCE_MOUSE` 事件（按下用 `WAIT_FOR_FINISH`，
未按下移动发 `ACTION_HOVER_MOVE`，按住移动发 `ACTION_MOVE`），mobile 模式保持触摸注入。
内嵌 Activity 把自己的密度固定为 160 dpi，使 1280×720 虚拟屏恰好对应 1280×720 CSS 像素，
桌面版门户的 ROI 可以直接复用；手机版门户布局不同，`resource_cloud_browser_mobile` 把
「开始游戏」与登录框改回整帧 OCR。

PI 协议不接受未知 `controller[].type`（MaaPiCli 会拒绝整个 interface），所以这两条仍声明为
`Win32`，另带 `embedded_browser: { "mode": "pc" | "mobile" }` 标记。桌面端忽略该字段，照旧按
`win32` 配置抓外置 Chrome 窗口；MaaFwApp 看到标记就改走内嵌 WebView。`embedded_browser`
因此也是 MaaFwApp 判定「这条控制器能不能用」的依据，不要再按 type 判断。

虚拟屏的 IME 是隐藏的，首次登录与短信验证码无法在虚拟屏里输入。MaaFwApp 设置页提供
「登录云终末地」，在主屏用普通 WebView 打开同一门户；Cookie 与 localStorage 全应用共享，
登录一次后虚拟屏里的门户即为已登录状态。

内嵌浏览器复用同一套门户节点与 `OpenGame.next` 覆盖，游戏内资源按云平台选择：
电脑版用桌面 `resource`，手机版叠加 `resource_adb`。`CloudBrowser`（外置）仍只面向桌面端，
其 pretask 不会在 MaaFwApp 中执行。

## 三条硬约束

这三条是踩出来的，不满足就整条链路失效，改动 `agent/go-service/pretask/cloudbrowser/` 前请先读完。

1. **客户区必须正好 1280×720。** 仓库所有 ROI 与模板都以 720p 为基准；ADB 有 MaaFramework 自动缩放，
   桌面控制器截到多大就是多大。`--window-size` 设的是**外框**，所以 pretask 先量一次
   「外框 − 客户区」差值，再反推外框尺寸 `SetWindowPos`。
2. **DPI 必须 1:1。** Go 程序没有 manifest 时是 DPI-unaware，`GetWindowRect` 拿到的是 DWM 虚拟化坐标；
   系统缩放 125% 时页面会渲染成 1600×900 物理像素。因此进程启动即
   `SetProcessDpiAwarenessContext(PER_MONITOR_AWARE_V2)`，并给浏览器传 `--force-device-scale-factor=1`。
3. **只能有一个实例。** 云实例是单会话的，第二个客户端会把第一个挤回标题页（已实测）。
   pretask 按 `--user-data-dir=<绝对路径>` 匹配进程命令行复用已有实例，绝不启第二个。
   登录态就存在这个 profile 里（`os.UserConfigDir()/MaaEnd/cloud_browser`），不要挪进安装目录或版本库。

另外 pretask 无条件给浏览器传 `--proxy-bypass-list`，放行 `vlinkcloud.cn` / `hypergryph.com` /
`hycdn.cn` / `skland.com`。WebRTC 媒体走 UDP，被系统 HTTP 代理劫持时不会报错，只会表现为
「测速失败，请稍后再试(6544)」——排查成本极高，宁可预防。

## 节点

`assets/resource_cloud_browser/pipeline/CloudBrowser/Portal.json`（识别文案全部来自实测页面）：

| 节点 | 说明 |
| --- | --- |
| `CloudBrowserPortalStartGame` | 门户右下角「开始游戏」，唯一需要主动点的入口 |
| `CloudBrowserDialogConfirmButton` | 通用「知道了」确认 |
| `CloudBrowserSpeedtestFailed` | 「测速失败(6544)」弹窗，`max_hit: 5` |
| `CloudBrowserNetworkAbnormal` | 「网络连接异常」弹窗，`max_hit: 3` |
| `CloudBrowserPortalReconnecting` | 「游戏正在重连中」，只识别不动作（单会话被顶掉的表征） |
| `CloudBrowserPortalLoggedIn` | 通行证 UID 行，用于判定已登录 |
| `CloudBrowserPortalLoginModal` | 「请输入手机号」，用于判定需要人工登录 |

### 入口链是「同名覆盖」，不是新建节点

`assets/tasks/CloudBrowserOpenGame.json` 的 `entry` 写的是基础节点 `OpenGame`，浏览器专属链路靠
`resource_cloud_browser/pipeline/CloudBrowser/OpenGame.json` **同名覆盖** `OpenGame.next` 实现。

原因有两条：

1. **任务 `entry` 必须存在于 base `resource`。** `interface.json` 的 `import` 是全控制器共享的，
   入口节点若只挂在某个 overlay 上，其它控制器加载不到，`maa-tools check` 会报
   「未知任务 / 未知入口任务」。`AndroidOpenGame` 同样是 `entry: "OpenGame"` + option 覆盖。
2. **不能直接用基础 `OpenGame` 链路。** 它的 `next` 末尾是 `StartUpGame`，而 `StartUpGame` 没有
   `recognition`、恒命中并去拉起进程——浏览器版没有可拉起的进程。

也不能反过来把门户节点塞进基础 `OpenGame`：门户的「开始游戏」与云·终末地安卓端
`CloudEndfieldStartGame`（ROI `[887, 522, 393, 198]`）区域重叠，同一条 `next` 里会互相抢命中。

进入游戏后的标题页仍然复用桌面端 `EnterGame`。

## 测试

`tests/CloudBrowser/` + `tests/MaaEndTestset/CloudBrowser/Official_CN/`，控制器矩阵在
`tests/scripts/loader.mts` 的 `controllerMap` 里注册。覆盖：

- 门户未登录（登录弹窗 + 开始游戏同时在场）、弹窗关闭后只剩开始游戏、被顶号后的重连提示；
- 云端串流出的标题页能被桌面端 `EnterGame` / `EnterGameOCR` / `EnterGameRecognition` 命中，
  启动 logo 帧全不命中——即 H264 压缩与云侧叠加层（12+ CADPA、沪ICP 备案号、版本号、ms 角标）
  不影响游戏内识别。

## 验证状态

> [!IMPORTANT]
>
> 以下结论来自 Linux Chromium 上的 Playwright 探测与节点测试，**尚未在真实 Windows + MaaFramework
> Win32 控制器上跑通**。第一次真机验证请按这个顺序看：
>
> 1. `ScreenDC` 能否截到 Chromium 窗口。Chromium 由 GPU 合成，GDI BitBlt 有截到黑帧的风险；
>    若识别全部失败，改 `interface.json` 里的 `screencap` 为 `FramePool` 或 `DXGI_DesktopDup_Window`。
> 2. `Seize` 鼠标/键盘能否真正进入页面（WebRTC 输入与鼠标视角需要真实事件）。
> 3. 进入游戏后页面标题是否仍含「云·终末地」，即 `window_regex` 是否还匹配。
> 4. 排队、时长耗尽、断线重连后的自动恢复分支尚未实现。
