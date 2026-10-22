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
Activity pins its density to 160 dpi so the 1280×720 virtual display maps to 1280×720 CSS
pixels and the desktop portal ROIs still apply; the mobile portal layout differs, so
`resource_cloud_browser_mobile` switches the "开始游戏" button and login form back to full-frame
OCR.

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

The embedded path has not been verified on a real device yet. For the first run, sign in from
Settings, pick `CloudBrowserPC` and run `CloudBrowserOpenGame`, then check the log for controller
connection to the virtual display, portal OCR hits and whether injected mouse clicks move the
page into the queue; then switch to `CloudBrowserMobile` to verify touch injection and title
recognition against `resource_adb`.

## Three hard constraints

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

- Portal before login (login modal and 开始游戏 present together), only 开始游戏 after the modal
  closes, and the reconnect toast after the session is taken over;
- The cloud-streamed title page is recognized by the desktop `EnterGame` / `EnterGameOCR` /
  `EnterGameRecognition`, while the boot logo frame hits none of them — i.e. H264 compression and
  the cloud-side overlays (12+ CADPA badge, ICP filing number, build string, latency badge) do not
  affect in-game recognition.

## Verification status

> [!IMPORTANT]
>
> The conclusions above come from Playwright probing on Linux Chromium and from node tests. They
> have **not been run on real Windows with the MaaFramework Win32 controller**. For the first
> on-device verification, check these in order:
>
> 1. Whether `ScreenDC` can capture a Chromium window. Chromium is GPU-composited and GDI BitBlt
>    may capture black frames; if every recognition fails, switch `screencap` in `interface.json`
>    to `FramePool` or `DXGI_DesktopDup_Window`.
> 2. Whether `Seize` mouse and keyboard input actually reach the page (WebRTC input and the mouse
>    look controls need real events).
> 3. Whether the page title still contains "云·终末地" after entering the game, i.e. whether
>    `window_regex` still matches.
> 4. Recovery branches for queueing, exhausted playtime and reconnection are not implemented yet.
