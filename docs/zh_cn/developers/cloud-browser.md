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
内嵌 Activity 用 `config.densityDpi = DisplayMetrics.DENSITY_DEFAULT`（`CloudBrowserActivity.kt`）把
**Activity 的配置密度**压到 160，但 **WebView 的 `devicePixelRatio` 跟的是显示屏**：实测 dpr 1.5、
CSS 视口 854×480（详见缺口 2）。页面自己把内容按 CSS 缩放摆放，落回物理像素后仍是 1280×720，
所以**桌面版门户的 ROI 直接按物理像素复用**，不要按 dpr 折算、也不要声称它是 1280×720 CSS。
注意虚拟屏本身的 `densityDpi` 仍是手机的 240（`adb shell dumpsys display` 里看到的就是这个值），
被覆盖的只是 Activity 那一份 Configuration——按「虚拟屏密度=160」去查会查不到，别把这个数写进断言。
手机版门户布局不同，`resource_cloud_browser_mobile` 把「开始游戏」与登录框改回整帧 OCR。

同名节点是**按字段合并**而不是整节点替换（MaaFramework 多资源目录语义，仓库里的先例：
`resource_linux/pipeline/GameLauncher.json` 只重写 `next`，识别与 `action` 都沿用基础节点）。
所以手机版那份只写 `roi` 与 `expected` 的覆盖仍然继承基础节点的 `"action": "Click"`，
不必为了怕丢字段把整个节点抄一遍。

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

内嵌方案已在真机跑通到云端的游戏标题页（详见「内嵌真机实测」）：`CloudBrowserPC` 连上虚拟屏、命中
`CloudBrowserPortalStartGame`、由应用自己注入的鼠标点击打开鹰角登录弹窗；登录一次之后，
门户点「开始游戏」→ 遮罩等待 → 1280×720 串流标题页 → `EnterGame` 命中 → 任务成功。
手机版（`CloudBrowserMobile`）仍未测。

### 内嵌路径的已知缺口

下面每一条都读源码核对过，各自对应一种具体症状；已修或已实测的条目带着实测数字。
排查内嵌问题时先过一眼，别把已知问题当新 bug。

1. **打包配方会静默漏掉门户资源。** `Android/MaaFwApp/INTEGRATION.md` 的默认 include 本来覆盖
   `resource_*/**`，但 `Android/profile.yaml` 一旦显式写出 `include`，默认规则就整条被替换。
   门户 overlay 没进 `include` 时 APK 里根本没有门户节点，症状是「控制器连上了、什么也识别不到」，
   和截图方式坏了长得一模一样。现在 `resource_cloud_browser/**` 与 `resource_cloud_browser_mobile/**`
   已在 `include` 里。
   验收时别直接翻 APK 的文件清单：PI 树是 `syncPiAssets` → `packPiArchive` 打成
   `assets/pi.zip` 再塞进去的（松散 asset 会被 AAPT 改名），所以按 `resource_cloud_browser/` 搜 APK
   条目永远是 0，是个假阴性。要看的是 `packPiArchive` 打的「PI archive pi.zip … N entries」那行，
   或把 `assets/pi.zip` 解出来看它的根级目录。解包侧不用操心：`PiInstaller.unpackZip`
   按条目名原样铺开每一棵根目录，只额外要求顶层有 `interface.json`。
