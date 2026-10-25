package brand

import _ "embed"

//go:embed tray.png
var trayPNG []byte

// TrayIcon returns the app's menu-bar icon. It is application branding, not
// part of a replaceable pet skin.
func TrayIcon() []byte { return trayPNG }
