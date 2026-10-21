// Package cloudbrowser 是 CloudBrowser 控制器的 pretask：在 MaaFramework 连接窗口之前，
// 把云·终末地网页门户拉成一个客户区恰好 1280x720 的独立浏览器窗口。
//
// 之所以必须钉死 1280x720：仓库所有 ROI 与模板以 1280x720 为基准，而桌面控制器截到多大就是
// 多大，不会像 ADB 那样按设备分辨率缩放基准。
package cloudbrowser

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/MaaXYZ/MaaEnd/agent/go-service/pkg/fsutil"
	"github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/bytedance/sonic"
	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	component = "cloudbrowser"

	portalURL = "https://endfield.hypergryph.com/cloud"

	// clientWidth / clientHeight 即 pipeline 的 720p 基准，改动会让所有 ROI 失效。
	clientWidth  = 1280
	clientHeight = 720

	windowTimeout = 60 * time.Second
	pollInterval  = 500 * time.Millisecond
)

// 与 interface.json 中 CloudBrowser 的窗口匹配规则保持一致。
var (
	windowClassRegex = regexp.MustCompile(`Chrome_WidgetWin_1`)
	windowTitleRegex = regexp.MustCompile(`云·终末地`)
)

// proxyBypassDomains 是必须直连的云游戏自身域名。蔚领的媒体流走 UDP，HTTP 代理承载不了，
// 被系统代理劫持时表现为「测速失败(6544)」而不是明确报错，故无条件放行。
var proxyBypassDomains = []string{
	"*.vlinkcloud.cn",
	"*.hypergryph.com",
	"*.hycdn.cn",
	"*.skland.com",
}

type options struct {
	// BrowserExe 为用户手动指定的浏览器可执行文件，空则自动查找。
	BrowserExe string `json:"BrowserExe"`
}

// Run 对应 assets/tasks/pretasks/CloudBrowserLaunch.json。
// 只负责把窗口准备好，页面内的流程全部交给 pipeline。
func Run(args []string) bool {
	opts := parseOptions(args)

	exe, err := findBrowser(opts.BrowserExe)
	if err != nil {
		log.Error().
			Err(err).
			Str("component", component).
			Msg("no usable browser found")
		return false
	}

	profile, err := profileDir()
	if err != nil {
		log.Error().
			Err(err).
			Str("component", component).
			Msg("failed to resolve browser profile dir")
		return false
	}
	if err := os.MkdirAll(profile, 0o755); err != nil {
		log.Error().
			Err(err).
			Str("dir", profile).
			Str("component", component).
			Msg("failed to create browser profile dir")
		return false
	}

	// 云实例是单会话：第二个客户端会把第一个挤回标题页，表现为任务莫名卡在门户空转。
	if running, pid := runningInstance(profile); running {
		log.Info().
			Str("component", component).
			Int("pid", pid).
			Msg("reusing the browser instance already opened on this profile")
	} else if err := launch(exe, profile); err != nil {
		log.Error().
			Err(err).
			Str("component", component).
			Str("exe", exe).
			Msg("failed to launch browser")
		return false
	}

	if err := initMaa(); err != nil {
		log.Error().
			Err(err).
			Str("component", component).
			Msg("failed to init MaaFramework toolkit")
		return false
	}

	hWnd := waitWindow(windowTimeout)
	if hWnd == nil {
		log.Error().
			Str("component", component).
			Str("class_regex", windowClassRegex.String()).
			Str("title_regex", windowTitleRegex.String()).
			Msg("portal window not found")
		return false
	}

	fitClientArea(hWnd, clientWidth, clientHeight)
	if err := foreground(hWnd); err != nil {
		log.Warn().
			Err(err).
			Str("component", component).
			Msg("failed to bring window to foreground")
	}

	log.Info().
		Str("component", component).
		Int("width", clientWidth).
		Int("height", clientHeight).
		Msg("cloud browser window ready")
	return true
}

func parseOptions(args []string) options {
	opts := options{}
	if len(args) == 0 {
		return opts
	}
	raw := strings.TrimSpace(args[len(args)-1])
	if !strings.HasPrefix(raw, "{") {
		log.Warn().
			Str("component", component).
			Str("arg", raw).
			Msg("last arg is not options JSON, using defaults")
		return opts
	}
	if err := json.Unmarshal([]byte(raw), &opts); err != nil {
		log.Warn().
			Err(err).
			Str("component", component).
			Msg("failed to parse options, using defaults")
	}
	return opts
}

// profileDir 是浏览器用户数据目录，登录态存在里面。放在用户配置目录而非安装目录：
// 重装 MaaEnd 不该把账号一起清掉，也不该有被版本库收下的风险。
func profileDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "MaaEnd", "cloud_browser"), nil
}

