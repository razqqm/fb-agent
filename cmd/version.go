package cmd

import (
	"fmt"
	"runtime"
)

// Version prints build version information.
func Version(version, buildTime string) {
	fmt.Printf("fb-agent %s\n", version)
	fmt.Printf("Build time: %s\n", buildTime)
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