2. **坐标系基准和外置浏览器不同（已实测，节点命中）。** 外置那条截的是 `--app` 窗口客户区，抓帧 1280×720 中顶部约 31px 是
   标题栏、右侧 16px 是滚动条，CSS 视口实际只有 1264×681。内嵌 Activity 虽然在 `attachBaseContext` 里把
   `config.densityDpi` 钉到 160，但 **WebView 的 `devicePixelRatio` 跟的是显示屏而不是 Activity 配置**：
   2026-10-24 实测 `window.devicePixelRatio = 1.5`、`innerWidth/Height = 854×480`，即虚拟屏 1280×720
   在页面里是 854×480 个 CSS 像素。门户自己把关键区域按 CSS 缩放摆放（标题页的 `<video>` 布局宽
   `854 / 0.6667 ≈ 1281` CSS、再乘 `transform: scale(0.666667)` 落回 854 CSS，乘 dpr 1.5 正好铺满
   1280 物理像素），所以**截图仍是 1280×720、ROI 仍按物理像素写**，不按 dpr 折算。
   但响应式排版确实不同：2026-10-23 内嵌真机实测「开始游戏」文本框 (1045,660)-(1153,692)，落在原 roi 内；
   登录框「请输入手机号」却在 (402,211)-(549,239)，旧 roi 左边界 420 切进「请」字导致整行检不出，
   `CloudBrowserPortalLoginModal` 的 roi 已放宽到 `[380,180,520,180]` 同时容纳桌面与内嵌两种几何。
3. **cpp-algo 按控制器类型选资源根，内嵌两条会被当成安卓客户端。**
   `MapNavigator/controller_type_utils.h` 的 `ResourceImageRoots()` 只看类型，`native_android` 属于
   `kAdbLikeControllerTypes`，而 `MapLocateAction.cpp` 的 `DetectControllerType()` 读的是
   `MaaControllerGetInfo().type`——在手机上就是 `native_android`。于是跑着 Windows 客户端、桌面布局的
   `CloudBrowserPC` 也会拿到 `resource_adb` 优先的模板层次。要区分只能比较控制器名，而 cpp-algo 目前
   没有读 `PI_CONTROLLER`（读环境变量的先例是 `notice.cpp` 里的 `PI_CLIENT_LANGUAGE`）。
   另一点动手时会踩到：读类型的代码有两份。`MapNavigator/controller_info_utils.cpp` 的
   `DetectControllerType(MaaController*)` 被 `GameRegion.cpp`、`Backend/backend.cpp`、
   `position_provider.cpp` 复用，而 `MapLocateAction.cpp` 里有一份同名的本地副本，自己走
   `MaaControllerGetInfo` 再解析一次 JSON。按名字改写时两处都要覆盖，只改共享那份会留下小地图 ROI
   仍在按类型判定。
4. **画面稳定检测过去漏掉内嵌两条（已修）。** `scenemanager/screenshotstable.go` 的
   `shouldRunScreenshotStable()` 以前只认 `Win32-Front` 与 `pienv.ControllerNameCloudBrowser`，
   `CloudBrowserPC` / `CloudBrowserMobile` 不在名单里。云游戏有黑场首帧和加载帧，漏掉就意味着依赖
   稳定帧的节点会把没就绪的画面当有效画面用。现在统一走 `pienv.IsCloudBrowserController()`，
   三条控制器名都算——比较的仍然是名字，因为设备侧框架报的类型是 `native_android`。
5. **手机版缺 `OpenGame.next` 覆盖。** `resource_cloud_browser_mobile` 目前只覆盖了两个门户节点的 roi，
   没有覆盖入口链，所以 `CloudBrowserMobile` 继承的是电脑版那条以 `EnterGame` 打头的桌面序列。
   云安卓客户端进入游戏后的标题页本该走 `resource_adb`，`CloudEndfield*` 在当前配置下不可达。
   补上覆盖之前，手机版只能验证「从门户点进去」，不能验证进入游戏之后的链路。
6. **`<video>` 的画面几何（已标定）。** 门户里的云画面是页面内的一个 `<video>` 元素，而虚拟屏截的是整页
   1280×720。2026-10-24 内嵌真机实测：元素 `getBoundingClientRect()` 为 (0,0)-(854,480) CSS，
   `videoWidth×videoHeight = 1280×720`，`object-fit: fill`，外层 `transform: scale(0.666667)`，dpr 1.5
   ——换算到物理像素正好 1281×720，**整帧铺满、没有黑边**。串流标题页的截图与桌面 720p 同构，
   `EnterGameOCR` 的 roi（x ≥ W-250、y 200–600）覆盖到「点击任意位置继续」(约 1113,371)-(1250,390)，
   `EnterGameRecognition` 的公告/设置/修复按钮也在 (1067,0,213,276) 内，`resource_adb` 的 720p roi 可以直接用。
   唯一的变化来自视频压缩：文字有轻微拖影，但 2026-10-24 离线用例里新增的内嵌真帧（淡入完成后的那一帧）
   仍能命中三个模板节点。**淡入过程本身不算命中**：出帧后一两秒里画面还压着滚动的黄色色块 + 图标淡出，
   实测同一会话的淡入帧 `EnterGameOCR` 已是 0.9999，`EnterGameRecognition` 只有 0.39，`EnterGame` 的 `And`
   因此不成立——这是「等稳定帧」，不是「识别坏了」。所以遮罩撤掉后 NextList 先空一两轮是正常的。
