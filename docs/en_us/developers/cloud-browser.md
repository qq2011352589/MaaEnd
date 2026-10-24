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
(`CloudBrowserActivity.kt`), but **the WebView's `devicePixelRatio` follows the display**: measured
dpr 1.5 and an 854×480 CSS viewport (see gap 2). The page scales its own content into that CSS space
and lands back on 1280×720 physical pixels, so **the desktop portal ROIs are reused in physical
pixels** — do not divide by the dpr, and do not describe the viewport as 1280×720 CSS. Note that the
virtual display itself still reports the phone's
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

The embedded path is verified on a real device all the way to the cloud title screen (see
"Embedded device runs"): `CloudBrowserPC` attaches to the virtual display, `CloudBrowserPortalStartGame`
hits, a mouse click injected by the app itself opens the Hypergryph login modal, and once signed in
the portal's "开始游戏" → boot overlay → 1280×720 stream → `EnterGame` chain ends in a successful
task. The mobile controller (`CloudBrowserMobile`) is still untested.

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
2. **The coordinate basis differs from the external browser (measured; nodes hit).** The external path
   captures the `--app` client area, where the captured 1280×720 frame carries about 31px of title
   bar on top and a 16px scrollbar on the right, leaving a 1264×681 CSS viewport. The embedded
   Activity pins `config.densityDpi` to 160 in `attachBaseContext`, but **the WebView's
   `devicePixelRatio` follows the display, not the Activity configuration**: measured on 2026-10-24,
   `window.devicePixelRatio = 1.5` and `innerWidth/Height = 854×480`, i.e. the 1280×720 virtual
   display spans 854×480 CSS pixels. The portal lays its key areas out in that CSS space (the title
   page's `<video>` box is `854 / 0.6667 ≈ 1281` CSS wide and then `transform: scale(0.666667)` brings
   it back to 854 CSS; ×1.5 dpr lands on exactly 1280 physical pixels), so **captures are still
   1280×720 and ROIs are still written in physical pixels** — do not divide by the dpr.
   The responsive layout does change, though: on the 2026-10-23 embedded run the "开始游戏" text box
   measured (1045,660)-(1153,692) — inside the existing ROI — while the login modal's "请输入手机号"
   measured (402,211)-(549,239): the old ROI's left edge at 420 cut into the first glyph and OCR then
   found no line at all. `CloudBrowserPortalLoginModal`'s ROI is now `[380,180,520,180]`, wide enough
   for both the desktop and the embedded geometry.
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
6. **The `<video>` geometry (now calibrated).** The streamed picture is a `<video>` element inside the
   portal page, while the virtual display captures the whole 1280×720 page. Measured on the
   embedded device on 2026-10-24: `getBoundingClientRect()` is (0,0)-(854,480) CSS,
   `videoWidth×videoHeight = 1280×720`, `object-fit: fill`, the wrapper carries
   `transform: scale(0.666667)`, and dpr is 1.5 — which works out to 1281×720 physical pixels, so the
   frame is **full-bleed with no letterbox**. A streamed title-page capture is structurally identical
   to the desktop 720p one: `EnterGameOCR`'s ROI (x ≥ W-250, y 200–600) covers "点击任意位置继续"
   at about (1113,371)-(1250,390), and `EnterGameRecognition`'s announcement/settings/repair buttons
   sit inside (1067,0,213,276), so the 720p ROIs from `resource_adb` apply unchanged. The only
   difference is video compression: text has slight ghosting, yet the new embedded real frame added
   to the offline matrix on 2026-10-24 (the one taken after the fade-in finished) still hits all
   three template nodes. **The fade-in itself does not count as a hit**: for a second or two after
   the first frame the picture still carries scrolling yellow blocks and fading icons. Measured on
   that same session, the fade-in frame gave `EnterGameOCR` 0.9999 but
   `EnterGameRecognition` only 0.39, so the `And` in `EnterGame` did not hold — that is "waiting for
   a stable frame", not "recognition is broken", and it is why the NextList may idle for a round or
   two after the overlay disappears.
