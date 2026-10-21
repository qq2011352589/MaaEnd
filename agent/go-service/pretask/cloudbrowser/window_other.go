//go:build !windows

package cloudbrowser

import (
	"errors"
	"unsafe"

	"github.com/rs/zerolog/log"
)

// registryBrowser 只有 Windows 有 App Paths 注册表，其余平台靠 PATH 与常见安装目录。
func registryBrowser() (string, bool) {
	return "", false
}

// fitClientArea 在非 Windows 上无法调整窗口几何：CloudBrowser 控制器是 Win32 类型，
// 本就不会在非 Windows 上连接，这里只提示窗口尺寸由浏览器自己决定。
func fitClientArea(hWnd unsafe.Pointer, w, h int) {
	log.Warn().
		Str("component", component).
		Str("os", "non-windows").
		Msg("client area cannot be pinned, pipeline ROI calibration may not match")
}

func foreground(hWnd unsafe.Pointer) error {
	return errors.New("bringing the window to foreground is only implemented on Windows")
}