7. **门户层还有孤儿节点与缺失分支。** `CloudBrowserPortalLoggedIn`（`Portal.json`）没有任何节点或 `next`
   引用它；排队（`排队中`）、时长耗尽、重连失败仍只有识别、没有恢复分支。
   启动遮罩这一支已补：`CloudBrowserGameStarting` 整帧 OCR「游戏启动中」、`DoNothing`、`rate_limit 2000`，
   在 `OpenGame.next` 里带 `[JumpBack]` 当等待态用（见「内嵌真机实测」第二轮）。i18n 那边不算缺口但值得记一笔：
   `tools/i18n/sync_ocr_expected.py` 只扫 `resource` / `resource_adb` / `resource_cloud_adb` 三个 pipeline，
   两个门户 overlay 不在扫描路径里（门户文案跟客户端语言无关），节点已逐个标 `@i18n-skip`，
   将来真把 overlay 纳入扫描也不会把「开始游戏」按语言表展开。
8. **内嵌鼠标注入点不动页面（已修）。** `InputControlUtils` 原本给 `ACTION_DOWN` 带上
   `buttonState = BUTTON_PRIMARY`，Chromium 把它当成「按键状态同步」而不是点击，页面只收到 `mousemove`，
   永远不派发 mousedown/mouseup/click——框架侧全是 `Delivered`，看起来像注入被吞。现已改为按下/抬起时
   `buttonState = 0` 并用 `InputManager.setActionButton(event, BUTTON_PRIMARY)` 表达是哪颗键，
   `ACTION_MOVE` 仍保留 `buttonState`（按住拖动时页面要能从 `buttons` 看出键还压着）。
   逐字段的实测过程记在下面「注入字段的定位方法」一节。
9. **门户节点是终点节点（已修）。** `OpenGame.next` 里不带 `[JumpBack]` 前缀的节点是终点：
   命中并执行完 `action` 就 `Tasker.Task.Succeeded`，之后页面发生什么再也没有一帧被识别。
   首版覆盖把 `CloudBrowserPortalStartGame` 与三个异常分支都写成了终点节点，所以「点击成功」之后
   登录弹窗出现这件事根本没人看见。现在门户节点全部 `[JumpBack]`，只有 `CloudBrowserOpenGameFailed`
   保持终点（判定失败就该收尾）。
10. **再点一次开始任务会打断正在串流的会话（已修）。** `CloudBrowserActivity` 是 `singleTask`，
    特权进程每一轮都会 launch 一次；旧的 `onNewIntent` 无条件 `loadPortal()`，于是画面已经在标题页、
    云实例正在跑的时候再点「开始任务」，页面被刷回门户，还因为页面注册了 `beforeunload` 弹出
    「确认离开」——这个原生弹窗盖在虚拟屏上，任何节点都识别不到，任务空转到失败。
    2026-10-24 真机实测就是这个症状（任务 08:07:32 起、08:07:51 失败）。现在 `mode` 与 `url` 都没变时
    只 `requestFocus()`，不重载。
11. **点「开始游戏」到遮罩出现之间有 1~2 秒空窗，可能连点（已缓解）。** 同一轮里
    `CloudBrowserPortalStartGame` 在 07:55:35.7 和 39.5 各命中一次，服务端 1.4 秒内收到两次
    `session/dispatch`，随后首个会话的 `<video>` 轨道直接 `ended`。节点加了 `post_delay: 2000`
    把连点压掉；遮罩出现后由 `CloudBrowserGameStarting` 接手，不会再点。

