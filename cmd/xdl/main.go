package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/ghostlawless/xdl/internal/app"
)

func genRunID() string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "run"
	}
	return hex.EncodeToString(b[:])
}

func hasDebugFlag(args []string) bool {
	for _, a := range args {
		if a == "-d" || a == "/d" {
			return true
		}
	}
	return false
}

func main() {
	runID := genRunID()
	if hasDebugFlag(os.Args[1:]) {
		fmt.Println("xD run_id:", runID)
	}

	defer func() {
		if r := recover(); r != nil {
			_, _ = fmt.Fprintln(os.Stderr, "fatal:", r)
			if st := debug.Stack(); len(st) > 0 {
				_, _ = os.Stderr.Write(st)
			}
			os.Exit(1)
		}
	}()

	if err := app.RunWithArgsAndID(os.Args[1:], runID); err != nil {
		os.Exit(1)
	}
}
