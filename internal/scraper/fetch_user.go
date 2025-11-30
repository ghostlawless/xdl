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

type fetchUserResponse struct {
	Data struct {
		User struct {
			Result struct {
				RestID string `json:"rest_id"`
			} `json:"result"`
		} `json:"user"`
	} `json:"data"`
}

func FetchUserID(client *http.Client, conf *config.EssentialsConfig, username string) (string, error) {
	if client == nil || conf == nil {
		return "", errors.New("nil client or config")
	}
	if username == "" {
		return "", errors.New("empty username")
	}

	endpoint, err := conf.GraphQLURL("user_by_screen_name")
	if err != nil {
		return "", err
	}

	varsJSON, _ := json.Marshal(map[string]string{"screen_name": username})
	featJSON, _ := conf.FeatureJSONFor("user_by_screen_name")
	referer := strings.TrimRight(conf.X.Network, "/") + "/" + username

	u := fmt.Sprintf("%s?variables=%s&features=%s",
		endpoint, url.QueryEscape(string(varsJSON)), url.QueryEscape(featJSON),
	)

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	conf.BuildRequestHeaders(req, referer)
	req.Header.Set("Accept", "application/json, */*;q=0.1")

	body, status, err := httpx.DoRequestWithOptions(client, req, httpx.RequestOptions{
		MaxBytes: 2 << 20,
		Decode:   true,
		Accept:   func(s int) bool { return s >= 200 && s < 300 },
	})
	if err != nil {
		if conf.Runtime.DebugEnabled {
			bodyPath, _ := utils.SaveTimestamped(conf.Paths.Debug, "err_user_by_screen_name", "json", body)
			meta := fmt.Sprintf("METHOD: GET\nSTATUS: %d\nURL: %s\n", status, u)
			_, _ = utils.SaveTimestamped(conf.Paths.Debug, "err_user_by_screen_name_meta", "txt", []byte(meta))
			log.LogError("user", fmt.Sprintf("UserByScreenName failed (status %d). see: %s", status, bodyPath))
		} else {
			log.LogError("user", fmt.Sprintf("UserByScreenName failed (status %d). run with -d for details.", status))
		}
		return "", err
	}

	var resp fetchUserResponse
	if jerr := json.Unmarshal(body, &resp); jerr == nil && resp.Data.User.Result.RestID != "" {
		return resp.Data.User.Result.RestID, nil
	}

	var generic any
	if jerr := json.Unmarshal(body, &generic); jerr == nil {
		if id := findUserID(generic); id != "" {
			return id, nil
		}
	}
	return "", errors.New("rest_id not found in response")
}

func findUserID(v any) string {
	switch t := v.(type) {
	case map[string]any:
		if s, ok := t["rest_id"].(string); ok && s != "" {
			return s
		}
		for _, vv := range t {
			if id := findUserID(vv); id != "" {
				return id
			}
		}
	case []any:
		for _, vv := range t {
			if id := findUserID(vv); id != "" {
				return id
			}
		}
	}
	return ""
}
