package log

import (
	"fmt"
	"io"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	mu  sync.RWMutex
	lg  *stdlog.Logger
	on  = true
	out io.Closer
)

func Init(path string) {
	mu.Lock()
	defer mu.Unlock()

	a := path
	if !filepath.IsAbs(a) {
		exePath, _ := os.Executable()
		baseDir := filepath.Dir(exePath)
		a = filepath.Join(baseDir, path)
	}

	if err := os.MkdirAll(filepath.Dir(a), 0o755); err != nil {
		lg = stdlog.New(os.Stderr, "", 0)
		_ = lg.Output(2, "log init fallback stderr: "+err.Error())
		return
	}

	f, err := os.OpenFile(a, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		lg = stdlog.New(os.Stderr, "", 0)
		_ = lg.Output(2, "log open fallback stderr: "+err.Error())
		return
	}

	out = f
	lg = stdlog.New(io.MultiWriter(f, os.Stderr), "", 0)
	on = true
}

func Disable() {
	mu.Lock()
	defer mu.Unlock()
	on = false
}

func LogInfo(tag, msg string) { fx("INFO", tag, msg) }

func LogDebug(tag, msg string) { fx("DEBUG", tag, msg) }

func LogError(tag, msg string) { fx("ERROR", tag, msg) }

func fx(level, tag, msg string) {
	mu.RLock()
	defer mu.RUnlock()

	if !on {
		return
	}
	if lg == nil {
		lg = stdlog.New(os.Stderr, "", 0)
	}

	prefix := "xdl>"
	if level == "ERROR" {
		prefix = "xdl!"
	}

	var line string
	if tag != "" {
		line = fmt.Sprintf("%s [%s] %s", prefix, tag, msg)
	} else {
		line = fmt.Sprintf("%s %s", prefix, msg)
	}

	_ = lg.Output(3, line)
}

func BuildRunFolderName(username, userID, runID string) string {
	base := []string{username, runID}
	basePath := strings.Join(base, "_")
	if runID == "" {
		return basePath
	}
	return fmt.Sprintln(basePath)
}

func BuildRunLogPath(baseDir, username, userID, runID string) string {
	folder := BuildRunFolderName(username, userID, runID)
	return filepath.Join(baseDir, folder, "xdl.log")
}
