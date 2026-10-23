# Cloud Browser (CloudBrowser) Controller

Opens the web version of Cloud Endfield (<https://endfield.hypergryph.com/cloud>) in a local
Chrome / Edge window and drives that window with the Win32 controller. Users do not need to
download the Android Cloud Endfield client or an APK.

The cloud instance runs the **Chinese-server Windows client** (observed as `CN_WIN_REL_1.6.3`
on an X86 instance), so every in-game pipeline is reused straight from the desktop `resource`.
This controller only adds the "web portal → enter game" segment.

## Resource layering

| Directory | Purpose |
| --- | --- |
| `assets/resource` | Desktop baseline; all in-game nodes come from here |
| `assets/resource_cloud_browser` | Portal-layer nodes plus a same-name override of `OpenGame.next` |

In `interface.json`, `CloudBrowser` attaches only `./resource_cloud_browser`, unlike `CloudADB`
which also attaches `./resource_adb` — the web cloud is not an Android client.

ProjectInterface v2 has no browser `controller[].type`, so `CloudBrowser` borrows `Win32`.
**Any code that must tell it apart from a real desktop client has to compare the controller name,
not the type.** The constant lives in `pkg/pienv`: `pienv.ControllerNameCloudBrowser`.

## Android embedded browser (MaaFwApp)

MaaFwApp additionally provides two controllers that do not need an external browser; they open
the portal in an in-app WebView on a virtual display:

| Controller | Cloud platform | Attached resources |
| --- | --- | --- |
| `CloudBrowserPC` | PC (the cloud runs the Windows client) | `./resource_cloud_browser` |
| `CloudBrowserMobile` | Mobile (the cloud runs the Android client) | `./resource_adb`, `./resource_cloud_browser`, `./resource_cloud_browser_mobile` |

The portal picks the cloud platform from the `pointerType` of the "开始游戏" click: mouse → PC,
touch → mobile. PC mode therefore injects native `SOURCE_MOUSE` events through
`InputControlUtils` (down with `WAIT_FOR_FINISH`, unpressed movement as `ACTION_HOVER_MOVE`,
pressed movement as `ACTION_MOVE`), while mobile mode keeps touch injection. The embedded
Activity overrides its *configuration* density with `config.densityDpi = DisplayMetrics.DENSITY_DEFAULT`
(`CloudBrowserActivity.kt`) so the 1280×720 virtual display maps to 1280×720 CSS pixels and the
desktop portal ROIs still apply. Note that the virtual display itself still reports the phone's
240 dpi (`adb shell dumpsys display` shows that number); only the Activity's Configuration is
overridden, so do not assert "the virtual display density is 160". The mobile portal layout differs,
so `resource_cloud_browser_mobile` switches the "开始游戏" button and login form back to full-frame
OCR.

Same-name nodes are merged **field by field**, not swapped wholesale (MaaFramework's multi-resource
semantics; the in-repo precedent is `resource_linux/pipeline/GameLauncher.json`, which rewrites only
`next` and keeps the base node's recognition and action). So the mobile overlay that carries only
`roi` and `expected` still inherits the base node's `"action": "Click"` — there is no need to copy the
whole node for fear of losing fields.

The PI protocol rejects unknown `controller[].type` values (MaaPiCli rejects the whole
interface), so both controllers are still declared as `Win32` and carry an
`embedded_browser: { "mode": "pc" | "mobile" }` marker. Desktop clients ignore the field and use
the `win32` block to grab an external Chrome window; MaaFwApp switches to the embedded WebView
when it sees the marker. The marker is therefore also how MaaFwApp decides whether a controller
is usable — do not key that decision on the type.

The virtual display hides the IME, so first-time login and SMS codes cannot be typed there.
MaaFwApp's settings page offers "Sign in to Endfield Cloud", which opens the same portal in a
normal WebView on the primary display; cookies and localStorage are shared application-wide, so
after signing in once the portal inside the virtual display is already authenticated.

The embedded browser reuses the same portal nodes and `OpenGame.next` override. In-game
resources follow the cloud platform: PC uses the desktop `resource`, mobile also attaches
`resource_adb`. `CloudBrowser` (external) remains desktop-only, and its pretask does not run in
MaaFwApp.

The embedded path is now verified on a real device down to the portal layer (see "First embedded
device run"): `CloudBrowserPC` attaches to the virtual display, `CloudBrowserPortalStartGame` hits,
and a mouse click injected by the app itself opens the Hypergryph login modal. Nothing past that
modal has been verified yet.

### Known gaps on the embedded path

Every item here was checked against the source, not guessed, and each one maps to a concrete
symptom; fixed or measured items carry the numbers. Read this before debugging the embedded path.

1. **Packaging silently drops the portal resources.** The default include in
   `Android/MaaFwApp/INTEGRATION.md` covers `resource_*/**`, but once `Android/profile.yaml` states
   an explicit `include`, the default is replaced entirely. Without the portal overlays in that list
   the APK has no portal nodes at all, which looks exactly like "the controller connected but nothing
   is ever recognized" — indistinguishable from a broken screencap method. `resource_cloud_browser/**`
   and `resource_cloud_browser_mobile/**` are in `include` now.
   When verifying, do not grep the APK's own file listing: the PI tree reaches the package as
   `syncPiAssets` → `packPiArchive` → `assets/pi.zip` (loose assets get rewritten by AAPT), so
   searching the APK entries for `resource_cloud_browser/` always returns 0 — a false negative.
   Look at the "PI archive pi.zip … N entries" line from `packPiArchive`, or unpack `assets/pi.zip`
   and inspect its root directories. Nothing to worry about on the install side:
   `PiInstaller.unpackZip` lays out every root verbatim and only additionally requires a top-level
   `interface.json`.
2. **The coordinate basis differs from the external browser (now measured).** The external path
   captures the `--app` client area, where the captured 1280×720 frame carries about 31px of title
   bar on top and a 16px scrollbar on the right, leaving a 1264×681 CSS viewport. The embedded
   Activity overrides `config.densityDpi` to 160, so the 1280×720 virtual display is a clean
   1280×720 CSS viewport. On the 2026-10-23 embedded device run the "开始游戏" text box measured
   (1045,660)-(1153,692) — inside the existing ROI, no density conversion needed — while the login modal's
   "请输入手机号" measured (402,211)-(549,239): the old ROI's left edge at 420 cut into the first
   glyph and OCR then found no line at all. `CloudBrowserPortalLoginModal`'s ROI is now
   `[380,180,520,180]`, wide enough for both the desktop and the embedded geometry.
3. **cpp-algo picks resource roots by controller type, so both embedded controllers look like an
   Android client.** `ResourceImageRoots()` in `MapNavigator/controller_type_utils.h` only looks at the
   type, `native_android` is in `kAdbLikeControllerTypes`, and `DetectControllerType()` in
   `MapLocateAction.cpp` reads `MaaControllerGetInfo().type` — which is `native_android` on the phone.
   So `CloudBrowserPC`, which streams a desktop-layout Windows client, still gets the
   `resource_adb`-first template layering. Telling them apart requires the controller name, and
   cpp-algo does not read `PI_CONTROLLER` today (the precedent for reading the environment is
   `PI_CLIENT_LANGUAGE` in `notice.cpp`).
   One more thing that bites while implementing: the type lookup exists twice.
   `DetectControllerType(MaaController*)` in `MapNavigator/controller_info_utils.cpp` is reused by
   `GameRegion.cpp`, `Backend/backend.cpp` and `position_provider.cpp`, while `MapLocateAction.cpp`
   carries a local namesake that calls `MaaControllerGetInfo` and re-parses the JSON itself. A
   name-based fix has to cover both, otherwise the minimap ROI keeps switching on the type.
4. **Frame stabilization missed the embedded controllers (now fixed).** `shouldRunScreenshotStable()`
   in `scenemanager/screenshotstable.go` used to accept only `Win32-Front` and
   `pienv.ControllerNameCloudBrowser`, leaving `CloudBrowserPC` and `CloudBrowserMobile` out. Cloud
   gaming emits black first frames and loading frames, so anything relying on a stable frame could
   consume a frame that is not ready yet. It now calls `pienv.IsCloudBrowserController()`, which covers
   all three names — still compared by name, because on-device the framework reports
   `native_android`.
5. **Mobile is missing its `OpenGame.next` override.** `resource_cloud_browser_mobile` currently only
   overrides the ROI of two portal nodes and never touches the entry chain, so `CloudBrowserMobile`
   inherits the PC-mode desktop sequence that starts with `EnterGame`. The title page of the cloud
   Android client should be reached through `resource_adb`, and `CloudEndfield*` is unreachable under
   the current configuration. Until that override exists, mobile can only verify "clicked into the queue
   from the portal", not anything after entering the game.
6. **The `<video>` geometry is uncalibrated.** The streamed picture is a `<video>` element inside the
   portal page, while the virtual display captures the whole 1280×720 page. Whether the picture fills
   the viewport, and whether there are letterbox bars, decides whether the 720p ROIs of `resource_adb`
   can be used as-is. That can only be measured on device.
7. **Orphan nodes and missing branches in the portal layer.** Nothing references
   `CloudBrowserPortalLoggedIn`, neither a node nor a `next`; queueing, exhausted playtime and
   reconnecting are recognized but have no recovery branch. The i18n part is not a gap but is worth
   recording: `tools/i18n/sync_ocr_expected.py` scans only the `resource`, `resource_adb` and
   `resource_cloud_adb` pipelines, so neither portal overlay is in scope (portal wording does not
   follow the client language), and every portal node now carries `@i18n-skip` so that adding the
   overlays later cannot expand "开始游戏" through the language tables.
8. **Embedded mouse clicks did nothing (now fixed).** `InputControlUtils` used to put
   `buttonState = BUTTON_PRIMARY` on `ACTION_DOWN`. Chromium reads that as a button-state sync rather
   than a click, so the page only received `mousemove` and never dispatched
   mousedown/mouseup/click — every injection reported `Delivered`, which looked exactly like the
   input being swallowed. Down and up now carry `buttonState = 0` with
   `InputManager.setActionButton(event, BUTTON_PRIMARY)` to say which button moved; `ACTION_MOVE`
   keeps `buttonState`, so a drag still shows the held button in `buttons`. The field-by-field
   bisect is recorded under "How the injection fields were traced".
9. **Portal nodes were terminal nodes (now fixed).** In `OpenGame.next`, a node without the
   `[JumpBack]` prefix is terminal: once it hits and its `action` finishes, `Tasker.Task.Succeeded`
   fires and no later frame is ever recognized. The first version of the override left
   `CloudBrowserPortalStartGame` and the three error branches as terminal nodes, so nobody ever saw
   the login modal appear after the "successful" click. All portal nodes are now `[JumpBack]`, and
   only `CloudBrowserOpenGameFailed` stays terminal (deciding the task failed is a real endpoint).

### First embedded device run

2026-10-23, on a real phone with an APK built locally and installed. The portal layer works:

- **Activity and virtual display.** `CloudBrowserActivity: mode=pc` started on the virtual display
  and mirroring worked. The virtual display id changes every run (observed 4 → 6 → 7 → 8); never
  hard-code it in a script.
- **Portal recognition.** `CloudBrowserPortalStartGame` hit (1045,660,108,32); after the ROI
  widening, `CloudBrowserPortalLoginModal` hit (402,211)-(549,239). That frame is now case 4 of
  `tests/CloudBrowser/test_portal.json` and passed the offline matrix on 2026-10-24 (see "Tests").
- **Portal click.** The app's own injected mouse click really did open the Hypergryph login modal,
  after which `CloudBrowserOpenGameFailed` hit and stopped the task — the expected outcome, since
  the virtual display has no IME and logging in is a human job.
- **The preview mirror is clickable.** The full-screen preview on the task page is the only
  interactive surface (the small card does not forward touches). Mapping is
  `screen = (288 + 1.5·vx, 1.5·vy)` — the 1280×720 virtual display scaled 1.5x into the portrait
  screen, with the display rotated to landscape — verified against three calibration taps. After
  gap 8 was fixed, a probe that tapped the modal's ✕ eight times hit 8/8, with one miss only during
  a cold start. Preview touches arrive with a non-zero contact index from `RemoteServiceImpl`, and
  pc mode must funnel them through `InputControlUtils.previewContact` back to 0, otherwise the
  `SOURCE_MOUSE` event carries no pointer.
- **PI resource refresh.** `PiInstaller.isCurrentInstall()` only re-unpacks when `versionCode`
  changes (currently 3698), so editing resources does not take effect by itself. While tuning ROIs,
  push single JSON files into `files/pi/resource_cloud_browser/pipeline/CloudBrowser/` with owner
  `u0_a51:sdcard_rw` and mode 660; a real regression still needs a rebuilt APK.
- **Still unverified.** Everything after the login modal — it needs a real account and an SMS code.
  Gaps 5 (mobile `OpenGame.next`) and 6 (`<video>` geometry) therefore produced no data, and
  `CloudBrowserMobile` has not been run at all.

### How the injection fields were traced

Field-by-field experiments need no APK rebuild: a small `app_process` harness does it. Compile with
`javac -bootclasspath android.jar`, dex with `d8` (export `JAVA_HOME` first, and the `--output`
directory must already exist), `jar` it up, `adb push` to `/data/local/tmp`, then
`CLASSPATH=… app_process /system/bin Inj …`. `InputManager.injectInputEvent` plus
`InputEvent.setDisplayId` work as uid shell and as uid root; `InputManagerGlobal` cannot be loaded
from app_process, so bootstrap it with `Looper.prepareMainLooper()` →
`ActivityThread.systemMain()` → `getSystemContext()` → `getSystemService("input")` — the same trick
as `FakeContext` / `Workarounds.getSystemContext` in MaaFwApp.

Bisecting against the ✕ on the modal gave: the old shape (`toolType=MOUSE` with
`buttonState=PRIMARY`) does nothing; `buttonState=0` alone works; `toolType=UNKNOWN` alone works;
and `buttonState=0` **with** `setActionButton(PRIMARY)` while keeping `toolType=MOUSE` also works —
that is the combination that shipped. Changing `deviceId` to -1 does nothing, and adding
`actionButton` on top of `buttonState=PRIMARY` still does nothing. adb's `input mouse` always worked
because it uses the simple `MotionEvent.obtain(dt, et, action, x, y, metaState)` factory, where
`buttonState` is 0 by construction (and `toolType` is UNKNOWN, `deviceId` -1).
`/system/bin/input` is a `cmd input "$@"` script running in system_server as uid system, while
MaaFwApp injects from `…:root_service` as uid shell — so the difference is in the event fields, not
in the caller's identity. When "adb can click but the app cannot", compare fields before blaming
permissions.

### Ordering for the remaining runs

The run above already covered the first four items of the original checklist (the Activity starting
on the virtual display, the portal nodes hitting, mouse injection, the ROI check). What is left, in
order:

1. Once an account that can sign in is available, sign in from Settings first (the virtual display
   has no IME, so the SMS code can only be typed in the ordinary WebView on the primary display);
   cookies are shared, so the portal inside the virtual display is then already authenticated.
2. Pick `CloudBrowserPC`, run `CloudBrowserOpenGame` and grep `adb logcat` for
   `CloudBrowserActivity: mode=` (the `mode` / `display` / `url` line logged at creation) to confirm the
   Activity really started on the expected virtual display. When it does not, the error is
   "任务未出现在虚拟屏" and now carries `presence` plus the app-side `appLaunch` exception;
   `EmbeddedBrowserPresence` relies on the page self-reporting because `getRunningTasks` cannot see
   `excludeFromRecents` tasks on some ROMs.
3. Past the login modal, re-check whether the streamed title page is hit by `EnterGame` /
   `EnterGameRecognition`. This is also where gap 6 gets measured: whether the `<video>` fills the
   viewport and whether it has letterbox bars.
4. For mobile, switch to `CloudBrowserMobile`: first prove touch injection works and that full-frame
   OCR hits the mobile portal layout, then add the `OpenGame.next` override (gap 5).
5. Drag semantics while the button is held (`ACTION_MOVE` carrying `buttonState`) are still untested;
   verifying them needs a draggable control on the page.

## Four hard constraints

These were learned the hard way; the whole chain breaks without them. Read this before touching
`agent/go-service/pretask/cloudbrowser/`.

1. **The client area must be exactly 1280×720.** Every ROI and template in the repository is
   based on 720p. ADB gets automatic scaling from MaaFramework, a desktop controller captures
   whatever size the window is. `--window-size` sets the **outer frame**, so the pretask measures
   the "frame minus client area" delta once and derives the outer size for `SetWindowPos`.
2. **DPI must be 1:1.** A Go binary without a manifest is DPI-unaware and `GetWindowRect` returns
   DWM-virtualized coordinates; at 125% system scaling the page renders as 1600×900 physical
   pixels. The process therefore calls `SetProcessDpiAwarenessContext(PER_MONITOR_AWARE_V2)` at
   startup and passes `--force-device-scale-factor=1` to the browser.
3. **Only one instance may run.** The cloud session is single-seat: a second client kicks the
   first one back to the title screen (observed). The pretask matches `--user-data-dir=<absolute
   path>` in process command lines and reuses the existing instance instead of starting another.
   The login state lives in that profile (`os.UserConfigDir()/MaaEnd/cloud_browser`) — keep it out
   of the install directory and out of version control.
4. **The portal window must stay on top while actions run.** `Seize` input is a real event injected
   at an absolute *screen* coordinate, not something posted to the target HWND; MaaFramework only
   tries to take the foreground before a `ScreenDC` capture, so once `screencap` is `FramePool` even
   that disappears. The pretask raises the window once at startup; if another window covers the
   portal later, recognition still succeeds, the action still reports success, and the click goes
   into the occluding window. See "FramePool fixed recognition, but input still needs the window on
   top".

The pretask also always passes `--proxy-bypass-list` for `vlinkcloud.cn` / `hypergryph.com` /
`hycdn.cn` / `skland.com`. WebRTC media uses UDP; when a system HTTP proxy swallows it there is
no error, only "测速失败，请稍后再试(6544)" (speedtest failed). That is expensive to diagnose,
so we prevent it.

## Nodes

`assets/resource_cloud_browser/pipeline/CloudBrowser/Portal.json` (every expected string comes
from the real page):

| Node | Purpose |
| --- | --- |
| `CloudBrowserPortalStartGame` | The "开始游戏" button at the bottom right — the only entry we click |
| `CloudBrowserDialogConfirmButton` | Generic "知道了" confirmation |
| `CloudBrowserSpeedtestFailed` | Speedtest failed (6544) dialog, `max_hit: 5` |
| `CloudBrowserNetworkAbnormal` | Network abnormal dialog, `max_hit: 3` |
| `CloudBrowserPortalReconnecting` | "游戏正在重连中"; recognition only, no action (symptom of being kicked by a second session) |
| `CloudBrowserPortalLoggedIn` | Passport UID row, used to tell that we are logged in |
| `CloudBrowserPortalLoginModal` | "请输入手机号", used to tell that a human has to log in |

### The entry chain overrides by name instead of adding a node

`assets/tasks/CloudBrowserOpenGame.json` uses the base node `OpenGame` as its `entry`; the
browser-specific chain is implemented by **overriding `OpenGame.next` under the same name** in
`resource_cloud_browser/pipeline/CloudBrowser/OpenGame.json`.

Two reasons:

1. **A task `entry` must exist in the base `resource`.** The `import` list in `interface.json` is
   shared by every controller, so an entry node that only lives in one overlay cannot be resolved
   elsewhere and `maa-tools check` reports "unknown task / unknown entry task". `AndroidOpenGame`
   follows the same shape: `entry: "OpenGame"` plus an option override.
2. **The base `OpenGame` chain cannot be used as is.** Its `next` ends with `StartUpGame`, which has
   no `recognition`, always hits, and tries to launch a process — there is no process to launch here.

The reverse is also wrong: putting the portal nodes into the base `OpenGame` would make the portal's
"开始游戏" compete with `CloudEndfieldStartGame` (ROI `[887, 522, 393, 198]`, the Android cloud
client) inside one `next` list.

Once inside the game, the title screen still reuses the desktop `EnterGame`.

## Tests

`tests/CloudBrowser/` plus `tests/MaaEndTestset/CloudBrowser/Official_CN/`; the controller matrix
is registered in `controllerMap` inside `tests/scripts/loader.mts`. Coverage:

- Portal before login (login modal and 开始游戏 present together), the same modal as captured on the
  embedded virtual display (case 4, `云浏览器_门户_内嵌登录弹窗.png`, whose layout sits further
  up-left than the desktop), only 开始游戏 after the modal closes, and the reconnect toast after the
  session is taken over;
- The cloud-streamed title page is recognized by the desktop `EnterGame` / `EnterGameOCR` /
  `EnterGameRecognition`, while the boot logo frame hits none of them — i.e. H264 compression and
  the cloud-side overlays (12+ CADPA badge, ICP filing number, build string, latency badge) do not
  affect in-game recognition.

How it runs locally (measured 2026-10-24): `npm test` first lets maa-tools fetch MaaFramework
(`preparing maafw` → download-scripts → download-binary → move-folders). Those steps print only
step names, no progress, so the first two or three minutes look like a hang. There is no `node` on
this machine's shell PATH either; use the Node bundled with Qoder:
`ELECTRON_RUN_AS_NODE=1 "<Qoder CN.exe>" node_modules/@nekosu/maa-tools/bin/maa-tools test`.
All 1438 cases finished in about 18 minutes with exit code 0, `tests/maatools/error_details.json`
was not created, and the new embedded frame (`云浏览器_门户_内嵌登录弹窗.png`) passed along with them.

## Verification status

Verified on real Windows on 2026-10-23 for the external-browser path: Snapdragon 8cx Gen 2 /
Adreno 685 tablet, 2560×1600 at 200% scaling, MaaFramework v5.14.3 (Windows ARM64).
`agent/go-service --pretask CloudBrowserLaunch` exited 0, Edge opened the portal with `--app` and
the client area was exactly 1280×720, the Win32 controller attached and OCR hit the portal texts,
so the first three hard constraints behave as designed; the fourth one (the window staying on top
during actions) was in fact violated during the end-to-end run described next.
The production chain was also run end to end (`OpenGame` + `resource_cloud_browser`):
`reco hit [result.name=CloudBrowserPortalStartGame]` → `Click` → `Node.Action.Succeeded` →
`### All tasks have been completed ###`. That particular click landed on a screen point covered by
another user window (see "FramePool fixed recognition, but input still needs the window on top"
below), so it proves the chain runs, not that the cloud actually started queueing.

Also on 2026-10-23, the portal layer of the MaaFwApp embedded browser was verified on a real phone:
the APK was built locally and installed, `CloudBrowserPC` started on the virtual display, hit
"开始游戏", and the app's own injected mouse click opened the Hypergryph login modal (it could not
before gap 8 was fixed). See "First embedded device run" for the details.

`CloudBrowserMobile` still has no device data at all. The offline matrix (`maa-tools test`, 1438
cases) went fully green locally on 2026-10-24, including the embedded login-modal frame.
Remaining unverified items: whether the
desktop `EnterGame` nodes hit the real streamed picture once logged in, whether the page title still
contains "云·终末地" after entering the game (`window_regex`), and recovery branches for queueing /
exhausted playtime / reconnection (not implemented).

### Choosing `screencap`

Running `speed_test` plus a full-frame OCR against the same window at the same geometry, with a
topmost window covering the left half of the portal as a control:

| `screencap` | method bit | connects | per frame | normal foreground | covered by a topmost window |
| --- | --- | --- | --- | --- | --- |
| `FramePool` | 2 | yes | 30-33ms | full page | **still the full page**, and it does not touch z-order |
| `ScreenDC` | 32 | yes | 24-28ms | full page | **captures the occluding window**, the portal's key controls are gone |
| `PrintWindow` | 16 | yes | ~253ms | full page | not measured |
| `GDI` | 1 | yes | ~7ms | pure black frames | - |
| `DXGI_DesktopDup` | 4 | **no** | - | `DuplicateOutput` returns `E_ACCESSDENIED` (Adreno 685) | - |
| `DXGI_DesktopDup_Window` | 8 | **no** | - | same as above | - |

The default is therefore `FramePool`. `ScreenDC` reads screen pixels, so MaaFramework has to raise
the target window before every capture (the `Failed to ensure foreground window before screencap`
log line only means the foreground activation was refused - the z-order change already happened),
which steals z-order from the user and still loses to any topmost window. `GDI` returns black
frames on Chromium, and the `DXGI_*` methods are refused by some integrated GPUs. `Foreground` and
`Background` are not ProjectInterface values; they silently fall back to `DXGI_DesktopDup`, so do
not copy MaaFramework's internal enum names.

### ROIs must be calibrated against the captured frame

In `--app` mode Chromium draws its caption *inside* the client area, so within the 1280×720 capture
the top ~31px is the caption, the right 16px is the page scrollbar and the bottom 8px is padding -
the CSS viewport is only 1264×681. The portal's "开始游戏" is pinned to the bottom-right corner and
the login modal is vertically centered, so **their y coordinates move with the window height**; a ROI
frozen at one measurement is simply wrong on another machine. The two ROIs in `Portal.json` now
cover the quadrant they live in and were measured to hit: "开始游戏" text (1124,678)-(1197,702),
"请输入手机号" (481,276)-(580,295) with score 0.9999.

Another trap: once the page has loaded, `SetWindowPos` can still change the width but the height
snaps back to 1622px because of the page's min-content height. The 1280×720 client area can only be
guaranteed by `--window-size` at launch; it cannot be repaired afterwards.

### "Everything captures as white" is usually not the capture method

When Modern Standby suspends the session or turns the display off, `document.visibilityState`
becomes `hidden` and Chromium stops painting; every window capture method then returns white frames
and it looks like the capture method is broken. Check whether the desktop clock is still advancing
first, then read `visibilityState` over CDP (`--remote-debugging-port`). For unattended runs call
`SetThreadExecutionState(ES_CONTINUOUS | ES_DISPLAY_REQUIRED | ES_SYSTEM_REQUIRED)`.

### FramePool fixed recognition, but input still needs the window on top

`Seize` mouse and keyboard are `SendInput` with **absolute screen coordinates**, not messages posted
to the HWND: in `SeizeInput::touch_down [x=1179][y=688][point.x=1187][point.y=688][hwnd_=true]`
the `point` is a screen point, so whichever window is topmost at that pixel receives the click.
MaaFramework only tries to take the foreground in two places (those are the only matching strings
left in `MaaWin32ControlUnit.dll`): before a `ScreenDCScreencap` capture, and before
`InterceptionInput` keyboard injection. The ProjectInterface `win32` block has just
`class_regex` / `window_regex` / `screencap` / `mouse` / `keyboard` - the `win32_config.*` strings in
`MaaPiCli.exe` confirm there is no topmost/foreground option at all.

Switching to `FramePool` therefore produces a counter-intuitive combination: captures no longer need
the foreground and succeed even while covered, `Node.Action.Succeeded` still reports green, but the
click goes into the window that is covering the portal. With `ScreenDC` the incidental z-order raise
on every capture hid this. The `CloudBrowserLaunch` pretask raises the window exactly once, at
startup (`ShowWindow(swRestore)` + `SetForegroundWindow`); nothing raises it again during the task.

Consequence: **the portal window must stay on top and unobstructed while a task runs.** If that has
to be guaranteed automatically, it has to be done by the caller (GUI or pretask) before actions.
Making the window `HWND_TOPMOST` is the most reliable but covers everything the user owns, so it is
not a sensible default.

### The portal page false-hits the desktop title-screen nodes

During the real end-to-end run, `EnterGameResourceInitializeCompleteText` hit the login modal's
"已同意《用户注册协议》..." text (box `[465,403,368,17]`), and `EnterGameRecognition` template matching
produced a 0.427 candidate on the 12+ CADPA badge (box `[1183,29,31,32]`, below threshold, hence a
miss). Neither of them took over only because their enclosing `And` did not hold - the ordering of
this `next` list is fragile. Re-check it on the portal page before touching `OpenGame.next` or
lowering those thresholds.

### Notes on manual verification with stock MaaPiCli

MaaEnd's production `interface.json` cannot be loaded by `MaaPiCli.exe` of v5.14.3:
`Parser::parse_import_data json is not a valid ImportData [error_key=option]`, caused by the
`"type": "hotkey"` + `hotkeys` custom option in `assets/tasks/setting/Keymap.json`, and finally
`Failed to parse interface.json`. For manual verification, use a minimal interface without `import`
that mounts `["./resource", "./resource_cloud_browser"]` and the single
`CloudBrowserOpenGame` task (`entry: "OpenGame"`); it reproduces the same chain.

