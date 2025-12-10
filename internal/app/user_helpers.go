package app

import (
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/ghostlawless/xdl/internal/config"
	"github.com/ghostlawless/xdl/internal/log"
	"github.com/ghostlawless/xdl/internal/scraper"
	"github.com/ghostlawless/xdl/internal/utils"
)

func newSpinnerForUser(rctx RunContext, username string) *spinner {
	if rctx.Mode == ModeQuiet {
		return nil
	}

	label := fmt.Sprintf("scanning media for target @%s", username)

	if rctx.Mode == ModeVerbose {
		utils.PrintInfo("%s", label)
	}

	return startSpinner(label)
}

func stopSpinner(spin *spinner) {
	if spin == nil {
		return
	}
	spin.Stop()
}

func prepareRunOutputDir(rctx RunContext, conf *config.EssentialsConfig, username string, spin *spinner) (string, error) {
	_ = conf
	_ = spin

	if rctx.NoDownload {
		return "", nil
	}

	runDirName := username
	runDir := filepath.Join(rctx.OutRoot, runDirName)

	if err := utils.EnsureDir(rctx.OutRoot); err != nil {
		return "", err
	}

	if utils.DirExists(runDir) {
		i := 1
		for {
			candidateName := fmt.Sprintf("%s_%03d", username, i)
			candidatePath := filepath.Join(rctx.OutRoot, candidateName)
			if !utils.DirExists(candidatePath) {
				runDir = candidatePath
				break
			}
			i++
			if i > 9999 {
				return "", fmt.Errorf("failed to allocate output folder for @%s", username)
			}
		}
	}

	if err := utils.EnsureDir(runDir); err != nil {
		return "", err
	}

	if rctx.Mode == ModeVerbose {
		utils.PrintInfo("output: %s", runDir)
	}

	return runDir, nil
}

func resolveUserID(rctx RunContext, conf *config.EssentialsConfig, apiClient *http.Client, username string, spin *spinner) (string, error) {
	_ = conf
	_ = spin

	uid, err := scraper.FetchUserID(apiClient, conf, username)
	if err != nil {
		if rctx.Mode == ModeVerbose {
			utils.PrintError("user lookup failed for @%s: %v", username, err)
		}
		log.LogError("user", err.Error())
		return "", err
	}

	if rctx.Mode == ModeDebug {
		log.LogInfo("user", "["+uid+"]")
	}

	return uid, nil
}

func printRunSummary(rctx RunContext, username string, start time.Time, scan scanResult, stats downloadStats) {
	if rctx.Mode == ModeDebug {
		log.LogInfo("media", fmt.Sprintf(
			"media found: %d (images:%d videos:%d)",
			scan.TotalMedia, scan.TotalImages, scan.TotalVideos,
		))
		log.LogInfo("download", fmt.Sprintf(
			"done: ok=%d skipped=%d failed=%d bytes=%d",
			stats.Downloaded, stats.Skipped, stats.Failed, stats.Bytes,
		))
		log.LogInfo("main", fmt.Sprintf(
			"xdl[%s] exit [%.2fs] user=%s",
			rctx.RunID, time.Since(start).Seconds(), username,
		))
	} else if rctx.Mode == ModeVerbose {
		if !rctx.NoDownload {
			totalMB := float64(stats.Bytes) / 1024.0 / 1024.0
			utils.PrintSuccess(
				"complete @%s — ok:%d skip:%d fail:%d (%.2f MB, %.2fs)",
				username, stats.Downloaded, stats.Skipped, stats.Failed, totalMB, time.Since(start).Seconds(),
			)
		} else {
			utils.PrintSuccess(
				"scan complete @%s — media:%d (images:%d videos:%d)",
				username, scan.TotalMedia, scan.TotalImages, scan.TotalVideos,
			)
		}
	}
}
