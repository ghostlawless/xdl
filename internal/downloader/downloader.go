package downloader

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ghostlawless/xdl/internal/config"
	"github.com/ghostlawless/xdl/internal/httpx"
	"github.com/ghostlawless/xdl/internal/scraper"
	"github.com/ghostlawless/xdl/internal/utils"
)

type Options struct {
	RunDir            string
	User              string
	MediaMaxBytes     int64
	DryRun            bool
	Attempts          int
	PerAttemptTimeout time.Duration
	Progress          func(ProgressEvent)
	ShouldPause       func() bool
	ShouldQuit        func() bool
	Checkpoint        *Checkpoint

	Concurrency         int
	BatchSize           int
	JobJitterMax        time.Duration
	JitterDeterministic bool
}

type Summary struct {
	Downloaded int
	Skipped    int
	Failed     int
	TotalBytes int64
	Cycles     int
}

type ProgressKind int

const (
	ProgressKindDownloaded ProgressKind = iota
	ProgressKindSkipped
	ProgressKindFailed
)

type ProgressEvent struct {
	User string
	Kind ProgressKind
	Size int64
}

type item struct {
	Idx  int
	URL  string
	Type string
	Size int64
	Ext  string
}

func DownloadAllCycles(cl *http.Client, cf *config.EssentialsConfig, ms []scraper.Media, opt Options) (Summary, error) {
	s := Summary{}
	if len(ms) == 0 {
		return s, nil
	}
	ds := binsOf(opt.RunDir)
	for _, d := range ds.all() {
		if err := utils.EnsureDir(d); err != nil {
			return s, err
		}
	}
	cp := opt.Checkpoint
	if cp == nil {
		cp = NewCheckpoint(opt.User, "", ms)
	}
	it := make([]item, 0, len(cp.Items))
	for _, v := range cp.Items {
		switch v.Status {
		case CheckpointDone, CheckpointSkipped:
			s.Skipped++
			continue
		default:
			ext := httpx.InferExt("", v.URL, v.Type)
			it = append(it, item{Idx: v.Index, URL: v.URL, Type: v.Type, Size: v.Size, Ext: ext})
		}
	}
	if len(it) == 0 {
		return s, nil
	}

	cc := opt.Concurrency
	if cc <= 0 {
		cc = runtime.NumCPU()
	}
	bs := opt.BatchSize
	if bs <= 0 {
		bs = cc * 2
	}

	pd := make([]item, len(it))
	copy(pd, it)

	for len(pd) > 0 {
		if opt.ShouldQuit != nil && opt.ShouldQuit() {
			return s, errors.New("download aborted by user")
		}
		if opt.ShouldPause != nil && opt.ShouldPause() {
			for opt.ShouldPause != nil && opt.ShouldPause() {
				if opt.ShouldQuit != nil && opt.ShouldQuit() {
					return s, errors.New("download aborted by user")
				}
				time.Sleep(200 * time.Millisecond)
			}
			if opt.ShouldQuit != nil && opt.ShouldQuit() {
				return s, errors.New("download aborted by user")
			}
		}

		k := bs
		if k > len(pd) {
			k = len(pd)
		}
		b := pd[:k]
		pd = pd[k:]

		ok, sk, fl, by := doBatch(cl, cf, b, ds, opt, cp)
		s.Downloaded += ok
		s.Skipped += sk
		s.Failed += fl
		s.TotalBytes += by
		s.Cycles++
	}
	return s, nil
}

type bins struct {
	I string
	V string
}

func binsOf(root string) bins {
	return bins{
		I: filepath.Join(root, "images"),
		V: filepath.Join(root, "videos"),
	}
}

func (sd bins) all() []string {
	return []string{sd.I, sd.V}
}

