//go:build windows

package main

import "golang.org/x/sys/windows"

// setupConsole bật UTF-8 cho console Windows để hiển thị đúng tiếng Việt & emoji.
func setupConsole() {
	_ = windows.SetConsoleOutputCP(65001)
	_ = windows.SetConsoleCP(65001)
}