### 内嵌真机实测

#### 第一轮（2026-10-23）：门户层

在真机（MaaFwApp 本地出包并安装）跑通门户层，逐项对上上面的缺口：

- **Activity 与虚拟屏。** `CloudBrowserActivity: mode=pc` 起在虚拟屏，投屏成功。虚拟屏 id 每轮都不同
  （实测 4 → 6 → 7 → 8 → 9 → 10），任何脚本都不要把它当常量。
- **门户识别。** `CloudBrowserPortalStartGame` 命中 (1045,660,108,32)；`CloudBrowserPortalLoginModal`
  在放宽 roi 之后命中 (402,211)-(549,239)。这一帧已作为第 2 例进 `tests/CloudBrowser/test_portal.json`，
  2026-10-24 离线用例矩阵跑通（见「测试」一节）。
- **门户点击。** 应用自己注入的鼠标点击（走 `Win32` 那套 pc 模式通路）确实点开了鹰角登录弹窗，
  随后 `CloudBrowserOpenGameFailed` 命中并 StopTask——这正是期望行为：虚拟屏没有 IME，登录只能人工做。
- **预览镜像可点。** 任务页的全屏预览是唯一的交互入口（小卡片不转发触摸），映射为
  `screen = (288 + 1.5·vx, 1.5·vy)`（虚拟屏 1280×720 按 1.5 倍缩放贴进竖屏，显示旋转为横屏），
  三点标定核对过。修好缺口 8 之后连点 8 发的探针 8/8 命中，只有一次冷启动首帧落空。
  预览的多指槽位会被 `RemoteServiceImpl` 传成非 0 contact，pc 模式必须经 `InputControlUtils.previewContact`
  压回 0，否则 `SOURCE_MOUSE` 事件带不上指针。
- **PI 资源更新踩点。** `PiInstaller.isCurrentInstall()` 只在 `versionCode` 变化时重新解包
  （当前 3698），改资源不会自动生效。调 roi 时直接把单个 JSON 推进
  `files/pi/resource_cloud_browser/pipeline/CloudBrowser/`，属主 `u0_a51:sdcard_rw`、权限 660；
  正式回归仍要走重新出包。

#### 第二轮（2026-10-24）：登录 → 启动遮罩 → 云端标题页

这一轮补上了缺口 6，也把第二轮踩到的三个问题（缺口 10、11 与等待节点）修掉：

- **登录。** 云实例的登录只在主屏的 `CloudBrowserLoginActivity` 做（虚拟屏没有 IME，且 WebView 输入焦点
  会被虚拟屏的页面抢走）：绑定手机号 + 短信验证码，登录后门户显示通行证 UID、畅玩卡剩余 23天16小时、
  免费时长 565分钟。账号与验证码不写进文档。Cookie 落在应用私有目录 `app_webview/`，之后虚拟屏里直接是登录态。
  **这些 Cookie 不要提交到任何仓库。**
- **点击 → 启动遮罩。** 07:55 那轮由注入点击触发 `session/dispatch`，但服务端在 1.4 秒内收到两次
  dispatch（见缺口 11），首个会话的 `<video>` 轨道随后 `ended`（控制台一行
  `[reconnectServer], isOnceStartError`），页面停在「游戏启动中，请耐心等待」；`OpenGame.next` 没有等待节点，
  20 秒后整条判失败（`Tasker.Task.Failed` 07:56:17）。此后的两分钟里控制台没有任何新错误，纯属空转。
- **启动遮罩 → 标题页。** 另起一轮（页面重载后点「开始游戏」，`/session/dispatch` 与 `/user/exchange` 均 200）
  后约 20 秒 `<video>` 出帧：`videoWidth×videoHeight = 1280×720`、轨道 `live`、标题页
  「明日方舟 终末地 / 点击任意位置继续 / 公告·设置·修复」整帧铺满 1280×720。
  淡入完成后的同一帧存为 `云浏览器_云流_内嵌标题页.png`，`EnterGame` / `EnterGameOCR` / `EnterGameRecognition`
  三个节点都能命中（离线矩阵见「测试」）。
