package gamesetup

import (
	"os"
	"runtime"
)

var goos = runtime.GOOS

func driveExists(root string) bool {
	_, err := os.Stat(root)
	return err == nil
}
