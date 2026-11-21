//go:build !windows

package main

import "fmt"

func main() {
	fmt.Println("overlay-webview2: not supported on this OS (Windows only)")
}
