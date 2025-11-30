package httpx

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

type RequestOptions struct {
	MaxBytes int64
	Decode   bool
	Accept   func(status int) bool
}

func DoRequestWithOptions(client *http.Client, req *http.Request, opt RequestOptions) ([]byte, int, error) {
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	status := res.StatusCode
	if opt.Accept == nil {
		opt.Accept = func(s int) bool { return s >= 200 && s < 300 }
	}

	var r io.Reader = res.Body
	if opt.Decode {
		switch strings.ToLower(res.Header.Get("Content-Encoding")) {
		case "gzip":
			gz, err := gzip.NewReader(res.Body)
			if err == nil {
				defer gz.Close()
				r = gz
			}
		case "br":
			r = brotli.NewReader(res.Body)
		case "zstd":
			zr, err := zstd.NewReader(res.Body)
			if err == nil {
				defer zr.Close()
				r = zr
			}
		}
	}
	reader := r
	if opt.MaxBytes > 0 {
		reader = io.LimitReader(r, opt.MaxBytes)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return body, status, err
	}
	if !opt.Accept(status) {
		return body, status, fmt.Errorf("unacceptable HTTP status: %d", status)
	}
	return body, status, nil
}

func Head(client *http.Client, rawURL, referer string) (http.Header, int64, string, int, error) {
	req, err := http.NewRequest(http.MethodHead, rawURL, nil)
	if err != nil {
		return nil, 0, "", 0, err
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, "", 0, err
	}
	defer res.Body.Close()
	return res.Header.Clone(), res.ContentLength, res.Header.Get("Content-Type"), res.StatusCode, nil
}

func DownloadToFile(client *http.Client, req *http.Request, destPath string, maxBytes int64) (int64, int, error) {
	res, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		io.Copy(io.Discard, res.Body)
		return 0, res.StatusCode, fmt.Errorf("unacceptable HTTP status: %d", res.StatusCode)
	}

	dir := filepath.Dir(destPath)
	base := filepath.Base(destPath)
	tmpFile, err := os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		io.Copy(io.Discard, res.Body)
		return 0, res.StatusCode, err
	}
	tmpPath := tmpFile.Name()

	var src io.Reader = res.Body
	if maxBytes > 0 {
		src = io.LimitReader(res.Body, maxBytes)
	}
	n, copyErr := io.Copy(tmpFile, src)
	closeErr := tmpFile.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return n, res.StatusCode, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return n, res.StatusCode, closeErr
	}

	if _, err := os.Stat(destPath); err == nil {
		_ = os.Remove(destPath)
	}
	if err := os.Rename(tmpPath, destPath); err == nil {
		return n, res.StatusCode, nil
	}

	in, err := os.Open(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return n, res.StatusCode, err
	}
	defer in.Close()
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		_ = os.Remove(tmpPath)
		return n, res.StatusCode, err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(destPath)
		_ = os.Remove(tmpPath)
		return n, res.StatusCode, err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(destPath)
		_ = os.Remove(tmpPath)
		return n, res.StatusCode, err
	}
	_ = os.Remove(tmpPath)
	return n, res.StatusCode, nil
}

var ErrNot2xx = errors.New("non-2xx response")

func InferExt(contentType, rawURL, mediaType string) string {
	l := strings.ToLower(contentType)
	switch {
	case strings.Contains(l, "video/mp4"):
		return "mp4"
	case strings.Contains(l, "application/x-mpegurl"), strings.Contains(l, "vnd.apple.mpegurl"):
		return "m3u8"
	case strings.Contains(l, "image/jpeg"):
		return "jpg"
	case strings.Contains(l, "image/png"):
		return "png"
	case strings.Contains(l, "image/gif"):
		return "gif"
	case strings.Contains(l, "image/webp"):
		return "webp"
	}
	u := strings.ToLower(strings.Split(rawURL, "?")[0])
	switch {
	case strings.HasSuffix(u, ".mp4"):
		return "mp4"
	case strings.HasSuffix(u, ".m3u8"):
		return "m3u8"
	case strings.HasSuffix(u, ".jpg"), strings.HasSuffix(u, ".jpeg"):
		return "jpg"
	case strings.HasSuffix(u, ".png"):
		return "png"
	case strings.HasSuffix(u, ".gif"):
		return "gif"
	case strings.HasSuffix(u, ".webp"):
		return "webp"
	}
	if mediaType == "video" {
		return "mp4"
	}
	if mediaType == "image" {
		return "jpg"
	}
	return ""
}

func DownloadToFileWithTimeout(client *http.Client, req *http.Request, destPath string, maxBytes int64, perAttemptTimeout time.Duration) (int64, int, error) {
	ctx := req.Context()
	if perAttemptTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, perAttemptTimeout)
		defer cancel()
	}
	req = req.Clone(ctx)
	return DownloadToFile(client, req, destPath, maxBytes)
}
