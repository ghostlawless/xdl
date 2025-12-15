package app

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ghostlawless/xdl/internal/config"
	"github.com/ghostlawless/xdl/internal/log"
	"github.com/ghostlawless/xdl/internal/runtime"
	"github.com/ghostlawless/xdl/internal/utils"
)

func runWithContext(r0 RunContext) error {
	_ = context.Background()

	if r0.Mode == ModeVerbose {
		utils.PrintBanner()
	}

	startKeyboardControlListener(globalControl)

	p0 := []string{
		filepath.Join(".", "config", "essentials.json"),
		filepath.Join(".", "essentials.json"),
	}

	c0, e0 := config.LoadEssentialsWithFallback(p0)
	if e0 != nil {
		log.LogError("config", "failed to load essentials: "+e0.Error())
		return e0
	}

	if r0.Mode == ModeDebug {
		c0.Paths.Debug = r0.LogPath
		c0.Paths.DebugRaw = r0.LogPath
	}

	k0 := strings.TrimSpace(r0.CookiePath)
	m0 := strings.TrimSpace(c0.Auth.Cookies.AuthToken) == "" || strings.TrimSpace(c0.Auth.Cookies.Ct0) == ""

	if k0 != "" || m0 {
		e1 := config.ApplyCookiesFromFile(c0, k0)
		if e1 != nil {
			log.LogError("config", "cookie setup failed: "+e1.Error())
			return e1
		}

		if r0.Mode == ModeDebug {
			g0 := c0.Auth.Cookies.GuestID != ""
			g1 := c0.Auth.Cookies.AuthToken != ""
			g2 := c0.Auth.Cookies.Ct0 != ""
			log.LogInfo("config", fmt.Sprintf("cookies loaded: guest_id=%v auth_token=%v ct0=%v", g0, g1, g2))
		}
	}

	e2 := c0.ValidateRequiredCookies(k0)
	if e2 != nil {
		log.LogError("config", "missing auth cookies: "+e2.Error())
		return e2
	}

	t0 := c0.HTTPTimeout()
	h0 := buildAPIClient(t0)
	h1 := buildDownloadClient()

	if len(r0.Users) == 1 {
		return runSingleUser(r0, c0, h0, h1, r0.Users[0])
	}

	n0 := len(r0.Users)
	if n0 > 4 {
		n0 = 4
	}

	q0 := make(chan error, len(r0.Users))
	s1 := make(chan struct{}, n0)

	var w0 sync.WaitGroup
	for _, u0 := range r0.Users {
		u1 := u0
		w0.Add(1)
		go func() {
			defer w0.Done()
			s1 <- struct{}{}
			defer func() { <-s1 }()

			if e3 := runSingleUser(r0, c0, h0, h1, u1); e3 != nil {
				q0 <- fmt.Errorf("@%s: %w", u1, e3)
			}
		}()
	}

	w0.Wait()
	close(q0)

	for e4 := range q0 {
		if e4 != nil {
			return e4
		}
	}

	return nil

}
func runSingleUser(r0 RunContext, c0 *config.EssentialsConfig, h0, h1 *http.Client, u0 string) error {
	t0 := time.Now()
	l0 := runtime.NewLimiterWith(r0.RunSeed, []byte(strings.TrimSpace(c0.Runtime.LimiterSecret)))

	if r0.Mode == ModeDebug {
		log.LogInfo("main", fmt.Sprintf("xdl start | run_id=%s | target=%s", r0.RunID, u0))
	}
	if r0.Mode == ModeVerbose {
		utils.PrintInfo("Loading target profile: @%s", u0)
	}

	s0 := newSpinnerForUser(r0, u0)
	if s0 != nil {
		defer stopSpinner(s0)
	}

	d0, e0 := prepareRunOutputDir(r0, c0, u0, s0)
	if e0 != nil {
		return e0
	}

	i0, e1 := resolveUserID(r0, c0, h0, u0, s0)
	if e1 != nil {
		return e1
	}

	a0, b0, e2 := scanAndDownloadUserMedia(r0, c0, h0, h1, i0, u0, d0, l0)
	if e2 != nil {
		return e2
	}

	printRunSummary(r0, u0, t0, a0, b0)
	return nil

}