func doBatch(cl *http.Client, cf *config.EssentialsConfig, b []item, ds bins, opt Options, cp *Checkpoint) (ok, sk, fl int, by int64) {
	var wg sync.WaitGroup
	wg.Add(len(b))

	cc := opt.Concurrency
	if cc <= 0 {
		cc = runtime.NumCPU()
	}
	sem := make(chan struct{}, cc)

	var mu sync.Mutex
	for _, it := range b {
		it := it
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			if d := calcJobJitter(it, opt); d > 0 {
				if err := waitDurationWithControls(d, opt); err != nil {
					mu.Lock()
					fl++
					if cp != nil {
						cp.MarkByURL(it.URL, CheckpointFailed, 0)
					}
					mu.Unlock()
					return
				}
			}

			if opt.ShouldQuit != nil && opt.ShouldQuit() {
				mu.Lock()
				fl++
				if cp != nil {
					cp.MarkByURL(it.URL, CheckpointFailed, 0)
				}
				mu.Unlock()
				return
			}

			r := doOne(cl, cf, it, ds, opt)
			mu.Lock()
			defer mu.Unlock()
			if r.err != nil {
				fl++
				if cp != nil {
					cp.MarkByURL(it.URL, CheckpointFailed, 0)
				}
				if opt.Progress != nil {
					opt.Progress(ProgressEvent{User: opt.User, Kind: ProgressKindFailed, Size: 0})
				}
				return
			}
			if r.skipped {
				sk++
				if cp != nil {
					cp.MarkByURL(it.URL, CheckpointSkipped, r.size)
				}
				if opt.Progress != nil {
					opt.Progress(ProgressEvent{User: opt.User, Kind: ProgressKindSkipped, Size: 0})
				}
				return
			}
			ok++
			by += r.size
			if cp != nil {
				cp.MarkByURL(it.URL, CheckpointDone, r.size)
			}
			if opt.Progress != nil {
				opt.Progress(ProgressEvent{User: opt.User, Kind: ProgressKindDownloaded, Size: r.size})
			}
		}()
	}
	wg.Wait()
	return
}

type result struct {
	ok      bool
	skipped bool
	size    int64
	err     error
}

func doOne(cl *http.Client, cf *config.EssentialsConfig, it item, ds bins, opt Options) result {
	dst := pick(it, ds)
	_ = utils.EnsureDir(dst)
	base := baseFrom(it.URL)
	if base == "" {
		base = sh(it.URL)
	}
	base = utils.SanitizeFilename(base)
	if opt.DryRun || opt.MediaMaxBytes > 0 {
		_, sz, _, st, err := httpx.Head(cl, it.URL, cf.X.Network)
		if err != nil {
			if cf.Runtime.DebugEnabled {
				meta := fmt.Sprintf("HEAD_ERROR\nSTATUS: %d\nURL: %s\n", st, it.URL)
				_, _ = utils.SaveTimestamped(cf.Paths.Debug, "err_head_meta", "txt", []byte(meta))
			}
			return result{err: err}
		}
		if opt.MediaMaxBytes > 0 && sz > 0 && sz > opt.MediaMaxBytes {
			return result{skipped: true}
		}
		if opt.DryRun {
			return result{ok: true, size: sz}
		}
	}
	ext := it.Ext
	if ext == "" {
		ext = httpx.InferExt("", it.URL, it.Type)
	}
	fn := base
	if ext != "" && !strings.HasSuffix(strings.ToLower(fn), "."+ext) {
		fn += "." + ext
	}
	full := filepath.Join(dst, fn)
	if st, err := os.Stat(full); err == nil && st.Size() > 0 {
		return result{skipped: true, size: st.Size()}
	}
	req, err := http.NewRequest(http.MethodGet, it.URL, nil)
	if err != nil {
		return result{err: err}
	}
	cf.BuildRequestHeaders(req, cf.X.Network)
	req.Header.Set("Accept", "*/*")
	at := opt.Attempts
	if at <= 0 {
		at = 3
	}
	to := opt.PerAttemptTimeout
	if to <= 0 {
		to = 2 * time.Minute
	}
	var n int64
	var st int
	var last error
	for i := 0; i < at; i++ {
		n, st, last = httpx.DownloadToFileWithTimeout(cl, req, full, opt.MediaMaxBytes, to)
		if last == nil {
			return result{ok: true, size: n}
		}
		if isTemp(last) {
			sl := backoff(i)
			if cf.Runtime.DebugEnabled {
				meta := fmt.Sprintf("RETRY a=%d sleep=%s status=%d url=%s err=%v\n", i+1, sl, st, it.URL, last)
				_, _ = utils.SaveTimestamped(cf.Paths.Debug, "err_download_meta", "txt", []byte(meta))
			}
			time.Sleep(sl)
			continue
		}
		break
	}
	if cf.Runtime.DebugEnabled {
		meta := fmt.Sprintf("DOWNLOAD_ERROR\nSTATUS: %d\nURL: %s\nDEST: %s\nERR: %v\n", st, it.URL, full, last)
		_, _ = utils.SaveTimestamped(cf.Paths.Debug, "err_download_meta", "txt", []byte(meta))
	}
	return result{err: last}
}

