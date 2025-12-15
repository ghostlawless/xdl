package app

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ghostlawless/xdl/internal/config"
	"github.com/ghostlawless/xdl/internal/downloader"
	"github.com/ghostlawless/xdl/internal/log"
	"github.com/ghostlawless/xdl/internal/runtime"
	"github.com/ghostlawless/xdl/internal/scraper"
	"github.com/ghostlawless/xdl/internal/utils"
)

type scanAccumulator struct {
	media      []scraper.Media
	mediaCount int
	imageCount int
	videoCount int
}

func newScanAccumulator(initialCapacity int) *scanAccumulator {
	if initialCapacity <= 0 {
		initialCapacity = 256
	}
	return &scanAccumulator{
		media: make([]scraper.Media, 0, initialCapacity),
	}
}

func (a *scanAccumulator) Add(medias []scraper.Media) {
	if len(medias) == 0 {
		return
	}

	a.mediaCount += len(medias)
	for _, m := range medias {
		switch m.Type {
		case "image":
			a.imageCount++
		case "video":
			a.videoCount++
		}
	}

	a.media = append(a.media, medias...)

}

func (a *scanAccumulator) Result() scanResult {
	return scanResult{
		Media:       a.media,
		TotalMedia:  a.mediaCount,
		TotalImages: a.imageCount,
		TotalVideos: a.videoCount,
	}
}

type scanResult struct {
	Media       []scraper.Media
	TotalMedia  int
	TotalImages int
	TotalVideos int
}

type downloadStats struct {
	Downloaded int
	Skipped    int
	Failed     int
	Bytes      int64
}

func newPageProgressCallback(
	r0 RunContext,
	u0 string,
	p0 int,
	n0 int,
) func(downloader.ProgressEvent) {
	if n0 <= 0 {
		return nil
	}

	type x1 struct {
		a int
		b int
		c int
		d int64
		e int
	}
	x0 := &x1{}

	switch r0.Mode {
	case ModeVerbose:
		return func(ev downloader.ProgressEvent) {
			if globalControl.ShouldQuit() {
				return
			}

			switch ev.Kind {
			case downloader.ProgressKindDownloaded:
				x0.a++
				x0.d += ev.Size
			case downloader.ProgressKindSkipped:
				x0.b++
			case downloader.ProgressKindFailed:
				x0.c++
			}

			k0 := x0.a + x0.b + x0.c
			if k0 <= 0 {
				return
			}

			f0 := float64(k0) / float64(n0)
			if f0 < 0 {
				f0 = 0
			}
			if f0 > 1 {
				f0 = 1
			}

			pct := f0 * 100.0
			bar := buildProgressBar(30, f0)

			sfx := ""
			if globalControl.ShouldPause() {
				sfx = " (paused)"
			}

			termMu.Lock()
			defer termMu.Unlock()

			fmt.Printf(
				"\rxdl @%s%s  page %d  [%s] %3.0f%%  %d/%d  (ok:%d skip:%d fail:%d)",
				u0, sfx, p0, bar, pct, k0, n0,
				x0.a, x0.b, x0.c,
			)
		}

	case ModeDebug:
		return func(ev downloader.ProgressEvent) {
			switch ev.Kind {
			case downloader.ProgressKindDownloaded:
				x0.a++
				x0.d += ev.Size
			case downloader.ProgressKindSkipped:
				x0.b++
			case downloader.ProgressKindFailed:
				x0.c++
			}

			k0 := x0.a + x0.b + x0.c
			if k0 <= 0 {
				return
			}

			x0.e++
			emit := false
			if n0 <= 50 {
				emit = true
			} else if x0.e%10 == 0 || k0 == n0 {
				emit = true
			}
			if !emit {
				return
			}

			f0 := float64(k0) / float64(n0)
			if f0 < 0 {
				f0 = 0
			}
			if f0 > 1 {
				f0 = 1
			}
			pct := int(f0*100 + 0.5)

			msg := fmt.Sprintf(
				"progress user=%s page=%d done=%d/%d (%d%%) ok=%d skip=%d fail=%d bytes=%d",
				u0,
				p0,
				k0,
				n0,
				pct,
				x0.a,
				x0.b,
				x0.c,
				x0.d,
			)
			log.LogInfo("download", msg)
		}

	default:
		return nil
	}

}

func scanAndDownloadUserMedia(
	r0 RunContext,
	c0 *config.EssentialsConfig,
	h0, h1 *http.Client,
	u0 string,
	u1 string,
	d0 string,
	l0 *runtime.Limiter,
) (scanResult, downloadStats, error) {
	a0 := newScanAccumulator(256)
	s0 := downloadStats{}

	v0 := r0.Mode == ModeVerbose && len(r0.Users) == 1

	f0 := func(p0 int, _ string, m0 []scraper.Media) error {
		if globalControl.ShouldQuit() {
			return fmt.Errorf("Stopped by user.")
		}

		if len(m0) == 0 {
			return nil
		}

		a0.Add(m0)

		e0 := scraper.EnrichMediaWithTweetDetail(h0, c0, u1, m0, l0, v0)
		if len(e0) == 0 {
			return nil
		}

		cb := newPageProgressCallback(r0, u1, p0, len(e0))

		sum, err := downloader.DownloadAllCycles(h1, c0, e0, downloader.Options{
			RunDir:            d0,
			User:              u1,
			MediaMaxBytes:     0,
			DryRun:            r0.DryRun,
			Attempts:          3,
			PerAttemptTimeout: 2 * time.Minute,
			Progress:          cb,
			ShouldPause:       globalControl.ShouldPause,
			ShouldQuit:        globalControl.ShouldQuit,
		})
		if err != nil {
			log.LogError("download", err.Error())
			return fmt.Errorf("Download failed for @%s. Try again, or run with -d to generate logs.", u1)
		}

		s0.Downloaded += sum.Downloaded
		s0.Skipped += sum.Skipped
		s0.Failed += sum.Failed
		s0.Bytes += sum.TotalBytes

		if r0.Mode == ModeDebug {
			log.LogInfo("download", fmt.Sprintf(
				"page=%d user=%s ok=%d skip=%d fail=%d bytes=%d cycles=%d",
				p0, u1,
				sum.Downloaded,
				sum.Skipped,
				sum.Failed,
				sum.TotalBytes,
				sum.Cycles,
			))
		}

		if globalControl.ShouldQuit() {
			if r0.Mode == ModeVerbose {
				termMu.Lock()
				fmt.Print("\n")
				termMu.Unlock()
				utils.PrintWarn("Stopped by user for @%s", u1)
			}
			return fmt.Errorf("Stopped by user.")
		}

		if r0.Mode == ModeVerbose && cb != nil {
			termMu.Lock()
			fmt.Print("\n")
			termMu.Unlock()
		}

		return nil
	}

	if err := scraper.WalkUserMediaPages(h0, c0, u0, u1, v0, l0, f0); err != nil {
		return a0.Result(), s0, err
	}

	return a0.Result(), s0, nil

}
