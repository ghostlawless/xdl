package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ghostlawless/xdl/internal/config"
	"github.com/ghostlawless/xdl/internal/downloader"
	"github.com/ghostlawless/xdl/internal/log"
	"github.com/ghostlawless/xdl/internal/scraper"
	"github.com/ghostlawless/xdl/internal/utils"
)

type RunMode int

const (
	ModeVerbose RunMode = iota
	ModeQuiet
	ModeDebug
)

type RunContext struct {
	User       string
	Mode       RunMode
	RunID      string
	LogPath    string
	CookiePath string
	OutRoot    string
	NoDownload bool
	DryRun     bool
}

func generateRunID() string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func buildAPIClient(timeout time.Duration) *http.Client {
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

func buildDownloadClient() *http.Client {
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       120 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   7 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	return &http.Client{Transport: tr, Timeout: 0}
}

func parseArgs(args []string, presetRunID string) (RunContext, error) {
	var (
		fQuiet      bool
		fDebug      bool
		fCookiePath string
	)

	for _, a := range args {
		switch a {
		case "-q", "/q":
			fQuiet = true
		case "-d", "/d":
			fDebug = true
		}
	}

	fs := flag.NewFlagSet("xdl", flag.ContinueOnError)
	fs.BoolVar(&fQuiet, "q", fQuiet, "Quiet mode")
	fs.BoolVar(&fDebug, "d", fDebug, "Debug mode")
	fs.StringVar(&fCookiePath, "c", "", "Cookie JSON file exported from browser extension")

	if err := fs.Parse(args); err != nil {
		return RunContext{}, err
	}

	rest := fs.Args()
	if len(rest) == 0 || rest[0] == "" {
		return RunContext{}, fmt.Errorf("usage: xdl [-q|-d] -c cookies.json <username>")
	}

	ctx := RunContext{
		User:       rest[0],
		Mode:       ModeVerbose,
		RunID:      presetRunID,
		CookiePath: fCookiePath,
		OutRoot:    "xDownloads",
		NoDownload: false,
		DryRun:     false,
	}

	if fDebug {
		ctx.Mode = ModeDebug
	} else if fQuiet {
		ctx.Mode = ModeQuiet
	}

	if ctx.RunID == "" {
		ctx.RunID = generateRunID()
	}

	if ctx.Mode == ModeDebug {
		ctx.LogPath = filepath.Join("logs", "run_"+ctx.RunID)
		if err := os.MkdirAll(ctx.LogPath, 0o755); err != nil {
			return RunContext{}, fmt.Errorf("failed to create log dir: %w", err)
		}
		log.Init(filepath.Join(ctx.LogPath, "main.log"))
		log.LogInfo("main", "Debug mode enabled; logs stored in "+ctx.LogPath)
	} else {
		log.Disable()
	}

	return ctx, nil
}

func RunWithArgsAndID(args []string, runID string) error {
	rctx, err := parseArgs(args, runID)
	if err != nil {
		return err
	}
	return runWithContext(rctx)
}

func RunWithArgs(args []string) error { return RunWithArgsAndID(args, "") }

func Run() {
	if err := RunWithArgsAndID(os.Args[1:], ""); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
	}
}

