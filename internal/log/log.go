// file: internal/log/log.go
package log

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu      sync.RWMutex
	logger  *log.Logger
	enabled = true
	outFile io.Closer
)

func Init(path string) {
	mu.Lock()
	defer mu.Unlock()

	abs := path
	if !filepath.IsAbs(abs) {
		exe, _ := os.Executable()
		base := filepath.Dir(exe)
		abs = filepath.Join(base, path)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		logger = log.New(os.Stderr, "", 0)
		_ = logger.Output(2, "log init fallback stderr: "+err.Error())
		return
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logger = log.New(os.Stderr, "", 0)
		_ = logger.Output(2, "log open fallback stderr: "+err.Error())
		return
	}
	outFile = f
	logger = log.New(io.MultiWriter(f, os.Stderr), "", 0) // arquivo + console
	enabled = true
}

func Disable() {
	mu.Lock()
	defer mu.Unlock()
	enabled = false
}

func LogInfo(tag, msg string)  { logf("INFO", tag, msg) }
func LogDebug(tag, msg string) { logf("DEBUG", tag, msg) }
func LogError(tag, msg string) { logf("ERROR", tag, msg) }

func logf(level, tag, msg string) {
	mu.RLock()
	defer mu.RUnlock()
	if !enabled {
		return
	}
	if logger == nil {
		logger = log.New(os.Stderr, "", 0)
	}
	ts := time.Now().Format(time.RFC3339)
	_ = logger.Output(3, fmt.Sprintf("%s [%s] %s: %s", ts, level, tag, msg))
}
