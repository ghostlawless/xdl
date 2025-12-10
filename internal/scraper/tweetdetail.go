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
	"time"

	"github.com/ghostlawless/xdl/internal/config"
	"github.com/ghostlawless/xdl/internal/log"
	xruntime "github.com/ghostlawless/xdl/internal/runtime"
)

type tweetDetailResponse struct {
	Data struct {
		ThreadedConv struct {
			Instructions []struct {
				Type    string `json:"type"`
				Entries []struct {
					Content struct {
						ItemContent struct {
							TweetResults struct {
								Result *tweetResult `json:"result"`
							} `json:"tweet_results"`
						} `json:"itemContent"`
					} `json:"content"`
				} `json:"entries"`
			} `json:"instructions"`
		} `json:"threaded_conversation_with_injections_v2"`
	} `json:"data"`
}

type tweetResult struct {
	RestID string `json:"rest_id"`
	Legacy struct {
		Entities struct {
			Media []legacyMedia `json:"media"`
		} `json:"entities"`
		ExtendedEntities struct {
			Media []legacyMedia `json:"media"`
		} `json:"extended_entities"`
	} `json:"legacy"`
}

type legacyMedia struct {
	IDStr         string `json:"id_str"`
	MediaURLHTTPS string `json:"media_url_https"`
	Type          string `json:"type"`
	VideoInfo     struct {
		Variants []struct {
			URL         string `json:"url"`
			Bitrate     *int   `json:"bitrate,omitempty"`
			ContentType string `json:"content_type"`
		} `json:"variants"`
	} `json:"video_info"`
}

func GetHighQualityMediaForTweet(
	cl *http.Client,
	cf *config.EssentialsConfig,
	tweetID string,
	vb bool,
	lim *xruntime.Limiter,
) ([]Media, error) {
	if cl == nil || cf == nil {
		return nil, errors.New("nil client or config")
	}
	if tweetID == "" {
		return nil, errors.New("empty tweetID")
	}

	var slept time.Duration
	if lim != nil {
		startSleep := time.Now()
		lim.SleepBeforeRequest(context.Background(), "tweet_detail", 0, 0)
		slept = time.Since(startSleep)
	}

	base, err := cf.GraphQLURL("tweet_detail")
	if err != nil {
		return nil, fmt.Errorf("resolve tweet_detail endpoint: %w", err)
	}

	vars := map[string]any{
		"focalTweetId":                           tweetID,
		"with_rux_injections":                    false,
		"includePromotedContent":                 true,
		"withCommunity":                          true,
		"withQuickPromoteEligibilityTweetFields": true,
		"withBirdwatchNotes":                     true,
		"withVoice":                              true,
	}

	varsJSON, err := json.Marshal(vars)
	if err != nil {
		return nil, fmt.Errorf("marshal variables: %w", err)
	}

	q := url.Values{}
	q.Set("variables", string(varsJSON))
	q.Set("features", "{}")
	q.Set("fieldToggles", "{}")

	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse tweet_detail base url: %w", err)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build TweetDetail request: %w", err)
	}

	reqStart := time.Now()
	resp, err := cl.Do(req)
	reqDur := time.Since(reqStart)
	if err != nil {
		if true {
			log.LogInfo("media", fmt.Sprintf(
				"TweetDetail for %s failed (slept=%s, req_dur=%s, url=%s, err=%v)",
				tweetID, slept, reqDur, u.String(), err,
			))
			fmt.Println("media", fmt.Sprintf("TweetDetail for %s failed (slept=%s, req_dur=%s, url=%s)", tweetID, slept, reqDur.String(), err))
		}
		return nil, fmt.Errorf("do TweetDetail request: %w", err)
	}
	defer resp.Body.Close()

	if true {
		log.LogInfo("media", fmt.Sprintf("TweetDetail for %s -> status=%d, slept=%s, req_dur=%s, url=%s", tweetID, resp.StatusCode, slept, reqDur, u.String()))
		fmt.Println("media", fmt.Sprintf("TweetDetail for %s failed (slept=%s, req_dur=%s, url=%s)", tweetID, slept, reqDur.String(), err))

	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tweet detail http status %d", resp.StatusCode)
	}

	var td tweetDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&td); err != nil {
		return nil, fmt.Errorf("decode TweetDetail: %w", err)
	}

	tweet := firstTweetResult(&td)
	if tweet == nil {
		return nil, errors.New("no tweet result in TweetDetail response")
	}

	return extractBestMediaFromTweet(tweet), nil
}

func firstTweetResult(td *tweetDetailResponse) *tweetResult {
	if td == nil {
		return nil
	}
	ins := td.Data.ThreadedConv.Instructions
	for _, inst := range ins {
		if inst.Type != "TimelineAddEntries" {
			continue
		}
		for _, e := range inst.Entries {
			tr := e.Content.ItemContent.TweetResults.Result
			if tr != nil {
				return tr
			}
		}
	}
	return nil
}

func extractBestMediaFromTweet(tr *tweetResult) []Media {
	if tr == nil {
		return nil
	}

	seen := make(map[string]struct{}, 8)
	var out []Media

	merge := func(ms []legacyMedia) {
		for _, m := range ms {
			switch m.Type {
			case "photo":
				u := upgradePhotoURL(m.MediaURLHTTPS)
				if u == "" {
					continue
				}
				if _, ok := seen[u]; ok {
					continue
				}
				seen[u] = struct{}{}
				out = append(out, Media{
					URL:  u,
					Type: "image",
				})
			case "video", "animated_gif":
				u := bestVideoVariantURL(m.VideoInfo.Variants)
				if u == "" {
					continue
				}
				if _, ok := seen[u]; ok {
					continue
				}
				seen[u] = struct{}{}
				out = append(out, Media{
					URL:  u,
					Type: "video",
				})
			default:
				continue
			}
		}
	}

	merge(tr.Legacy.ExtendedEntities.Media)
	merge(tr.Legacy.Entities.Media)

	return out
}

func upgradePhotoURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	if _, ok := q["name"]; ok {
		q.Set("name", "orig")
		u.RawQuery = q.Encode()
		return u.String()
	}
	return raw
}

func bestVideoVariantURL(vs []struct {
	URL         string `json:"url"`
	Bitrate     *int   `json:"bitrate,omitempty"`
	ContentType string `json:"content_type"`
}) string {
	if len(vs) == 0 {
		return ""
	}

	type candidate struct {
		url string
		br  int
	}
	var cands []candidate

	for _, v := range vs {
		if v.URL == "" {
			continue
		}
		if !strings.HasPrefix(v.ContentType, "video/") {
			continue
		}
		br := 0
		if v.Bitrate != nil {
			br = *v.Bitrate
		}
		cands = append(cands, candidate{url: v.URL, br: br})
	}

	if len(cands) == 0 {
		return ""
	}

	sort.Slice(cands, func(i, j int) bool {
		return cands[i].br > cands[j].br
	})
	return cands[0].url
}