7. **Orphan nodes and missing branches in the portal layer.** Nothing references
   `CloudBrowserPortalLoggedIn`, neither a node nor a `next`; queueing, exhausted playtime and
   reconnecting are recognized but have no recovery branch. The boot-overlay branch now exists:
   `CloudBrowserGameStarting` OCRs "游戏启动中" over the full frame, does `DoNothing`, sets
   `rate_limit 2000`, and is used as a waiting state with `[JumpBack]` in `OpenGame.next` (see the
   second round under "Embedded device runs"). The i18n part is not a gap but is worth
   recording: `tools/i18n/sync_ocr_expected.py` scans only the `resource`, `resource_adb` and
   `resource_cloud_adb` pipelines, so neither portal overlay is in scope (portal wording does not
   follow the client language), and every portal node carries `@i18n-skip` so that adding the
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
10. **Starting the task again tears down a live stream (now fixed).** `CloudBrowserActivity` is
    `singleTask` and the privileged process launches it once per run; the old `onNewIntent` called
    `loadPortal()` unconditionally. So while the stream was already on the title screen, tapping
    "开始任务" again reloaded the page back to the portal — and because the page registers
    `beforeunload`, it raised a native "确认离开" dialog. That dialog covers the virtual display,
    nothing matches, and the task spins until it fails. That is exactly what the device showed on
    2026-10-24 (task started 08:07:32, failed 08:07:51). `loadPortal()` now compares `mode` and
    `url` and only calls `requestFocus()` when both are unchanged.
11. **A 1–2 second gap between the "开始游戏" click and the overlay, which invited a double click
    (now mitigated).** In one run `CloudBrowserPortalStartGame` hit at 07:55:35.7 and again at 39.5;
    the server received two `session/dispatch` calls 1.4 s apart and the first session's `<video>`
    track then `ended` outright. The node now carries `post_delay: 2000` to suppress the repeat; once
    the overlay is up, `CloudBrowserGameStarting` takes over and nothing clicks again.

### Embedded device runs

#### Round one (2026-10-23): portal layer

On a real phone, with the APK built locally and installed. The portal layer works:

- **Activity and virtual display.** `CloudBrowserActivity: mode=pc` started on the virtual display
  and mirroring worked. The virtual display id changes every run (observed 4 → 6 → 7 → 8 → 9 → 10);
  never hard-code it in a script.
- **Portal recognition.** `CloudBrowserPortalStartGame` hit (1045,660,108,32); after the ROI
  widening, `CloudBrowserPortalLoginModal` hit (402,211)-(549,239). That frame is now case 2 of
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

#### Round two (2026-10-24): login → boot overlay → cloud title screen

This round closed gap 6 and fixed the three problems it hit (gaps 10 and 11 plus the missing
waiting node):

- **Login.** Signing in to the cloud instance only works in `CloudBrowserLoginActivity` on the
  primary display (the virtual display has no IME, and the virtual display's page steals WebView
  input focus). A bound phone number plus an SMS code; afterwards the portal shows the passport UID,
  "畅玩卡剩余 23天16小时" and "免费时长 565分钟". The account and the code are deliberately not
  recorded here. The cookies land in the app's private `app_webview/` directory, and the virtual
  display is authenticated from then on. **Never commit those cookies to any repository.**
- **Click → boot overlay.** In the 07:55 run the injected click triggered `session/dispatch`, but the
  server received two dispatches within 1.4 s (gap 11) and the first session's `<video>` track then
  `ended` (a `[reconnectServer], isOnceStartError` line in the console); the page stayed on
  "游戏启动中，请耐心等待". `OpenGame.next` had no waiting node, so the whole task failed 20 s later
  (`Tasker.Task.Failed` at 07:56:17). The console showed nothing new for the next two minutes — it
  was pure idling.
- **Boot overlay → title screen.** In a fresh run (reload the page, click "开始游戏"; `/session/dispatch`
  and `/user/exchange` both returned 200) the `<video>` produced frames after about 20 s:
  `videoWidth×videoHeight = 1280×720`, the track `live`, and the title screen
  ("明日方舟 终末地 / 点击任意位置继续 / 公告·设置·修复") full-bleed across 1280×720. The frame taken
  after the fade-in finished is stored as `云浏览器_云流_内嵌标题页.png`, and `EnterGame` /
  `EnterGameOCR` / `EnterGameRecognition` all hit it (offline matrix under "Tests").
