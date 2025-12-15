package app

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ghostlawless/xdl/internal/config"
	"github.com/ghostlawless/xdl/internal/log"
	"github.com/ghostlawless/xdl/internal/scraper"
	"github.com/ghostlawless/xdl/internal/utils"
)

func newSpinnerForUser(_ RunContext, label string) *spinner {
	return startSpinner(label)
}

func stopSpinner(s *spinner) {
	if s != nil {
		s.Stop()
	}
}

func prepareRunOutputDir(r0 RunContext, _ *config.EssentialsConfig, u0 string, _ *spinner) (string, error) {
	n0 := u0
	p0 := filepath.Join(r0.OutRoot, n0)

	if e0 := utils.EnsureDir(r0.OutRoot); e0 != nil {
		return "", e0
	}

	if utils.DirExists(p0) {
		i0 := 1
		for {
			n1 := fmt.Sprintf("%s_%03d", u0, i0)
			p1 := filepath.Join(r0.OutRoot, n1)
			if !utils.DirExists(p1) {
				p0 = p1
				break
			}
			i0++
			if i0 > 9999 {
				return "", fmt.Errorf("Could not create a new output folder for @%s (too many existing runs).", u0)
			}
		}
	}

	if e1 := utils.EnsureDir(p0); e1 != nil {
		return "", e1
	}

	if r0.Mode == ModeVerbose {
		utils.PrintInfo("Output folder: %s", p0)
	}

	return p0, nil
}

func resolveUserID(r0 RunContext, c0 *config.EssentialsConfig, h0 *http.Client, u0 string, _ *spinner) (string, error) {
	i0, e0 := scraper.FetchUserID(h0, c0, u0)
	if e0 != nil {
		log.LogError("user", e0.Error())

		if r0.Mode == ModeDebug {
			return "", fmt.Errorf("user lookup failed for @%s: %w", u0, e0)
		}

		return "", fmt.Errorf(
			"Could not load @%s.\n\nFix:\n  1) Make sure you are logged in to x.com in your browser\n  2) Export cookies as JSON and save to config/cookies.json\n  3) Run xdl again\n\nTip: run with -d to generate logs.",
			u0,
		)
	}

	if r0.Mode == ModeDebug {
		log.LogInfo("user", "["+i0+"]")
	}

	return i0, nil
}

func printRunSummary(r0 RunContext, u0 string, t0 time.Time, s0 scanResult, d0 downloadStats) {
	if r0.Mode == ModeDebug {
		log.LogInfo("media", fmt.Sprintf(
			"media found: %d (images:%d videos:%d)",
			s0.TotalMedia, s0.TotalImages, s0.TotalVideos,
		))
		log.LogInfo("download", fmt.Sprintf(
			"done: ok=%d skipped=%d failed=%d bytes=%d",
			d0.Downloaded, d0.Skipped, d0.Failed, d0.Bytes,
		))
		log.LogInfo("main", fmt.Sprintf(
			"xdl[%s] exit [%.2fs] user=%s",
			r0.RunID, time.Since(t0).Seconds(), u0,
		))
		return
	}

	if r0.Mode == ModeVerbose {
		mb := float64(d0.Bytes) / 1024.0 / 1024.0
		utils.PrintSuccess(
			"Done @%s — ok:%d skip:%d fail:%d (%.2f MB, %.2fs)",
			u0, d0.Downloaded, d0.Skipped, d0.Failed, mb, time.Since(t0).Seconds(),
		)
	}
}

var termMu sync.Mutex

type interactiveControl struct{}

func (c *interactiveControl) ShouldPause() bool { return false }
func (c *interactiveControl) ShouldQuit() bool  { return false }
func (c *interactiveControl) setPaused(bool)    {}
func (c *interactiveControl) setQuit()          {}

var globalControl = &interactiveControl{}

func startKeyboardControlListener(_ *interactiveControl) {}

type spinner struct {
	label   string
	stopCh  chan struct{}
	wg      sync.WaitGroup
	lastLen int
}

func startSpinner(label string) *spinner {
	s := &spinner{
		label:  label,
		stopCh: make(chan struct{}),
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		frames := []rune{'-', '\\', '|', '/'}
		i := 0
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				out := fmt.Sprintf("%s %c", s.label, frames[i%len(frames)])
				s.lastLen = len(out)
				fmt.Printf("\r%s", out)
				i++
			}
		}
	}()
	return s
}

func (s *spinner) Stop() {
	if s == nil {
		return
	}
	close(s.stopCh)
	s.wg.Wait()
	fmt.Printf("\r%s\r", strings.Repeat(" ", s.lastLen))
}

func buildProgressBar(width int, fraction float64) string {
	if width <= 0 {
		width = 20
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	filled := int(float64(width)*fraction + 0.5)
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	b := make([]byte, width)
	for i := 0; i < width; i++ {
		if i < filled {
			b[i] = '='
		} else {
			b[i] = ' '
		}
	}
	return string(b)
}