- **串流几何。** 见缺口 6 的实测数字：854×480 CSS × dpr 1.5、视频布局 1281×720 CSS、`object-fit: fill`，
  物理 1280×720 无黑边。`resource_adb` 的 720p roi 不需要重标。
- **整条链路的收尾（08:28 那轮，装上新包之后）。** Activity 起在虚拟屏 10；门户检测到账号已有会话，
  自动 `dispatch` 接回；`CloudBrowserGameStarting` 从 08:28:26 起每 ~2 秒命中一次（DoNothing），
  08:28:48 遮罩消失、有一轮什么都没命中，08:28:50 `EnterGame` 命中，
  08:28:51 `Tasker.Task.Succeeded`。`CloudBrowserOpenGame` 的语义就是「停在标题页」，所以任务到这就结束。
- **仍未验证。** `CloudBrowserMobile` 整条（缺口 5）；断线重连与排队分支；按住拖动的语义（见「后续回归顺序」）。

### 注入字段的定位方法

不重新出包也能逐个字段试注入，靠的是一个 `app_process` 注入台：`javac -bootclasspath android.jar` →
`d8`（要先 `export JAVA_HOME` 且 `--output` 目录已存在）→ `jar` → `adb push /data/local/tmp` →
`CLASSPATH=… app_process /system/bin Inj …`。`InputManager.injectInputEvent` + `InputEvent.setDisplayId`
在 uid shell 与 uid root 下都可用；`InputManagerGlobal` 从 app_process 加载不到，得先
`Looper.prepareMainLooper()` → `ActivityThread.systemMain()` → `getSystemContext()` →
`getSystemService("input")`——和 MaaFwApp 里 `FakeContext` / `Workarounds.getSystemContext` 同一个套路。

对弹窗上的 ✕ 逐字段二分，结论是：现状形状（`toolType=MOUSE` + `buttonState=PRIMARY`）不生效；
只改 `buttonState=0` 生效；只改 `toolType=UNKNOWN` 生效；`buttonState=0` 且 `setActionButton(PRIMARY)`
且保持 `toolType=MOUSE` 也生效——即最终采用的组合。`deviceId` 改成 -1 无效，
单独往 `buttonState=PRIMARY` 上加 `actionButton` 也无效。adb 的 `input mouse` 之所以一直点得动，
是因为它用的是 `MotionEvent.obtain(dt, et, action, x, y, metaState)` 简单工厂，`buttonState` 天生为 0
（顺带一提 `toolType` 是 UNKNOWN、`deviceId` 是 -1）；`/system/bin/input` 是 `cmd input "$@"`，
跑在 system_server 里（uid system），而 MaaFwApp 的注入发生在 `…:root_service`（uid shell）——
差异在事件字段，不在调用者身份。排查这类「adb 能点、应用点不动」时先比字段，别再怀疑权限。

### 后续回归顺序

上面那轮已覆盖 1–4 步（Activity 起屏、门户命中、鼠标注入、roi 复核），剩下这些按序做：

1. 拿到能登录的账号后，先在设置页登录门户（虚拟屏没有 IME，验证码只能在主屏 WebView 输），
   Cookie 共享后虚拟屏里即为已登录态。
2. 选 `CloudBrowserPC` 跑 `CloudBrowserOpenGame`，`adb logcat` 里 grep `CloudBrowserActivity: mode=`
   （创建时打的 `mode` / `display` / `url`）确认 Activity 起在预期虚拟屏。投不上时报的是
   「任务未出现在虚拟屏」，后面会带上 `presence` 与 app 侧 `appLaunch` 异常；`EmbeddedBrowserPresence`
   靠页面自报，因为 `getRunningTasks` 在部分 ROM 上看不见 `excludeFromRecents` 的任务。
3. 过掉登录弹窗之后再看云端标题页能否被 `EnterGame` / `EnterGameRecognition` 命中；
   这一步同时量缺口 6（`<video>` 是否铺满、有没有黑边）。
