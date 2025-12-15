package httpx

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

type RequestOptions struct {
	Decode       bool
	MaxBytes     int64
	Accept       func(int) bool
	DebugLogPath string
}

func ualist() []string {
	return []string{
		"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Mobile Safari/537.36",
		"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Edg/139.0.0.0 Mobile Safari/537.36",
	}
}

func uapick(u *url.URL) string {
	l := ualist()
	if len(l) == 0 {
		return "Mozilla/5.0"
	}
	k := ""
	if u != nil {
		k = strings.ToLower(u.Host) + u.Path
	}
	if k == "" {
		return l[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(k))
	i := int(h.Sum32() % uint32(len(l)))
	return l[i]
}

func stdh(rq *http.Request) {
	if rq == nil {
		return
	}
	if rq.Header.Get("User-Agent") == "" {
		ua := strings.TrimSpace(os.Getenv("XDL_UA"))
		if ua == "" {
			ua = uapick(rq.URL)
		}
		rq.Header.Set("User-Agent", ua)
	}
	if rq.Header.Get("Accept") == "" {
		ac := strings.TrimSpace(os.Getenv("XDL_ACCEPT"))
		if ac == "" {
			ac = "*/*"
		}
		rq.Header.Set("Accept", ac)
	}
}

func DoRequestWithOptions(cl *http.Client, rq *http.Request, op RequestOptions) ([]byte, int, error) {
	if cl == nil || rq == nil {
		return nil, 0, errors.New("nil client or request")
	}

	stdh(rq)
	rq.Header.Set("Referer", "https://x.com/")

	res, err := cl.Do(rq)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()

	st := res.StatusCode

	if op.Accept == nil {
		op.Accept = func(s int) bool { return s >= 200 && s < 300 }
	}

	var rd io.Reader = res.Body
	if op.Decode {
		switch strings.ToLower(res.Header.Get("Content-Encoding")) {
		case "gzip":
			g, err := gzip.NewReader(res.Body)
			if err == nil {
				defer g.Close()
				rd = g
			}
		case "br":
			rd = brotli.NewReader(res.Body)
		case "zstd":
			zr, err := zstd.NewReader(res.Body)
			if err == nil {
				defer zr.Close()
				rd = zr
			}
		}
	}

	if op.MaxBytes > 0 {
		rd = io.LimitReader(rd, op.MaxBytes)
	}

	b, rerr := io.ReadAll(rd)
	if rerr != nil {
		return b, st, rerr
	}

	if !op.Accept(st) {
		return b, st, fmt.Errorf("unacceptable HTTP status: %d", st)
	}

	return b, st, nil
}

func Head(cl *http.Client, raw, ref string) (http.Header, int64, string, int, error) {
	if cl == nil {
		return nil, 0, "", 0, errors.New("nil client")
	}
	rq, err := http.NewRequest(http.MethodHead, raw, nil)
	if err != nil {
		return nil, 0, "", 0, err
	}
	stdh(rq)
	rq.Header.Set("Referer", "https://x.com/")
	res, err := cl.Do(rq)
	if err != nil {
		return nil, 0, "", 0, err
	}
	defer res.Body.Close()
	return res.Header.Clone(), res.ContentLength, res.Header.Get("Content-Type"), res.StatusCode, nil
}

func DownloadToFile(cl *http.Client, rq *http.Request, dst string, max int64) (int64, int, error) {
	if cl == nil || rq == nil {
		return 0, 0, errors.New("nil client or request")
	}
	stdh(rq)
	rq.Header.Set("Referer", "https://x.com/")
	res, err := cl.Do(rq)
	if err != nil {
		return 0, 0, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, res.Body)
		return 0, res.StatusCode, fmt.Errorf("unacceptable HTTP status: %d", res.StatusCode)
	}
	dir := filepath.Dir(dst)
	base := filepath.Base(dst)
	tmp, err := os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		_, _ = io.Copy(io.Discard, res.Body)
		return 0, res.StatusCode, err
	}
	tpath := tmp.Name()
	var src io.Reader = res.Body
	if max > 0 {
		src = io.LimitReader(res.Body, max)
	}
	n, cerr := io.Copy(tmp, src)
	clos := tmp.Close()
	if cerr != nil {
		_ = os.Remove(tpath)
		return n, res.StatusCode, cerr
	}
	if clos != nil {
		_ = os.Remove(tpath)
		return n, res.StatusCode, clos
	}
	if _, err := os.Stat(dst); err == nil {
		_ = os.Remove(dst)
	}
	if err := os.Rename(tpath, dst); err == nil {
		return n, res.StatusCode, nil
	}
	in, err := os.Open(tpath)
	if err != nil {
		_ = os.Remove(tpath)
		return n, res.StatusCode, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		_ = os.Remove(tpath)
		return n, res.StatusCode, err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(dst)
		_ = os.Remove(tpath)
		return n, res.StatusCode, err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		_ = os.Remove(tpath)
		return n, res.StatusCode, err
	}
	_ = os.Remove(tpath)
	return n, res.StatusCode, nil
}

var ErrNot2xx = errors.New("non-2xx response")

func InferExt(ct, raw, mt string) string {
	l := strings.ToLower(ct)
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
	u := strings.ToLower(strings.Split(raw, "?")[0])
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
	if mt == "video" {
		return "mp4"
	}
	if mt == "image" {
		return "jpg"
	}
	return ""
}

func DownloadToFileWithTimeout(cl *http.Client, rq *http.Request, dst string, max int64, per time.Duration) (int64, int, error) {
	if cl == nil || rq == nil {
		return 0, 0, errors.New("nil client or request")
	}
	ctx := rq.Context()
	if per > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, per)
		defer cancel()
	}
	rq = rq.Clone(ctx)
	return DownloadToFile(cl, rq, dst, max)
}
