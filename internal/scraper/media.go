package scraper

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ghostlawless/xdl/internal/config"
	"github.com/ghostlawless/xdl/internal/httpx"
	"github.com/ghostlawless/xdl/internal/log"
	"github.com/ghostlawless/xdl/internal/utils"
)

type Media struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

func GetMediaLinksForUser(client *http.Client, conf *config.EssentialsConfig, userID string) ([]Media, error) {
	if client == nil || conf == nil {
		return nil, errors.New("nil client or config")
	}
	if userID == "" {
		return nil, errors.New("empty userID")
	}

	endpoint, err := conf.GraphQLURL("user_media")
	if err != nil {
		return nil, err
	}

	all := make(map[string]Media, 512)
	cursor := ""
	page := 1
	stagnant := 0
	const maxPages = 200
	seenCursors := make(map[string]struct{}, 256)
	seenCursors[""] = struct{}{}

	referer := strings.TrimRight(conf.X.Network, "/") + "/i/user/" + userID + "/media"

	for {
		vars := map[string]any{
			"userId":                 userID,
			"count":                  100,
			"includePromotedContent": false,
			"withClientEventToken":   false,
			"withVoice":              false,
		}
		if cursor != "" {
			vars["cursor"] = cursor
		}
		varsJSON, _ := json.Marshal(vars)
		featJSON, _ := conf.FeatureJSONFor("user_media")

		u := fmt.Sprintf("%s?variables=%s&features=%s",
			endpoint, url.QueryEscape(string(varsJSON)), url.QueryEscape(featJSON),
		)
		req, gerr := http.NewRequest(http.MethodGet, u, nil)
		if gerr != nil {
			return nil, fmt.Errorf("build request: %w", gerr)
		}
		conf.BuildRequestHeaders(req, referer)
		req.Header.Set("Accept", "application/json, */*;q=0.1")

		prevTotal := len(all)
		body, status, err := httpx.DoRequestWithOptions(client, req, httpx.RequestOptions{
			MaxBytes: 8 << 20,
			Decode:   true,
			Accept:   func(s int) bool { return s >= 200 && s < 300 },
		})
		if err != nil {
			if conf.Runtime.DebugEnabled {
				bodyPath, _ := utils.SaveTimestamped(conf.Paths.Debug, "err_user_media", "json", body)
				meta := fmt.Sprintf("METHOD: GET\nSTATUS: %d\nURL: %s\nPAGE: %d\nCURSOR: %s\n", status, u, page, cursor)
				_, _ = utils.SaveTimestamped(conf.Paths.Debug, "err_user_media_meta", "txt", []byte(meta))
				log.LogError("media", fmt.Sprintf("UserMedia failed (status %d). see: %s", status, bodyPath))
			} else {
				log.LogError("media", fmt.Sprintf("UserMedia failed (status %d). run with -d for details.", status))
			}
			break
		}

		pageMedias, jerr := extractMediaFromBody(body)
		if jerr != nil {
			if conf.Runtime.DebugEnabled {
				bodyPath, _ := utils.SaveTimestamped(conf.Paths.Debug, "err_user_media_parse", "json", body)
				meta := fmt.Sprintf("PARSE_ERROR: %v\nPAGE: %d\nCURSOR: %s\n", jerr, page, cursor)
				_, _ = utils.SaveTimestamped(conf.Paths.Debug, "err_user_media_parse_meta", "txt", []byte(meta))
				log.LogError("media", fmt.Sprintf("parse page %d failed. see: %s", page, bodyPath))
			} else {
				log.LogError("media", fmt.Sprintf("parse page %d failed.", page))
			}
			break
		}
		for _, m := range pageMedias {
			all[m.URL] = m
		}

		if len(all) == prevTotal {
			stagnant++
		} else {
			stagnant = 0
		}
		if stagnant >= 3 {
			log.LogInfo("media", "no progress for 3 pages — stopping")
			break
		}

		next := extractNextCursor(body)
		if next == "" {
			log.LogInfo("media", "no next cursor — reached end of timeline")
			break
		}
		if _, dup := seenCursors[next]; dup {
			log.LogInfo("media", "repeated cursor detected — stopping")
			break
		}
		seenCursors[next] = struct{}{}

		if page >= maxPages {
			log.LogInfo("media", fmt.Sprintf("max pages reached (%d) — stopping", maxPages))
			break
		}

		cursor = next
		page++
		time.Sleep(300 * time.Millisecond)
	}

	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Media, 0, len(keys))
	for _, k := range keys {
		out = append(out, all[k])
	}
	log.LogInfo("media", fmt.Sprintf("Total unique media found: %d", len(out)))
	return out, nil
}

func extractMediaFromBody(body []byte) ([]Media, error) {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	agg := make(map[string]mediaAgg, 256)
	walkForLegacyMedia(root, agg)

	out := make([]Media, 0, len(agg))
	for _, v := range agg {
		out = append(out, Media{URL: v.URL, Type: v.Type})
	}
	return out, nil
}