- **Stream geometry.** See the measured numbers in gap 6: 854×480 CSS × dpr 1.5, video box 1281×720
  CSS, `object-fit: fill`, 1280×720 physical with no letterbox. The 720p ROIs of `resource_adb`
  need no recalibration.
- **End of the chain (the 08:28 run, after installing the new build).** The Activity started on
  virtual display 10; the portal noticed the account already had a session and auto-dispatched back
  into it; `CloudBrowserGameStarting` hit every ~2 s from 08:28:26 on (`DoNothing`); at 08:28:48 the
  overlay went away and the next NextList round missed once; 08:28:50 `EnterGame` hit and 08:28:51
  ended with `Tasker.Task.Succeeded`. `CloudBrowserOpenGame` means "stop at the title screen", so the
  task ends there by design.
- **Still unverified.** `CloudBrowserMobile` end to end (gap 5); reconnection and queueing branches;
  drag-while-held semantics (see "Ordering for the remaining runs").

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

Round one covered the Activity starting on the virtual display, the portal nodes hitting, mouse
injection and the ROI check; round two went through login, the boot overlay, the streamed title
screen and gap 6. What is left, in order:

1. Switch to `CloudBrowserMobile`: first prove touch injection works and that full-frame OCR hits
   the mobile portal layout, then add the `OpenGame.next` override (gap 5).
2. Queueing / exhausted playtime / reconnection recovery (gap 7) — has to be written before it can
   be run.
3. Drag semantics while the button is held (`ACTION_MOVE` carrying `buttonState`) are still untested;
   verifying them needs a draggable control on the page.
4. Whether the page title still contains "云·终末地" after entering the game (`window_regex`): the
   task ends at the title screen, so this has not been observed.

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
  embedded virtual display (case 2, `云浏览器_门户_内嵌登录弹窗.png`, whose layout sits further
  up-left than the desktop), only 开始游戏 after the modal closes, the reconnect toast after the
  session is taken over, and **the boot overlay that appears after the embedded click**
  (`云浏览器_门户_内嵌启动中.png`, where only "游戏启动中" is recognizable);
- The cloud-streamed title page is recognized by the desktop `EnterGame` / `EnterGameOCR` /
  `EnterGameRecognition` (`云浏览器_云流_内嵌标题页.png` is a real device stream frame, not a desktop
  screenshot), while the boot logo frame hits none of them — i.e. H264 compression and the cloud-side
  overlays (12+ CADPA badge, ICP filing number, build string, latency badge) do not affect in-game
  recognition.

How it runs locally (measured 2026-10-24): `npm test` first lets maa-tools fetch MaaFramework
(`preparing maafw` → download-scripts → download-binary → move-folders). Those steps print only
step names, no progress, so the first two or three minutes look like a hang. There is no `node` on
this machine's shell PATH either; use the Node bundled with Qoder:
`ELECTRON_RUN_AS_NODE=1 "<Qoder CN.exe>" node_modules/@nekosu/maa-tools/bin/maa-tools test`.
All 1449 cases finished in about 18 minutes with exit code 0, `tests/maatools/error_details.json`
was not created, and the new embedded frames (`云浏览器_门户_内嵌登录弹窗.png`,
`云浏览器_门户_内嵌启动中.png`, `云浏览器_云流_内嵌标题页.png`) passed along with them.

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
before gap 8 was fixed). On 2026-10-24 the chain continued to the cloud title screen after signing
in: injected click → boot overlay → 1280×720 stream → `EnterGame` hit → `Tasker.Task.Succeeded`.
See "Embedded device runs" for both rounds.

`CloudBrowserMobile` still has no device data at all. The offline matrix went fully green locally on
2026-10-24, including the embedded login-modal, boot-overlay and streamed-title-screen frames.
Remaining unverified items: recovery branches for queueing / exhausted playtime / reconnection (not
implemented), the injection semantics of dragging while held, and whether the page title still
contains "云·终末地" after entering the game (`window_regex` — this round ended at the title screen
and never went further).

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
