//go:build !windows

package main

import "fmt"

func main() {
	fmt.Println("overlay-gio: not supported on this OS (Windows only)")
}
