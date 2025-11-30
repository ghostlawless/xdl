package downloader

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ghostlawless/xdl/internal/config"
	"github.com/ghostlawless/xdl/internal/httpx"
	"github.com/ghostlawless/xdl/internal/scraper"
	"github.com/ghostlawless/xdl/internal/utils"
)

type Options struct {
	RunDir         string
	User           string
	PreflightWC    int
	MediaMaxBytes  int64
	DryRun         bool
	SortBySizeDesc bool

	Attempts          int
	PerAttemptTimeout time.Duration
}

type Summary struct {
	Downloaded int
	Skipped    int
	Failed     int
	TotalBytes int64
	Cycles     int
}

type item struct {
	Idx  int
	URL  string
	Type string
	Size int64
	Ext  string
}

func DownloadAllCycles(client *http.Client, conf *config.EssentialsConfig, medias []scraper.Media, opt Options) (Summary, error) {
	s := Summary{}
	if len(medias) == 0 {
		return s, nil
	}

	dirs := subdirs(opt.RunDir)
	for _, d := range dirs.All() {
		if err := utils.EnsureDir(d); err != nil {
			return s, err
		}
	}

	items := make([]item, 0, len(medias))
	for i, m := range medias {
		ext := httpx.InferExt("", m.URL, m.Type)
		items = append(items, item{Idx: i + 1, URL: m.URL, Type: m.Type, Size: -1, Ext: ext})
	}
	preflightSizes(client, conf, items, max(4, opt.PreflightWC))

	if opt.SortBySizeDesc {
		sort.SliceStable(items, func(i, j int) bool { return items[i].Size > items[j].Size })
	}

	pending := make([]item, len(items))
	copy(pending, items)

	for len(pending) > 0 {
		k := 3 + rand.Intn(13)
		if k > len(pending) {
			k = len(pending)
		}
		batch := pending[:k]
		pending = pending[k:]

		ok, skip, fail, bytes := downloadBatch(client, conf, batch, dirs, opt)
		s.Downloaded += ok
		s.Skipped += skip
		s.Failed += fail
		s.TotalBytes += bytes
		s.Cycles++
	}
	return s, nil
}

func preflightSizes(client *http.Client, conf *config.EssentialsConfig, items []item, wc int) {
	if wc < 1 {
		wc = 1
	}
	type job struct{ idx int }
	jobs := make(chan job, len(items))
	var wg sync.WaitGroup
	for w := 0; w < wc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				_, size, _, status, err := httpx.Head(client, items[j.idx].URL, conf.X.Network)
				if err != nil || status < 200 || status >= 300 {
					items[j.idx].Size = -1
					continue
				}
				items[j.idx].Size = size
			}
		}()
	}
	for i := range items {
		jobs <- job{idx: i}
	}
	close(jobs)
	wg.Wait()
}

type subDirs struct{ Images, Videos, Gifs, Others string }

func subdirs(runDir string) subDirs {
	return subDirs{
		Images: filepath.Join(runDir, "images"),
		Videos: filepath.Join(runDir, "videos"),
		Gifs:   filepath.Join(runDir, "gifs"),
		Others: filepath.Join(runDir, "others"),
	}
}

func (sd subDirs) All() []string {
	return []string{sd.Images, sd.Videos, sd.Gifs, sd.Others}
}

