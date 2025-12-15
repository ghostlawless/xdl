package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ghostlawless/xdl/internal/config"
	"github.com/ghostlawless/xdl/internal/httpx"
	"github.com/ghostlawless/xdl/internal/log"
	xruntime "github.com/ghostlawless/xdl/internal/runtime"
	"github.com/ghostlawless/xdl/internal/utils"
)

func EnrichMediaWithTweetDetail(
	cl *http.Client,
	cf *config.EssentialsConfig,
	screenName string,
	medias []Media,
	lim *xruntime.Limiter,
	vb bool,
) []Media {
	if cl == nil || cf == nil {
		return medias
	}

	ep, err := cf.GraphQLURL("tweet_detail")
	if err != nil || ep == "" {
		if cf.Runtime.DebugEnabled {
			log.LogError("media", fmt.Sprintf("TweetDetail GraphQL endpoint not configured: %v", err))
		}
		return medias
	}

	tweetIndex := make(map[string][]int)
	for i, m := range medias {
		if m.TweetID == "" {
			continue
		}
		tweetIndex[m.TweetID] = append(tweetIndex[m.TweetID], i)
	}

	totalTweets := len(tweetIndex)
	if totalTweets == 0 {
		return medias
	}

	if cf.Runtime.DebugEnabled {
		log.LogInfo("media", fmt.Sprintf("TweetDetail enrichment for %d tweet(s)", totalTweets))
	}

	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	out := make([]Media, len(medias))
	copy(out, medias)

	idx := 0
	attempted := 0
	successTweets := 0
	httpErrors := 0
	parseErrors := 0
	noMediaFound := 0
	updatedImages := 0
	updatedVideos := 0

	for tid, positions := range tweetIndex {
		idx++
		attempted++

		if lim != nil {
			lim.SleepBeforeRequest(context.Background(), screenName+"_tweetdetail", 0, idx)
		}

		base := 80
		extra := rnd.Intn(120)
		step := (idx % 5) * 20
		jitterMs := base + extra + step
		time.Sleep(time.Duration(jitterMs) * time.Millisecond)

		if cf.Runtime.DebugEnabled && idx%25 == 0 {
			log.LogInfo("media", fmt.Sprintf(
				"TweetDetail progress: %d/%d (success=%d http_err=%d parse_err=%d no_media=%d)",
				idx, totalTweets, successTweets, httpErrors, parseErrors, noMediaFound,
			))
		}

		vars := map[string]any{
			"focalTweetId":                           tid,
			"with_rux_injections":                    false,
			"includePromotedContent":                 false,
			"withCommunity":                          false,
			"withQuickPromoteEligibilityTweetFields": false,
			"withBirdwatchNotes":                     false,
			"withVoice":                              false,
			"withV2Timeline":                         true,
		}

		vj, _ := json.Marshal(vars)
		fj, _ := cf.FeatureJSONFor("tweet_detail")

		q := fmt.Sprintf("%s?variables=%s", ep, url.QueryEscape(string(vj)))
		if fj != "" {
			q = fmt.Sprintf("%s&features=%s", q, url.QueryEscape(fj))
		}

		ref := strings.TrimRight(cf.X.Network, "/") + "/" + screenName + "/status/" + tid

		req, rerr := http.NewRequest(http.MethodGet, q, nil)
		if rerr != nil {
			httpErrors++
			if cf.Runtime.DebugEnabled {
				log.LogError("media", fmt.Sprintf("TweetDetail build request failed for %s: %v", tid, rerr))
			}
			continue
		}
		cf.BuildRequestHeaders(req, ref)
		req.Header.Set("Accept", "application/json, */*;q=0.1")

		b, st, herr := httpx.DoRequestWithOptions(cl, req, httpx.RequestOptions{
			MaxBytes: 8 << 20,
			Decode:   true,
			Accept:   func(s int) bool { return s >= 200 && s < 300 },
		})
		if herr != nil {
			httpErrors++
			if cf.Runtime.DebugEnabled {
				p, _ := utils.SaveTimestamped(cf.Paths.Debug, "err_tweet_detail", "json", b)
				meta := fmt.Sprintf(
					"METHOD: GET\nSTATUS: %d\nTWEET_ID: %s\nURL: %s\n",
					st, tid, q,
				)
				_, _ = utils.SaveTimestamped(cf.Paths.Debug, "err_tweet_detail_meta", "txt", []byte(meta))
				log.LogError("media", fmt.Sprintf("TweetDetail failed for %s (status %d). see: %s", tid, st, p))
			} else {
				log.LogError("media", fmt.Sprintf("TweetDetail failed for %s (status %d).", tid, st))
			}
			continue
		}

		pageMedia, perr := fold(b)
		if perr != nil {
			parseErrors++
			if cf.Runtime.DebugEnabled {
				p, _ := utils.SaveTimestamped(cf.Paths.Debug, "err_tweet_detail_parse", "json", b)
				meta := fmt.Sprintf("PARSE_ERROR: %v\nTWEET_ID: %s\n", perr, tid)
				_, _ = utils.SaveTimestamped(cf.Paths.Debug, "err_tweet_detail_parse_meta", "txt", []byte(meta))
				log.LogError("media", fmt.Sprintf("parse TweetDetail for %s failed. see: %s", tid, p))
			} else {
				log.LogError("media", fmt.Sprintf("parse TweetDetail for %s failed: %v", tid, perr))
			}
			continue
		}

		filtered := make([]Media, 0, len(pageMedia))
		for _, m := range pageMedia {
			if m.TweetID == "" || m.TweetID == tid {
				filtered = append(filtered, m)
			}
		}

		tdImages := make([]Media, 0, len(filtered))
		tdVideos := make([]Media, 0, len(filtered))
		for _, m := range filtered {
			switch m.Type {
			case "image":
				tdImages = append(tdImages, m)
			case "video":
				tdVideos = append(tdVideos, m)
			}
		}

		if len(tdImages) == 0 && len(tdVideos) == 0 {
			noMediaFound++
			continue
		}

		origImgIdx := make([]int, 0, len(positions))
		origVidIdx := make([]int, 0, len(positions))
		for _, pos := range positions {
			if pos < 0 || pos >= len(out) {
				continue
			}
			switch out[pos].Type {
			case "image":
				origImgIdx = append(origImgIdx, pos)
			case "video":
				origVidIdx = append(origVidIdx, pos)
			}
		}

		updatedThisTweet := false

		for i := 0; i < len(origImgIdx) && i < len(tdImages); i++ {
			pos := origImgIdx[i]
			nu := tdImages[i].URL
			if nu == "" || nu == out[pos].URL {
				continue
			}
			out[pos].URL = nu
			updatedImages++
			updatedThisTweet = true
		}

		for i := 0; i < len(origVidIdx) && i < len(tdVideos); i++ {
			pos := origVidIdx[i]
			nu := tdVideos[i].URL
			if nu == "" || nu == out[pos].URL {
				continue
			}
			out[pos].URL = nu
			updatedVideos++
			updatedThisTweet = true
		}

		if updatedThisTweet {
			successTweets++
		} else {
			noMediaFound++
		}
	}

	if cf.Runtime.DebugEnabled {
		log.LogInfo("media", fmt.Sprintf(
			"TweetDetail enrichment summary: tweets=%d attempted=%d success=%d no_media=%d http_errors=%d parse_errors=%d updated_images=%d updated_videos=%d",
			totalTweets, attempted, successTweets, noMediaFound, httpErrors, parseErrors, updatedImages, updatedVideos,
		))
	} else if vb && (updatedImages > 0 || updatedVideos > 0) {
		utils.PrintInfo(
			"TweetDetail enrichment updated %d image(s) and %d video(s) from %d tweet(s)",
			updatedImages, updatedVideos, totalTweets,
		)
	}
	return out
}
