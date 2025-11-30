package utils

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	xlog "github.com/ghostlawless/xdl/internal/log"
)

func EnsureDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("empty dir")
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		xlog.LogError("utils.ensure_dir", err.Error())
		return err
	}
	return nil
}

func DirExists(dir string) bool {
	st, err := os.Stat(dir)
	return err == nil && st.IsDir()
}

func SanitizeFilename(name string) string {
	if name == "" {
		return "file"
	}
	name = filepath.Base(name)
	r := strings.NewReplacer(
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
	name = r.Replace(name)
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
	tmp, err := os.CreateTemp(filepath.Dir(path), base+".tmp-*")
	if err != nil {
		xlog.LogError("utils.save_file", err.Error())
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		xlog.LogError("utils.save_file", err.Error())
		return err
	}
	if err := tmp.Close(); err != nil {
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

func SaveText(path string, text string) error {
	return SaveToFile(path, []byte(text))
}

func SaveTimestamped(baseDir, prefix, ext string, data []byte) (string, error) {
	if baseDir == "" {
		return "", fmt.Errorf("empty baseDir")
	}
	if err := EnsureDir(baseDir); err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102_150405.000000000")
	sfx := randHex(4)
	prefix = SanitizeFilename(prefix)
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" {
		ext = "bin"
	}
	name := fmt.Sprintf("%s_%s_%s.%s", prefix, ts, sfx, ext)
	full := filepath.Join(baseDir, name)
	if err := SaveToFile(full, data); err != nil {
		return "", err
	}
	xlog.LogInfo("utils.save_ts", "saved: "+full)
	return full, nil
}

func SaveJSONDebug(baseDir, name string, content []byte) {
	if baseDir == "" || name == "" {
		xlog.LogError("utils.save_json_debug", "invalid baseDir/name")
		return
	}
	if err := EnsureDir(baseDir); err != nil {
		xlog.LogError("utils.save_json_debug", err.Error())
		return
	}
	name = SanitizeFilename(name)
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		name += ".json"
	}
	path := filepath.Join(baseDir, name)
	if err := SaveToFile(path, content); err != nil {
		xlog.LogError("utils.save_json_debug", err.Error())
		return
	}
	xlog.LogInfo("debug", "saved: "+path)
}

func PromptYesNoDefaultYes(question string) bool {
	fmt.Fprint(os.Stdout, question)
	in := bufio.NewReader(os.Stdin)
	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "" || line == "y" || line == "yes"
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		for i := range b {
			b[i] = byte(time.Now().UnixNano() >> (uint(i) & 7))
		}
	}
	return hex.EncodeToString(b)
}
