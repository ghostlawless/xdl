package scraper

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ghostlawless/xdl/internal/config"
	"github.com/ghostlawless/xdl/internal/httpx"
	"github.com/ghostlawless/xdl/internal/log"
	"github.com/ghostlawless/xdl/internal/utils"
)

type userByScreenNameResponse struct {
	Data struct {
		User struct {
			Result struct {
				RestID string `json:"rest_id"`
			} `json:"result"`
		} `json:"user"`
	} `json:"data"`
}

func FetchUserID(cl *http.Client, cf *config.EssentialsConfig, usr string) (string, error) {
	if cl == nil || cf == nil {
		return "", errors.New("nil client or config")
	}
	if usr == "" {
		return "", errors.New("empty username")
	}
	ep, err := cf.GraphQLURL("user_by_screen_name")
	if err != nil {
		return "", err
	}
	vj, _ := json.Marshal(map[string]string{"screen_name": usr})
	fj, _ := cf.FeatureJSONFor("user_by_screen_name")
	ref := strings.TrimRight(cf.X.Network, "/") + "/" + usr

	q := fmt.Sprintf("%s?variables=%s&features=%s", ep, url.QueryEscape(string(vj)), url.QueryEscape(fj))

	rq, err := http.NewRequest(http.MethodGet, q, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	cf.BuildRequestHeaders(rq, ref)
	rq.Header.Set("Accept", "application/json, */*;q=0.1")

	b, st, err := httpx.DoRequestWithOptions(cl, rq, httpx.RequestOptions{
		MaxBytes: 2 << 20,
		Decode:   true,
	})

	if err != nil {
		if cf.Runtime.DebugEnabled {
			p, _ := utils.SaveTimestamped(cf.Paths.Debug, "err_user_by_screen_name", "json", b)
			meta := fmt.Sprintf("METHOD: GET\nSTATUS: %d\nURL: %s\n", st, q)
			_, _ = utils.SaveTimestamped(cf.Paths.Debug, "err_user_by_screen_name_meta", "txt", []byte(meta))
			log.LogError("user", fmt.Sprintf("UserByScreenName failed (status %d). see: %s", st, p))
		} else {
			log.LogError("user", fmt.Sprintf("UserByScreenName failed (status %d). run with -d for details.", st))
		}
		return "", err
	}

	var typed userByScreenNameResponse
	if jerr := json.Unmarshal(b, &typed); jerr == nil && typed.Data.User.Result.RestID != "" {
		return typed.Data.User.Result.RestID, nil
	}

	var generic any
	if jerr := json.Unmarshal(b, &generic); jerr == nil {
		if id := extractRestIDFromAny(generic); id != "" {
			return id, nil
		}
	}

	return "", errors.New("rest_id not found in response")
}

func extractRestIDFromAny(v any) string {
	switch t := v.(type) {
	case map[string]any:
		if s, ok := t["rest_id"].(string); ok && s != "" {
			return s
		}
		for _, vv := range t {
			if id := extractRestIDFromAny(vv); id != "" {
				return id
			}
		}
	case []any:
		for _, vv := range t {
			if id := extractRestIDFromAny(vv); id != "" {
				return id
			}
		}
	}
	return ""
}