func runWithContext(rctx RunContext) error {
	conf, err := config.LoadEssentialsWithFallback([]string{
		filepath.Join(".", "config", "essentials.json"),
		filepath.Join(".", "essentials.json"),
	})
	if err != nil {
		if rctx.Mode == ModeVerbose {
			fmt.Fprintln(os.Stderr, "xdl error: failed to load essentials:", err)
		}
		log.LogError("config", "failed to load essentials: "+err.Error())
		return err
	}

	if rctx.Mode == ModeDebug {
		conf.Paths.Debug = rctx.LogPath
		conf.Paths.DebugRaw = rctx.LogPath
	}

	if rctx.CookiePath != "" {
		if err := config.ApplyCookiesFromFile(conf, rctx.CookiePath); err != nil {
			if rctx.Mode == ModeVerbose {
				fmt.Fprintln(os.Stderr, "xdl error: failed to apply cookies:", err)
			}
			log.LogError("config", "failed to apply cookies: "+err.Error())
			return err
		}
		if rctx.Mode == ModeDebug {
			hasGuest := conf.Auth.Cookies.GuestID != ""
			hasAuth := conf.Auth.Cookies.AuthToken != ""
			hasCt0 := conf.Auth.Cookies.Ct0 != ""
			log.LogInfo("config", fmt.Sprintf("cookies loaded: guest_id=%v auth_token=%v ct0=%v", hasGuest, hasAuth, hasCt0))
		}
	} else if rctx.Mode == ModeVerbose {
		fmt.Fprintln(os.Stderr, "xdl warning: no cookie file provided (-c). Requests may fail with 403.")
	}

	apiTimeout := conf.HTTPTimeout()
	apiClient := buildAPIClient(apiTimeout)
	dlClient := buildDownloadClient()

	start := time.Now()
	if rctx.Mode == ModeDebug {
		log.LogInfo("main", fmt.Sprintf("xdl start | run_id=%s | target=%s", rctx.RunID, rctx.User))
	} else if rctx.Mode == ModeVerbose {
		fmt.Printf("xdl: starting for @%s\n", rctx.User)
	}

	uid, err := scraper.FetchUserID(apiClient, conf, rctx.User)
	if err != nil {
		if rctx.Mode == ModeVerbose {
			fmt.Fprintln(os.Stderr, "xdl error: user lookup failed:", err)
		}
		log.LogError("user", err.Error())
		return err
	}
	if rctx.Mode == ModeDebug {
		log.LogInfo("user", "["+uid+"]")
	} else if rctx.Mode == ModeVerbose {
		fmt.Println("xdl: user id:", uid)
	}

	links, err := scraper.GetMediaLinksForUser(apiClient, conf, uid)
	if err != nil && rctx.Mode == ModeVerbose {
		fmt.Fprintln(os.Stderr, "xdl warning: media listing error:", err)
	}
	if rctx.Mode == ModeDebug {
		log.LogInfo("media", fmt.Sprintf("media found: %d", len(links)))
	} else if rctx.Mode == ModeVerbose {
		fmt.Printf("xdl: %d media found for @%s\n", len(links), rctx.User)
	}

	runDirName := fmt.Sprintf("xDownload - %s@%s", rctx.User, rctx.RunID)
	runDir := filepath.Join(rctx.OutRoot, runDirName)
	if err := utils.EnsureDir(rctx.OutRoot); err != nil {
		return err
	}
	if utils.DirExists(runDir) {
		proceed := true
		if rctx.Mode != ModeQuiet {
			proceed = utils.PromptYesNoDefaultYes(fmt.Sprintf("xdl: folder '%s' exists. Overwrite files? [Y/n]: ", runDir))
		}
		if !proceed {
			if rctx.Mode == ModeVerbose {
				fmt.Println("xdl: aborted by user.")
			}
			return nil
		}
	} else if err := utils.EnsureDir(runDir); err != nil {
		return err
	}

	if !rctx.NoDownload {
		if rctx.Mode == ModeVerbose {
			fmt.Printf("xdl: downloading to %s ...\n", runDir)
		}
		summary, derr := downloader.DownloadAllCycles(dlClient, conf, links, downloader.Options{
			RunDir:            runDir,
			User:              rctx.User,
			PreflightWC:       16,
			MediaMaxBytes:     0,
			DryRun:            rctx.DryRun,
			SortBySizeDesc:    true,
			Attempts:          3,
			PerAttemptTimeout: 2 * time.Minute,
		})
		if rctx.Mode == ModeDebug {
			log.LogInfo("download", fmt.Sprintf("done: ok=%d skipped=%d failed=%d bytes=%d cycles=%d",
				summary.Downloaded, summary.Skipped, summary.Failed, summary.TotalBytes, summary.Cycles))
			log.LogInfo("main", fmt.Sprintf("xdl[%s] exit [%.2fs]", rctx.RunID, time.Since(start).Seconds()))
		} else if rctx.Mode == ModeVerbose {
			fmt.Printf("xdl: downloaded %d, skipped %d, failed %d, total %0.2f MB, cycles %d\n",
				summary.Downloaded, summary.Skipped, summary.Failed, float64(summary.TotalBytes)/1024.0/1024.0, summary.Cycles)
			fmt.Printf("xdl: done in %.2fs\n", time.Since(start).Seconds())
		}
		if derr != nil {
			log.LogError("download", derr.Error())
		}
	}

	_ = context.Background()
	return nil
}
