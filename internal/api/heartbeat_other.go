//go:build !linux

package api

import "runtime"

// CollectSystemInfo returns stub values on non-Linux platforms
// (used for development and testing on macOS/Windows).
func CollectSystemInfo() (osName, arch, kernel string) {
	return "Development", runtime.GOARCH, "unknown"
}

// CollectSystemMetrics returns zero metrics on non-Linux platforms.
func CollectSystemMetrics() SystemMetrics {
	return SystemMetrics{}
}