type mediaAgg struct {
	URL     string
	Type    string
	Bitrate int
}

func walkForLegacyMedia(v any, agg map[string]mediaAgg) {
	switch t := v.(type) {
	case map[string]any:
		if leg, ok := t["legacy"].(map[string]any); ok {
			collectFromLegacy(leg, agg)
		}
		if _, ok := t["extended_entities"]; ok {
			collectFromLegacy(t, agg)
		}
		for _, vv := range t {
			walkForLegacyMedia(vv, agg)
		}
	case []any:
		for _, it := range t {
			walkForLegacyMedia(it, agg)
		}
	}
}

func collectFromLegacy(node map[string]any, agg map[string]mediaAgg) {
	ee, ok := node["extended_entities"].(map[string]any)
	if !ok {
		return
	}
	arr, ok := ee["media"].([]any)
	if !ok {
		return
	}
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		typ := strings.ToLower(str(m["type"]))
		id := mediaID(m)
		if id == "" {
			id = parseMediaIDFromURL(str(m["media_url_https"]))
		}
		if id == "" {
			continue
		}

		switch typ {
		case "photo":
			u := normalizeImageURL(str(m["media_url_https"]))
			if u == "" {
				continue
			}
			agg[id] = mediaAgg{URL: u, Type: "image"}
		case "video", "animated_gif":
			u, br := bestVideoVariant(m)
			if u == "" {
				continue
			}
			if prev, ok := agg[id]; !ok || br > prev.Bitrate {
				agg[id] = mediaAgg{URL: u, Type: "video", Bitrate: br}
			}
		}
	}
}

func mediaID(m map[string]any) string {
	if s := str(m["id_str"]); s != "" {
		return s
	}
	if s := str(m["media_key"]); s != "" {
		return s
	}
	if f, ok := m["id"].(float64); ok && f > 0 {
		return strconv.FormatInt(int64(f), 10)
	}
	return ""
}

func normalizeImageURL(u string) string {
	if u == "" {
		return ""
	}
	pu, err := url.Parse(u)
	if err != nil {
		return u
	}
	if !strings.Contains(strings.ToLower(pu.Host), "twimg.com") {
		return u
	}
	q := pu.Query()
	format := q.Get("format")
	q = url.Values{}
	if format != "" {
		q.Set("format", format)
	}
	q.Set("name", "orig")
	pu.RawQuery = q.Encode()
	return pu.String()
}

func bestVideoVariant(m map[string]any) (string, int) {
	vi, ok := m["video_info"].(map[string]any)
	if !ok {
		return "", 0
	}
	variants, ok := vi["variants"].([]any)
	if !ok || len(variants) == 0 {
		return "", 0
	}
	bestURL := ""
	bestBR := -1
	for _, it := range variants {
		mv, ok := it.(map[string]any)
		if !ok {
			continue
		}
		ct := strings.ToLower(str(mv["content_type"]))
		if !strings.Contains(ct, "video/mp4") {
			continue
		}
		u := str(mv["url"])
		if u == "" {
			continue
		}
		br := 0
		if f, ok := mv["bitrate"].(float64); ok {
			br = int(f)
		}
		if br > bestBR {
			bestBR = br
			bestURL = u
		}
	}
	if bestURL == "" {
		return "", 0
	}
	return bestURL, bestBR
}

func parseMediaIDFromURL(u string) string {
	if u == "" {
		return ""
	}
	pu, err := url.Parse(u)
	if err != nil {
		return ""
	}
	base := path.Base(pu.Path)
	base = strings.SplitN(base, ".", 2)[0]
	base = strings.TrimSpace(base)
	return base
}

func extractNextCursor(body []byte) string {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return ""
	}
	if v := findBottomCursor(root); v != "" {
		return v
	}
	return findAnyCursor(root)
}

func findBottomCursor(v any) string {
	switch t := v.(type) {
	case map[string]any:
		if strings.EqualFold(str(t["cursorType"]), "Bottom") {
			if val := str(t["value"]); val != "" {
				return val
			}
		}
		for _, vv := range t {
			if got := findBottomCursor(vv); got != "" {
				return got
			}
		}
	case []any:
		for _, it := range t {
			if got := findBottomCursor(it); got != "" {
				return got
			}
		}
	}
	return ""
}

func findAnyCursor(v any) string {
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			if strings.Contains(strings.ToLower(k), "cursor") {
				if s := str(vv); s != "" {
					return s
				}
			}
		}
		for _, vv := range t {
			if got := findAnyCursor(vv); got != "" {
				return got
			}
		}
	case []any:
		for _, it := range t {
			if got := findAnyCursor(it); got != "" {
				return got
			}
		}
	}
	return ""
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
