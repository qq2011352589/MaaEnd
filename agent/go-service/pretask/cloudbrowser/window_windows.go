//go:build windows

package cloudbrowser

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/rs/zerolog/log"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	user32                            = windows.NewLazySystemDLL("user32.dll")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetWindowRect                 = user32.NewProc("GetWindowRect")
	procGetClientRect                 = user32.NewProc("GetClientRect")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procShowWindow                    = user32.NewProc("ShowWindow")
)

const (
	swpNoMove   = 0x0002
	swpNoZOrder = 0x0004
	swRestore   = 0x0009
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2
	dpiPerMonitorAwareV2 = ^uintptr(3)
)

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

func init() {
	// Go 默认不带 application manifest，进程被当作 DPI-unaware，此时 GetWindowRect /
	// SetWindowPos 拿到的是 DWM 虚拟化后的坐标，按它设窗口会得到 1600x900 之类的物理尺寸。
	procSetProcessDpiAwarenessContext.Call(dpiPerMonitorAwareV2)
}

// registryBrowser 读 Chrome/Edge 在 App Paths 下注册的绝对路径。
func registryBrowser() (string, bool) {
	for _, exe := range []string{"chrome.exe", "msedge.exe"} {
		key := `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\` + exe
		for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
			k, err := registry.OpenKey(root, key, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			path, _, err := k.GetStringValue("")
			k.Close()
			if err != nil {
				continue
			}
			path = strings.Trim(path, `"`)
			if isFile(path) {
				return path, true
			}
		}
	}
	return "", false
}

// fitClientArea 把窗口外框调到「客户区恰好 w x h 物理像素」。
// 先量一次当前外框与客户区之差作为边框开销，再按这个差值反推外框尺寸。
func fitClientArea(hWnd unsafe.Pointer, w, h int) {
	outer, ok := getWindowRect(hWnd)
	if !ok {
		return
	}
	client, ok := getClientRect(hWnd)
	if !ok {
		return
	}
	frameW := int(outer.Right-outer.Left) - int(client.Right-client.Left)
	frameH := int(outer.Bottom-outer.Top) - int(client.Bottom-client.Top)
	targetW := w + frameW
	targetH := h + frameH
	if int(client.Right-client.Left) == w && int(client.Bottom-client.Top) == h {
		return
	}
	ret, _, err := procSetWindowPos.Call(
		uintptr(hWnd), 0, 0, 0, uintptr(targetW), uintptr(targetH), swpNoMove|swpNoZOrder)
	if ret == 0 {
		log.Error().
			Err(err).
			Str("component", component).
			Int("width", targetW).
			Int("height", targetH).
			Msg("failed to resize window")
	}
}

func getWindowRect(hWnd unsafe.Pointer) (winRect, bool) {
	var r winRect
	if ret, _, _ := procGetWindowRect.Call(uintptr(hWnd), uintptr(unsafe.Pointer(&r))); ret == 0 {
		return r, false
	}
	return r, true
}

func getClientRect(hWnd unsafe.Pointer) (winRect, bool) {
	var r winRect
	if ret, _, _ := procGetClientRect.Call(uintptr(hWnd), uintptr(unsafe.Pointer(&r))); ret == 0 {
		return r, false
	}
	return r, true
}

// foreground 把窗口带到最前。ScreenDC 截的是屏幕区域，被遮挡时识别必然失败。
func foreground(hWnd unsafe.Pointer) error {
	procShowWindow.Call(uintptr(hWnd), swRestore)
	if ret, _, err := procSetForegroundWindow.Call(uintptr(hWnd)); ret == 0 {
		return fmt.Errorf("SetForegroundWindow failed: %w", err)
	}
	return nil
}