4. 手机版换 `CloudBrowserMobile`：先确认触摸注入有效、整帧 OCR 命中的是手机版布局，再补
   `OpenGame.next` 覆盖（缺口 5）。
5. 按住拖动的语义（`ACTION_MOVE` 带 `buttonState` 能否让页面识别成拖拽）仍未实测，
   需要一段页面上可拖的控件才能验。

## 四条硬约束

这四条是踩出来的，不满足就整条链路失效，改动 `agent/go-service/pretask/cloudbrowser/` 前请先读完。

1. **客户区必须正好 1280×720。** 仓库所有 ROI 与模板都以 720p 为基准；ADB 有 MaaFramework 自动缩放，
   桌面控制器截到多大就是多大。`--window-size` 设的是**外框**，所以 pretask 先量一次
   「外框 − 客户区」差值，再反推外框尺寸 `SetWindowPos`。
2. **DPI 必须 1:1。** Go 程序没有 manifest 时是 DPI-unaware，`GetWindowRect` 拿到的是 DWM 虚拟化坐标；
   系统缩放 125% 时页面会渲染成 1600×900 物理像素。因此进程启动即
   `SetProcessDpiAwarenessContext(PER_MONITOR_AWARE_V2)`，并给浏览器传 `--force-device-scale-factor=1`。
3. **只能有一个实例。** 云实例是单会话的，第二个客户端会把第一个挤回标题页（已实测）。
   pretask 按 `--user-data-dir=<绝对路径>` 匹配进程命令行复用已有实例，绝不启第二个。
   登录态就存在这个 profile 里（`os.UserConfigDir()/MaaEnd/cloud_browser`），不要挪进安装目录或版本库。
4. **动作执行期间门户窗口必须保持在最前。** `Seize` 输入是按绝对屏幕坐标投递的真实事件，不属于目标
   窗口的 HWND；框架只在 `ScreenDC` 抓帧前尝试抢前台，抓帧方式换成 `FramePool` 后连这一次都没有。
   pretask 只在启动时置顶一次，之后被别的窗口盖住时识别仍成功、动作照报成功，点击却打进遮挡窗口。
   详见「FramePool 修好了识别，但输入仍然要求窗口在最前」。

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

- 门户未登录（登录弹窗 + 开始游戏同时在场）、**内嵌虚拟屏上的同一弹窗**（排版比桌面偏左上，
  第 2 例用真机抓帧 `云浏览器_门户_内嵌登录弹窗.png`）、弹窗关闭后只剩开始游戏、被顶号后的重连提示、
  **内嵌点了开始游戏之后的启动遮罩**（`云浏览器_门户_内嵌启动中.png`，只有「游戏启动中」可认）；
- 云端串流出的标题页能被桌面端 `EnterGame` / `EnterGameOCR` / `EnterGameRecognition` 命中
  （`云浏览器_云流_内嵌标题页.png` 是真机串流帧，不是桌面截图），
  启动 logo 帧全不命中——即 H264 压缩与云侧叠加层（12+ CADPA、沪ICP 备案号、版本号、ms 角标）
  不影响游戏内识别。

本机跑法与耗时（2026-10-24 实测）：`npm test` 要先让 maa-tools 联网 `preparing maafw`
（download-scripts → download-binary → move-folders），这几步只打步骤名、不打进度，头两三分钟看着像挂死；
本表 shell 里没有 `node`，要用随 Qoder 带的 Node：
`ELECTRON_RUN_AS_NODE=1 "<Qoder CN.exe>" node_modules/@nekosu/maa-tools/bin/maa-tools test`。
整套 1449 个用例约 18 分钟跑完，退出码 0、`tests/maatools/error_details.json` 没有生成，
新增的内嵌帧用例（`云浏览器_门户_内嵌登录弹窗.png`、`云浏览器_门户_内嵌启动中.png`、
`云浏览器_云流_内嵌标题页.png`）也在其中通过。

## 验证状态