// browserCommands 是 PATH 里常见的浏览器可执行文件名，按优先级排列。
func browserCommands() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"chrome.exe", "msedge.exe", "chromium.exe"}
	case "darwin":
		return []string{"google-chrome", "Google Chrome"}
	default:
		return []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "microsoft-edge"}
	}
}

// findBrowser 定位浏览器：手动路径优先，其次注册表，最后 PATH 与常见安装目录。
func findBrowser(manual string) (string, error) {
	if manual != "" {
		path := strings.Trim(manual, `"`)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
		return "", fmt.Errorf("browser path %q is not a file", path)
	}

	if path, ok := registryBrowser(); ok {
		return path, nil
	}

	for _, name := range browserCommands() {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
		if path, ok := commonInstallPath(name); ok {
			return path, nil
		}
	}
	return "", errors.New("no browser found; fill in the browser path in the CloudBrowser options")
}

// commonInstallPath 查各平台浏览器的固定安装位置。Windows 上浏览器不进 PATH，这一步是必需的。
func commonInstallPath(name string) (string, bool) {
	switch runtime.GOOS {
	case "windows":
		for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
			if root == "" {
				continue
			}
			for _, vendor := range []string{`Google\Chrome\Application`, `Microsoft\Edge\Application`, ""} {
				path := filepath.Join(root, filepath.FromSlash(vendor), name)
				if isFile(path) {
					return path, true
				}
			}
		}
	case "darwin":
		app := filepath.Join("/Applications", strings.TrimSuffix(name, ".exe")+".app", "Contents", "MacOS", name)
		if isFile(app) {
			return app, true
		}
	}
	return "", false
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// runningInstance 判断该用户数据目录上是否已有浏览器进程。
// Chromium 会把 --user-data-dir 透传给所有子进程，命中任意一个即说明实例已存在。
func runningInstance(profile string) (bool, int) {
	needle := "--user-data-dir=" + filepath.Clean(profile)
	procs, err := process.Processes()
	if err != nil {
		log.Warn().
			Err(err).
			Str("component", component).
			Msg("failed to enumerate processes")
		return false, 0
	}
	for _, p := range procs {
		cmdline, err := p.Cmdline()
		if err != nil || !strings.Contains(cmdline, needle) {
			continue
		}
		return true, int(p.Pid)
	}
	return false, 0
}

func launch(exe, profile string) error {
	cmd := exec.Command(exe, launchArgs(profile)...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	// pretask 退出后窗口要留着，不等它也不回收。
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Debug().
				Err(err).
				Str("component", component).
				Msg("browser process exited")
		}
	}()
	return nil
}

func launchArgs(profile string) []string {
	return []string{
		"--app=" + portalURL,
		"--user-data-dir=" + filepath.Clean(profile),
		// 不锁 1:1 的话，系统缩放 125% 时页面会渲染成 1600x900 物理像素，720p 基准全废。
		"--force-device-scale-factor=1",
		fmt.Sprintf("--window-size=%d,%d", clientWidth, clientHeight),
		"--window-position=0,0",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-features=Translate",
		"--proxy-bypass-list=" + strings.Join(proxyBypassDomains, ";"),
	}
}

// initMaa 初始化 MaaFramework 以便使用桌面窗口枚举 toolkit。
func initMaa() error {
	libDir := os.Getenv("MAAFW_BINARY_PATH")
	if libDir == "" {
		libDir = fsutil.OutputPath("maafw")
	}
	if err := maa.Init(
		maa.WithLibDir(libDir),
		maa.WithJSONEncoder(sonic.Marshal),
		maa.WithJSONDecoder(sonic.Unmarshal),
	); err != nil {
		return err
	}
	if err := maa.ConfigInitOption(fsutil.OutputPath(), "{}"); err != nil {
		log.Warn().
			Err(err).
			Str("component", component).
			Msg("failed to init toolkit config option")
	}
	return nil
}

func findWindow() unsafe.Pointer {
	windows, err := maa.FindDesktopWindows()
	if err != nil {
		log.Warn().
			Err(err).
			Str("component", component).
			Msg("failed to enumerate desktop windows")
		return nil
	}
	for _, w := range windows {
		if windowClassRegex.MatchString(w.ClassName) && windowTitleRegex.MatchString(w.WindowName) {
			return w.Handle
		}
	}
	return nil
}

// waitWindow 轮询等待门户窗口出现。页面标题在进游戏后会变化，故只等首次出现。
func waitWindow(timeout time.Duration) unsafe.Pointer {
	deadline := time.Now().Add(timeout)
	for {
		if hWnd := findWindow(); hWnd != nil {
			return hWnd
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(pollInterval)
	}
}
