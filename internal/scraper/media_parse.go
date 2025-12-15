package scraper

import (
	"encoding/json"
	"net/url"
	"strings"
)

func fold(b []byte) ([]Media, error) {
	var root any
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, err
	}

	out := make([]Media, 0, 64)
	seen := make(map[string]struct{}, 64)

	collectMedia(root, "", &out, seen)

	return out, nil
}

func collectMedia(v any, currentTweetID string, out *[]Media, seen map[string]struct{}) {
	switch t := v.(type) {
	case map[string]any:
		if id, ok := t["rest_id"].(string); ok && id != "" {
			currentTweetID = id
		}

		if rawURL, ok := t["media_url_https"]; ok {
			base, ok2 := rawURL.(string)
			if ok2 && base != "" {
				mediaType := "image"

				if rawType, ok3 := t["type"]; ok3 {
					if typeStr, ok4 := rawType.(string); ok4 {
						switch strings.ToLower(typeStr) {
						case "photo":
							mediaType = "image"
						case "video", "animated_gif":
							mediaType = "video"
						}
					}
				}

				urlStr := base
				if mediaType == "video" {
					if vu, _ := bestVideoVariant(t); vu != "" {
						urlStr = vu
					}
				} else {
					urlStr = normalizeImageURL(base)
				}

				if urlStr != "" {
					if _, dup := seen[urlStr]; !dup {
						seen[urlStr] = struct{}{}
						*out = append(*out, Media{
							URL:     urlStr,
							Type:    mediaType,
							TweetID: currentTweetID,
						})
					}
				}
			}
		}

		for _, child := range t {
			collectMedia(child, currentTweetID, out, seen)
		}

	case []any:
		for _, child := range t {
			collectMedia(child, currentTweetID, out, seen)
		}
	}
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

	nq := url.Values{}
	if format != "" {
		nq.Set("format", format)
	}
	nq.Set("name", "orig")
	pu.RawQuery = nq.Encode()
	return pu.String()
}

func bestVideoVariant(m map[string]any) (string, int) {
	vi, ok := m["video_info"].(map[string]any)
	if !ok {
		return "", 0
	}
	vs, ok := vi["variants"].([]any)
	if !ok || len(vs) == 0 {
		return "", 0
	}

	bestURL := ""
	bestBR := -1

	for _, it := range vs {
		mv, ok := it.(map[string]any)
		if !ok {
			continue
		}
		ct, _ := mv["content_type"].(string)
		ct = strings.ToLower(ct)
		if !strings.Contains(ct, "video/mp4") {
			continue
		}
		u, _ := mv["url"].(string)
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
