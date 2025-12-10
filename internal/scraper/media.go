package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/ghostlawless/xdl/internal/config"
	"github.com/ghostlawless/xdl/internal/httpx"
	"github.com/ghostlawless/xdl/internal/log"
	xruntime "github.com/ghostlawless/xdl/internal/runtime"
	"github.com/ghostlawless/xdl/internal/utils"
)

type Media struct {
	URL     string `json:"url"`
	Type    string `json:"type"` // "image" or "video"
	TweetID string `json:"tweet_id,omitempty"`
}

type PageHandler func(page int, cursor string, medias []Media) error

func WalkUserMediaPages(
	cl *http.Client,
	cf *config.EssentialsConfig,
	uid string,
	sn string,
	vb bool,
	lim *xruntime.Limiter,
	handler PageHandler,
) error {
	if cl == nil || cf == nil {
		return errors.New("nil client or config")
	}
	if uid == "" {
		return errors.New("empty userID")
	}

	// Local helper: tries to find "media_count" anywhere in the JSON.
	extractCount := func(b []byte) int {
		var root any
		if err := json.Unmarshal(b, &root); err != nil {
			return -1
		}

		var walk func(v any) int
		walk = func(v any) int {
			switch t := v.(type) {
			case map[string]any:
				if mc, ok := t["media_count"]; ok {
					switch vv := mc.(type) {
					case float64:
						if vv >= 0 {
							return int(vv)
						}
					case int:
						if vv >= 0 {
							return vv
						}
					}
				}
				for _, v2 := range t {
					if got := walk(v2); got >= 0 {
						return got
					}
				}
			case []any:
				for _, it := range t {
					if got := walk(it); got >= 0 {
						return got
					}
				}
			}
			return -1
		}

		return walk(root)
	}

	ep, err := cf.GraphQLURL("user_media")
	if err != nil {
		return err
	}

	cur := ""
	pg := 1
	stg := 0
	const mx = 200

	seenCursors := make(map[string]struct{}, 256)
	seenCursors[""] = struct{}{}

	// Global dedupe of media URLs across all pages.
	seenMedia := make(map[string]struct{}, 1024)

	ic := 0
	vc := 0
	ri := 0
	ref := strings.TrimRight(cf.X.Network, "/") + "/i/user/" + uid + "/media"

	end := ""

	// Total media reported by the server (media_count), when available.
	totalExpected := -1
	printedScan := false

	for {
		ri++
		if lim != nil {
			lim.SleepBeforeRequest(context.Background(), sn, pg, ri)
		}

		vars := map[string]any{
			"userId":                 uid,
			"count":                  100,
			"includePromotedContent": false,
			"withClientEventToken":   false,
			"withVoice":              false,
		}
		if cur != "" {
			vars["cursor"] = cur
		}

		vj, err := json.Marshal(vars)
		if cf.Runtime.DebugEnabled && err != nil {
			return fmt.Errorf("marshal variables: %w", err)
		}
		fj, err := cf.FeatureJSONFor("user_media")
		if cf.Runtime.DebugEnabled && err != nil {
			return fmt.Errorf("get features for user_media: %w", err)
		}

		q := fmt.Sprintf("%s?variables=%s&features=%s",
			ep,
			url.QueryEscape(string(vj)),
			url.QueryEscape(fj),
		)

		rq, gerr := http.NewRequest(http.MethodGet, q, nil)
		if gerr != nil {
			return fmt.Errorf("build request: %w", gerr)
		}
		cf.BuildRequestHeaders(rq, ref)
		rq.Header.Set("Accept", "application/json, */*;q=0.1")

		b, st, reqErr := httpx.DoRequestWithOptions(cl, rq, httpx.RequestOptions{
			MaxBytes: 8 << 20,
			Decode:   true,
			Accept:   func(s int) bool { return s >= 200 && s < 300 },
		})
		if reqErr != nil {
			if cf.Runtime.DebugEnabled {
				p, _ := utils.SaveTimestamped(cf.Paths.Debug, "err_user_media", "json", b)
				meta := fmt.Sprintf(
					"METHOD: GET\nSTATUS: %d\nURL: %s\nPAGE: %d\nCURSOR: %s\n",
					st, q, pg, cur,
				)
				_, _ = utils.SaveTimestamped(cf.Paths.Debug, "err_user_media_meta", "txt", []byte(meta))
				log.LogError("media", fmt.Sprintf("UserMedia failed (status %d). see: %s", st, p))
			} else {
				log.LogError("media", fmt.Sprintf("UserMedia failed (status %d). run with -d for details.", st))
			}
			end = "http_error"
			break
		}

		if cf.Runtime.DebugEnabled {
			fname := fmt.Sprintf("user_media_page_%03d", pg)
			p, _ := utils.SaveTimestamped(cf.Paths.Debug, fname, "json", b)
			log.LogInfo("media", fmt.Sprintf("saved UserMedia page %d to %s", pg, p))
		}

		// Try to read media_count once, from the first successful page.
		if totalExpected < 0 {
			if cnt := extractCount(b); cnt > 0 {
				totalExpected = cnt
				if cf.Runtime.DebugEnabled {
					log.LogInfo("media", fmt.Sprintf("server-reported media_count=%d", totalExpected))
				}
			}
		}

		pms, jerr := fold(b)
		if jerr != nil {
			if cf.Runtime.DebugEnabled {
				p, _ := utils.SaveTimestamped(cf.Paths.Debug, "err_user_media_parse", "json", b)
				meta := fmt.Sprintf("PARSE_ERROR: %v\nPAGE: %d\nCURSOR: %s\n", jerr, pg, cur)
				_, _ = utils.SaveTimestamped(cf.Paths.Debug, "err_user_media_parse_meta", "txt", []byte(meta))
				log.LogError("media", fmt.Sprintf("parse page %d failed. see: %s", pg, p))
			} else {
				log.LogError("media", fmt.Sprintf("parse page %d failed.", pg))
			}
			end = "parse_error"
			break
		}

		// Keep only new media for this page (global dedupe by URL).
		pageBatch := make([]Media, 0, len(pms))
		for _, m := range pms {
			if m.URL == "" {
				continue
			}
			if _, dup := seenMedia[m.URL]; dup {
				continue
			}
			seenMedia[m.URL] = struct{}{}
			pageBatch = append(pageBatch, m)
			if m.Type == "image" {
				ic++
			} else if m.Type == "video" {
				vc++
			}
		}

		total := len(seenMedia)
		if cf.Runtime.DebugEnabled {
			delta := len(pageBatch)
			log.LogInfo("media", fmt.Sprintf("page %d: +%d (total %d)", pg, delta, total))
		}

		// Verbose scan progress (single user): one line with bar + percent when possible.
		if vb {
			line := ""
			if totalExpected > 0 {
				f := float64(total) / float64(totalExpected)
				if f < 0 {
					f = 0
				}
				if f > 1 {
					f = 1
				}
				bar := buildScanProgressBar(30, f)
				pct := f * 100.0
				line = fmt.Sprintf(
					"xdl ▸ [scan] @%s [page:%d] %s %3.0f%% (%d/%d)",
					sn,
					pg,
					bar,
					pct,
					total,
					totalExpected,
				)
			} else {
				line = fmt.Sprintf(
					"xdl ▸ [scan] @%s [page:%d] images:%d videos:%d (total:%d)",
					sn,
					pg,
					ic,
					vc,
					total,
				)
			}
			fmt.Printf("\r\033[2K%s", line)
			printedScan = true
		}

		if handler != nil && len(pageBatch) > 0 {
			if err := handler(pg, cur, pageBatch); err != nil {
				return err
			}
		}

		if len(pageBatch) == 0 {
			stg++
		} else {
			stg = 0
		}

		if stg >= 3 {
			log.LogInfo("media", "no progress for 3 pages — stopping")
			end = "no_progress"
			break
		}

		nx := next(b)
		if nx == "" {
			log.LogInfo("media", "no next cursor — reached end of timeline")
			end = "no_next_cursor"
			break
		}
		if _, dup := seenCursors[nx]; dup {
			log.LogInfo("media", "repeated cursor detected — stopping")
			end = "repeat_cursor"
			break
		}
		seenCursors[nx] = struct{}{}

		if pg >= mx {
			log.LogInfo("media", fmt.Sprintf("max pages reached (%d) — stopping", mx))
			end = "max_pages"
			break
		}

		cur = nx
		pg++
	}

	if vb && printedScan {
		// Finish the scan line before download progress prints more things.
		fmt.Print("\n")
	}

	if end == "no_progress" || end == "no_next_cursor" || end == "repeat_cursor" || end == "max_pages" {
		log.LogInfo("media", fmt.Sprintf(
			"UserMedia endpoint reached its server-side end at page %d. This feed may expose fewer items than the media counter shown in the profile UI.",
			pg,
		))
	}

	return nil
}

func buildScanProgressBar(width int, fraction float64) string {
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

func GetMediaLinksForUser(cl *http.Client, cf *config.EssentialsConfig, uid string, sn string, vb bool, lim *xruntime.Limiter) ([]Media, error) {
	if cl == nil || cf == nil {
		return nil, errors.New("nil client or config")
	}
	if uid == "" {
		return nil, errors.New("empty userID")
	}

	all := make([]Media, 0, 512)

	handler := func(page int, cursor string, medias []Media) error {
		all = append(all, medias...)
		return nil
	}

	if err := WalkUserMediaPages(cl, cf, uid, sn, vb, lim, handler); err != nil {
		return nil, err
	}

	if len(all) == 0 {
		log.LogInfo("media", "Total unique media found: 0")
		return all, nil
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].URL < all[j].URL
	})

	log.LogInfo("media", fmt.Sprintf("Total unique media found: %d", len(all)))

	return all, nil
}