func downloadBatch(client *http.Client, conf *config.EssentialsConfig, batch []item, dirs subDirs, opt Options) (ok, skipped, failed int, bytes int64) {
	var wg sync.WaitGroup
	wg.Add(len(batch))
	var mu sync.Mutex
	for _, it := range batch {
		it := it
		go func() {
			defer wg.Done()
			r := downloadOne(client, conf, it, dirs, opt)
			mu.Lock()
			defer mu.Unlock()
			if r.err != nil {
				failed++
				return
			}
			if r.skipped {
				skipped++
				return
			}
			ok++
			bytes += r.size
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

func downloadOne(client *http.Client, conf *config.EssentialsConfig, it item, dirs subDirs, opt Options) result {
	destDir := classifyDir(it, dirs)
	_ = utils.EnsureDir(destDir)

	base := basenameFromURL(it.URL)
	if base == "" {
		base = shortHash(it.URL)
	}
	base = utils.SanitizeFilename(base)

	if opt.DryRun || opt.MediaMaxBytes > 0 {
		_, size, _, status, err := httpx.Head(client, it.URL, conf.X.Network)
		if err != nil {
			if conf.Runtime.DebugEnabled {
				meta := fmt.Sprintf("HEAD_ERROR\nSTATUS: %d\nURL: %s\n", status, it.URL)
				_, _ = utils.SaveTimestamped(conf.Paths.Debug, "err_head_meta", "txt", []byte(meta))
			}
			return result{err: err}
		}
		if opt.MediaMaxBytes > 0 && size > 0 && size > opt.MediaMaxBytes {
			return result{skipped: true}
		}
		if opt.DryRun {
			return result{ok: true, size: size}
		}
	}

	ext := it.Ext
	if ext == "" {
		ext = httpx.InferExt("", it.URL, it.Type)
	}
	filename := base
	if ext != "" && !strings.HasSuffix(strings.ToLower(filename), "."+ext) {
		filename += "." + ext
	}
	dest := filepath.Join(destDir, filename)

	if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
		_ = os.Remove(dest)
	}

	req, err := http.NewRequest(http.MethodGet, it.URL, nil)
	if err != nil {
		return result{err: err}
	}
	conf.BuildRequestHeaders(req, conf.X.Network)
	req.Header.Set("Accept", "*/*")

	attempts := opt.Attempts
	if attempts <= 0 {
		attempts = 3
	}
	perTry := opt.PerAttemptTimeout
	if perTry <= 0 {
		perTry = 2 * time.Minute
	}

	var n int64
	var status int
	var lastErr error
	for a := 0; a < attempts; a++ {
		n, status, lastErr = httpx.DownloadToFileWithTimeout(client, req, dest, opt.MediaMaxBytes, perTry)
		if lastErr == nil {
			return result{ok: true, size: n}
		}
		if isTimeoutOrTemp(lastErr) {
			sleep := backoff(a)
			if conf.Runtime.DebugEnabled {
				meta := fmt.Sprintf("RETRY a=%d sleep=%s status=%d url=%s err=%v\n", a+1, sleep, status, it.URL, lastErr)
				_, _ = utils.SaveTimestamped(conf.Paths.Debug, "err_download_meta", "txt", []byte(meta))
			}
			time.Sleep(sleep)
			continue
		}
		break
	}
	if conf.Runtime.DebugEnabled {
		meta := fmt.Sprintf("DOWNLOAD_ERROR\nSTATUS: %d\nURL: %s\nDEST: %s\nERR: %v\n", status, it.URL, dest, lastErr)
		_, _ = utils.SaveTimestamped(conf.Paths.Debug, "err_download_meta", "txt", []byte(meta))
	}
	return result{err: lastErr}
}

func classifyDir(it item, dirs subDirs) string {
	lurl := strings.ToLower(strings.Split(it.URL, "?")[0])
	switch {
	case strings.HasSuffix(lurl, ".gif"):
		return dirs.Gifs
	case strings.HasSuffix(lurl, ".mp4"), strings.HasSuffix(lurl, ".m3u8"), it.Type == "video":
		return dirs.Videos
	case strings.HasSuffix(lurl, ".jpg"), strings.HasSuffix(lurl, ".jpeg"),
		strings.HasSuffix(lurl, ".png"), strings.HasSuffix(lurl, ".webp"), it.Type == "image":
		return dirs.Images
	default:
		return dirs.Others
	}
}

func basenameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return ""
	}
	b := path.Base(u.Path)
	if b == "." || b == "/" || b == "" {
		return ""
	}
	b = strings.SplitN(b, "?", 2)[0]
	return b
}

func shortHash(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:8])
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func isTimeoutOrTemp(err error) bool {
	if err == nil {
		return false
	}
	var nerr net.Error
	if errors.As(err, &nerr) {
		if nerr.Timeout() || nerr.Temporary() {
			return true
		}
	}
	if errors.Is(err, contextDeadlineExceeded) {
		return true
	}
	e := strings.ToLower(err.Error())
	return strings.Contains(e, "timeout") || strings.Contains(e, "deadline")
}

var contextDeadlineExceeded = func() error {
	return errors.New("context deadline exceeded")
}()

func backoff(attempt int) time.Duration {
	base := 500 * time.Millisecond
	maxDur := 8 * time.Second
	d := base * time.Duration(1<<attempt)
	if d > maxDur {
		d = maxDur
	}
	j := time.Duration(rand.Int63n(int64(d/2))) - d/4
	return d + j
}