func pick(it item, ds bins) string {
	u := it.URL
	if i := strings.IndexByte(u, '?'); i >= 0 {
		u = u[:i]
	}
	l := strings.ToLower(u)
	switch {
	case strings.HasSuffix(l, ".mp4"), strings.HasSuffix(l, ".m3u8"), it.Type == "video":
		return ds.V
	case strings.HasSuffix(l, ".jpg"), strings.HasSuffix(l, ".jpeg"), strings.HasSuffix(l, ".png"), strings.HasSuffix(l, ".webp"), strings.HasSuffix(l, ".gif"), it.Type == "image":
		return ds.I
	default:
		return ds.I
	}
}

func baseFrom(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return ""
	}
	b := path.Base(u.Path)
	if b == "." || b == "/" || b == "" {
		return ""
	}
	return strings.SplitN(b, "?", 2)[0]
}

func sh(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:8])
}

func isTemp(err error) bool {
	if err == nil {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) {
		if ne.Timeout() || ne.Temporary() {
			return true
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	e := strings.ToLower(err.Error())
	return strings.Contains(e, "timeout") || strings.Contains(e, "deadline")
}

func backoff(i int) time.Duration {
	base := 500 * time.Millisecond
	max := 8 * time.Second
	d := base * time.Duration(1<<i)
	if d > max {
		d = max
	}
	j := time.Duration(rand.Int63n(int64(d/2))) - d/4
	return d + j
}

func calcJobJitter(it item, opt Options) time.Duration {
	if opt.JobJitterMax <= 0 {
		return 0
	}
	if opt.JitterDeterministic {
		h := fnv.New64a()
		_, _ = h.Write([]byte(opt.User))
		_, _ = h.Write([]byte("|"))
		_, _ = h.Write([]byte(it.URL))
		return time.Duration(h.Sum64() % uint64(opt.JobJitterMax))
	}

	return time.Duration(rand.Int63n(int64(opt.JobJitterMax)))
}

func waitDurationWithControls(d time.Duration, opt Options) error {
	if d <= 0 {
		return nil
	}
	start := time.Now()
	tick := 50 * time.Millisecond
	for {
		if opt.ShouldQuit != nil && opt.ShouldQuit() {
			return errors.New("aborted by user")
		}
		if opt.ShouldPause != nil && opt.ShouldPause() {
			for opt.ShouldPause != nil && opt.ShouldPause() {
				if opt.ShouldQuit != nil && opt.ShouldQuit() {
					return errors.New("aborted by user")
				}
				time.Sleep(100 * time.Millisecond)
			}
			start = time.Now()
		}
		if time.Since(start) >= d {
			return nil
		}
		time.Sleep(tick)
	}
}
