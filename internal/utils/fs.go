package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	xlog "github.com/ghostlawless/xdl/internal/log"
)

var filenameReplacer = strings.NewReplacer(
	"/", "_",
	"\\", "_",
	":", "_",
	"*", "_",
	"?", "_",
	"\"", "_",
	"<", "_",
	">", "_",
	"|", "_",
)

func EnsureDir(path string) error {
	if path == "" {
		return fmt.Errorf("empty dir")
	}
	if stat, err := os.Stat(path); err == nil && stat.IsDir() {
		return nil
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		xlog.LogError("utils.ensure_dir", err.Error())
		return err
	}
	return nil
}

func DirExists(path string) bool {
	stat, err := os.Stat(path)
	return err == nil && stat.IsDir()
}

func SanitizeFilename(name string) string {
	if name == "" {
		return "file"
	}
	name = filenameReplacer.Replace(filepath.Base(name))
	name = strings.TrimSpace(name)
	if name == "" {
		return "file"
	}
	return strings.TrimRight(name, ". ")
}

func SaveToFile(path string, data []byte) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}

	base := filepath.Base(path)
	tmpFile, err := os.CreateTemp(filepath.Dir(path), base+".tmp-*")
	if err != nil {
		xlog.LogError("utils.save_file", err.Error())
		return err
	}

	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpPath)
		xlog.LogError("utils.save_file", err.Error())
		return err
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		xlog.LogError("utils.save_file", err.Error())
		return err
	}

	if _, err := os.Stat(path); err == nil {
		_ = os.Remove(path)
	}

	if err := os.Rename(tmpPath, path); err == nil {
		return nil
	}

	in, err := os.Open(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		xlog.LogError("utils.save_file", err.Error())
		return err
	}

	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		in.Close()
		_ = os.Remove(tmpPath)
		xlog.LogError("utils.save_file", err.Error())
		return err
	}

	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		in.Close()
		_ = os.Remove(path)
		_ = os.Remove(tmpPath)
		xlog.LogError("utils.save_file", err.Error())
		return err
	}

	if err := out.Close(); err != nil {
		in.Close()
		_ = os.Remove(path)
		_ = os.Remove(tmpPath)
		xlog.LogError("utils.save_file", err.Error())
		return err
	}

	in.Close()
	_ = os.Remove(tmpPath)

	return nil
}

func SaveText(path string, content string) error {
	return SaveToFile(path, []byte(content))
}

func SaveTimestamped(dir, prefix, ext string, data []byte) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("empty baseDir")
	}
	if err := EnsureDir(dir); err != nil {
		return "", err
	}

	timestamp := time.Now().Format("20060102_150405.000000000")
	suffix := randHex(4)

	prefix = SanitizeFilename(prefix)
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" {
		ext = "bin"
	}

	name := fmt.Sprintf("%s_%s_%s.%s", prefix, timestamp, suffix, ext)
	fullPath := filepath.Join(dir, name)

	if err := SaveToFile(fullPath, data); err != nil {
		return "", err
	}

	xlog.LogInfo("utils.save_ts", "saved: "+fullPath)
	return fullPath, nil
}

func SaveJSONDebug(dir, name string, data []byte) {
	if dir == "" || name == "" {
		xlog.LogError("utils.save_json_debug", "invalid baseDir/name")
		return
	}
	if err := EnsureDir(dir); err != nil {
		xlog.LogError("utils.save_json_debug", err.Error())
		return
	}

	name = SanitizeFilename(name)
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		name += ".json"
	}

	path := filepath.Join(dir, name)
	if err := SaveToFile(path, data); err != nil {
		xlog.LogError("utils.save_json_debug", err.Error())
		return
	}

	xlog.LogInfo("debug", "saved: "+path)
}

func randHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		for i := range buf {
			buf[i] = byte(time.Now().UnixNano() >> (uint(i) & 7))
		}
	}
	return hex.EncodeToString(buf)
}