2026-10-23 在真实 Windows 上跑通了外置浏览器链路：Snapdragon 8cx Gen 2 / Adreno 685 平板，
2560×1600 系统缩放 200%，MaaFramework v5.14.3（Windows ARM64），`agent/go-service --pretask
CloudBrowserLaunch` 退出码 0，Edge 以 `--app` 打开门户且客户区正好 1280×720，
Win32 控制器连上窗口、OCR 命中门户文本。因此前三条硬约束均按预期工作；第四条（动作期间窗口保持在
最前）在这次端到端里恰好被违反了，见下文。
用生产 `OpenGame` + `resource_cloud_browser` 端到端跑了一遍：`reco hit
[result.name=CloudBrowserPortalStartGame]` → `Click` → `Node.Action.Succeeded` →
`### All tasks have been completed ###`；不过那次点击的屏幕坐标正好压在用户窗口上（见下文
「FramePool 修好了识别，但输入仍然要求窗口在最前」），所以只证明链路能跑，不证明云端真的开始排队。

2026-10-23 在真机上跑通了 MaaFwApp 内嵌浏览器的门户层：本地出包安装，`CloudBrowserPC` 起虚拟屏、
命中「开始游戏」、应用自己注入的鼠标点击真把鹰角登录弹窗点了出来（修缺口 8 之前点不动）。
2026-10-24 登录后继续跑通到云端标题页：注入点击 → 启动遮罩 → 1280×720 串流 → `EnterGame` 命中 →
`Tasker.Task.Succeeded`。两轮的细节见「内嵌真机实测」。

`CloudBrowserMobile` 至今没有真机数据。离线用例矩阵 2026-10-24 在本机全绿，含内嵌登录弹窗与内嵌启动遮罩、
内嵌串流标题页三帧。其余仍未验证：排队 / 时长耗尽 / 断线重连的自动恢复分支（尚未实现）、
按住拖动的注入语义、进入游戏后标题是否仍含「云·终末地」（`window_regex`，本轮任务在标题页收尾，没往下走）。

### 截图方式怎么选

同一窗口、同一几何下逐个跑 `speed_test` + 全帧 OCR，并用一个置顶窗口盖住门户左半侧做对照：

| `screencap` | 实际方法位 | 连接 | 单帧耗时 | 正常前台 | 被置顶窗口遮挡 |
| --- | --- | --- | --- | --- | --- |
| `FramePool` | 2 | 可 | 30–33ms | 完整页面 | **仍是完整页面**，且不动 z-order |
| `ScreenDC` | 32 | 可 | 24–28ms | 完整页面 | **把遮挡物一起截进来**，门户关键控件全部丢失 |
| `PrintWindow` | 16 | 可 | ~253ms | 完整页面 | 未测 |
| `GDI` | 1 | 可 | ~7ms | 纯黑帧 | — |
| `DXGI_DesktopDup` | 4 | **不可** | — | `DuplicateOutput` 返回 `E_ACCESSDENIED`（Adreno 685） | — |
| `DXGI_DesktopDup_Window` | 8 | **不可** | — | 同上 | — |

所以默认值取 `FramePool`。`ScreenDC` 抓的是屏幕像素，MaaFramework 每次抓帧前都要把目标窗口提到
最上层才截得对（日志里 `Failed to ensure foreground window before screencap` 只是前台权限被拒，
z-order 已经改了），既抢用户窗口层级，又挡不住任何置顶窗口；`GDI` 在 Chromium 上直接全黑；
`DXGI_*` 在部分集显上被驱动拒绝。`Foreground` / `Background` 不是 ProjectInterface 认可的字符串，
写进去会静默退化成 `DXGI_DesktopDup`，别照抄 MaaFramework 内部枚举名。

### 坐标必须按抓帧尺寸标定

`--app` 模式下 Chromium 把标题栏画在**客户区内部**，所以抓帧 1280×720 里：顶部约 31px 是标题栏，
右侧 16px 是页面滚动条，底部 8px 留白，CSS 视口只有 1264×681。门户的「开始游戏」贴在页面右下角、
登录框垂直居中，**y 随窗口高度浮动**，按某一次实测值卡死 roi 会在别的机器上整体错位。
`Portal.json` 里这两个节点的 roi 因此取覆盖所在象限的宽松值，实测命中：
「开始游戏」文本 (1124,678)-(1197,702)、「请输入手机号」(481,276)-(580,295)，score 0.9999。

