//go:build server

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "stepthrough: desktop UI not available in server build (compiled without Wails)")
	os.Exit(1)
}
