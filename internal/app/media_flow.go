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
	rctx RunContext,
	username string,
	page int,
	totalItems int,
) func(downloader.ProgressEvent) {
	if totalItems <= 0 {
		return nil
	}

	type counters struct {
		ok     int
		skip   int
		fail   int
		bytes  int64
		events int
	}
	c := &counters{}

	switch rctx.Mode {
	case ModeVerbose:
		return func(ev downloader.ProgressEvent) {
			if globalControl.ShouldQuit() {
				return
			}

			switch ev.Kind {
			case downloader.ProgressKindDownloaded:
				c.ok++
				c.bytes += ev.Size
			case downloader.ProgressKindSkipped:
				c.skip++
			case downloader.ProgressKindFailed:
				c.fail++
			}

			done := c.ok + c.skip + c.fail
			if done <= 0 {
				return
			}

			f := float64(done) / float64(totalItems)
			if f < 0 {
				f = 0
			}
			if f > 1 {
				f = 1
			}
			pct := f * 100.0
			bar := buildProgressBar(30, f)

			sfx := ""
			if globalControl.ShouldPause() {
				sfx = " [paused]"
			}

			termMu.Lock()
			defer termMu.Unlock()

			fmt.Printf(
				"\rxdl> [@%s]%s [page:%d] [%s] %3.0f%% %d/%d (ok:%d skip:%d fail:%d)",
				username, sfx, page, bar, pct, done, totalItems,
				c.ok, c.skip, c.fail,
			)
		}

	case ModeDebug:
		return func(ev downloader.ProgressEvent) {
			switch ev.Kind {
			case downloader.ProgressKindDownloaded:
				c.ok++
				c.bytes += ev.Size
			case downloader.ProgressKindSkipped:
				c.skip++
			case downloader.ProgressKindFailed:
				c.fail++
			}

			done := c.ok + c.skip + c.fail
			if done <= 0 {
				return
			}

			c.events++
			logNow := false
			if totalItems <= 50 {
				logNow = true
			} else if c.events%10 == 0 || done == totalItems {
				logNow = true
			}
			if !logNow {
				return
			}

			f := float64(done) / float64(totalItems)
			if f < 0 {
				f = 0
			}
			if f > 1 {
				f = 1
			}
			percent := int(f*100 + 0.5)

			msg := fmt.Sprintf(
				"progress user=%s page=%d done=%d/%d (%d%%) ok=%d skip=%d fail=%d bytes=%d",
				username,
				page,
				done,
				totalItems,
				percent,
				c.ok,
				c.skip,
				c.fail,
				c.bytes,
			)
			log.LogInfo("download", msg)
		}

	default:
		return nil
	}
}

func scanAndDownloadUserMedia(
	rctx RunContext,
	conf *config.EssentialsConfig,
	apiClient, dlClient *http.Client,
	uid string,
	username string,
	runDir string,
	lim *runtime.Limiter,
) (scanResult, downloadStats, error) {
	accumulator := newScanAccumulator(256)
	stats := downloadStats{}

	vb := rctx.Mode == ModeVerbose && len(rctx.Users) == 1

	handler := func(page int, cursor string, medias []scraper.Media) error {
		_ = cursor

		if globalControl.ShouldQuit() {
			return fmt.Errorf("aborted by user")
		}

		if len(medias) == 0 {
			return nil
		}

		accumulator.Add(medias)

		enriched := scraper.EnrichMediaWithTweetDetail(apiClient, conf, username, medias, lim, vb)
		if len(enriched) == 0 {
			return nil
		}

		cb := newPageProgressCallback(rctx, username, page, len(enriched))

		summary, err := downloader.DownloadAllCycles(dlClient, conf, enriched, downloader.Options{
			RunDir:            runDir,
			User:              username,
			MediaMaxBytes:     0,
			DryRun:            rctx.DryRun,
			Attempts:          3,
			PerAttemptTimeout: 2 * time.Minute,
			Progress:          cb,
			ShouldPause:       globalControl.ShouldPause,
			ShouldQuit:        globalControl.ShouldQuit,
		})
		if err != nil {
			log.LogError("download", err.Error())
			return err
		}

		stats.Downloaded += summary.Downloaded
		stats.Skipped += summary.Skipped
		stats.Failed += summary.Failed
		stats.Bytes += summary.TotalBytes

		if rctx.Mode == ModeDebug {
			log.LogInfo("download", fmt.Sprintf(
				"page=%d user=%s ok=%d skip=%d fail=%d bytes=%d cycles=%d",
				page, username,
				summary.Downloaded,
				summary.Skipped,
				summary.Failed,
				summary.TotalBytes,
				summary.Cycles,
			))
		}

		if globalControl.ShouldQuit() {
			if rctx.Mode == ModeVerbose {
				termMu.Lock()
				fmt.Print("\n")
				termMu.Unlock()
				utils.PrintWarn("run aborted by user for @%s", username)
			}
			return fmt.Errorf("aborted by user")
		}

		if rctx.Mode == ModeVerbose && cb != nil {
			termMu.Lock()
			fmt.Print("\n")
			termMu.Unlock()
		}

		return nil
	}

	err := scraper.WalkUserMediaPages(apiClient, conf, uid, username, vb, lim, handler)
	if err != nil {
		return accumulator.Result(), stats, err
	}

	return accumulator.Result(), stats, nil
}