另一个坑：页面加载完成后，`SetWindowPos` 只能改宽度，高度会被页面的 min-content 顶回 1622px。
所以客户区 1280×720 只能靠启动时的 `--window-size` 保证，事后补救不了。

### 「截到的全是白图」多半不是截图方式的问题

Modern Standby 把会话挂起或息屏后，`document.visibilityState` 变成 `hidden`，Chromium 直接停止绘制，
此时**任何**窗口截图方式拿到的都是白帧，看起来像截图方法坏了。排查顺序：先看桌面时钟是否还走，
再用 CDP（`--remote-debugging-port`）读 `visibilityState`。后台长跑前记得
`SetThreadExecutionState(ES_CONTINUOUS | ES_DISPLAY_REQUIRED | ES_SYSTEM_REQUIRED)`。

### FramePool 修好了识别，但输入仍然要求窗口在最前

`Seize` 鼠标/键盘走的是 `SendInput` 绝对屏幕坐标，**不是**按 HWND 投递消息：日志里
`SeizeInput::touch_down [x=1179][y=688][point.x=1187][point.y=688][hwnd_=true]` 的 `point`
就是屏幕坐标，谁的 z-order 压在这个点上，点击就落在谁身上。MaaFramework 只在两处主动抢前台
（扫 `MaaWin32ControlUnit.dll` 的字符串只剩这两条）：`ScreenDCScreencap` 抓帧前、
`InterceptionInput` 键盘注入前；ProjectInterface 的 `win32` 块也只有
`class_regex` / `window_regex` / `screencap` / `mouse` / `keyboard` 五个键（`MaaPiCli.exe` 里的
`win32_config.*` 字符串同样只有这些），没有任何置顶选项。

于是换到 `FramePool` 之后出现了一个反直觉的组合：抓帧不再需要前台，被遮挡也能识别成功，
`Node.Action.Succeeded` 照样报绿，但点击其实打进遮挡它的那个窗口。用 `ScreenDC` 时每次抓帧顺带的
z-order 提升把这个差异盖住了。pretask `CloudBrowserLaunch` 只在启动时 `ShowWindow(swRestore)` +
`SetForegroundWindow` 一次，任务跑起来之后不会有第二次。

结论：**任务运行期间门户窗口必须保持前台且不被遮挡。** 需要自动化保证的话，只能由调用方
（GUI / pretask）在动作前重新置顶门户；设成 `HWND_TOPMOST` 最稳但会压住用户所有窗口，
不适合作为默认行为。

### 门户页会假命中桌面端标题页节点

真机端到端跑生产 `OpenGame` 链时，`EnterGameResourceInitializeCompleteText` 命中了登录弹窗里的
「已同意《用户注册协议》…」（box `[465,403,368,17]`），`EnterGameRecognition` 的模板匹配在 12+ CADPA
角标上拿到 0.427 的候选（box `[1183,29,31,32]`，低于阈值才算失败）。两者只是整条 `And` 未成立，
才没抢走执行权——这条 `next` 的顺序是脆弱的，改动 `OpenGame.next` 或调低这些节点阈值之前，
先在门户页回归一遍。

### 用 stock MaaPiCli 手动验证的注意点

MaaEnd 生产 `interface.json` 加载不进 v5.14.3 的 `MaaPiCli.exe`：
`Parser::parse_import_data json is not a valid ImportData [error_key=option]`，来自
`assets/tasks/setting/Keymap.json` 的 `"type": "hotkey"` + `hotkeys` 自定义 option，最终
`Failed to parse interface.json`。手动验证时另写一份不含 `import`、只挂
`["./resource", "./resource_cloud_browser"]` 与 `CloudBrowserOpenGame`（`entry: "OpenGame"`）
的最小 interface 即可复现同一条链路。
