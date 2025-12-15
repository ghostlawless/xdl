package config

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ghostlawless/xdl/internal/httpx"
)

//go:embed defaults/essentials.json
var embeddedEssentialsJSON []byte

type GraphQLOperation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type GraphQLSection struct {
	Operations map[string]GraphQLOperation `json:"operations"`
}

type AuthCookies struct {
	GuestID   string `json:"guest_id"`
	AuthToken string `json:"auth_token"`
	Ct0       string `json:"ct0"`
}

type AuthSection struct {
	Bearer  string      `json:"bearer"`
	Cookies AuthCookies `json:"cookies"`
}

type FeaturesSection struct {
	User  map[string]any `json:"user"`
	Media map[string]any `json:"media"`
}

type PathsSection struct {
	Logs     string `json:"logs"`
	Debug    string `json:"debug"`
	DebugRaw string `json:"debug_raw"`
	Exports  string `json:"exports"`
}

type RuntimeSection struct {
	DebugEnabled   bool   `json:"debug_enabled"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxRetries     int    `json:"max_retries"`
	LimiterSecret  string `json:"limiter_secret"`
}

type XSection struct {
	Network string `json:"network"`
}

type EssentialsConfig struct {
	X        XSection          `json:"x,omitempty"`
	GraphQL  GraphQLSection    `json:"graphql"`
	Auth     AuthSection       `json:"auth"`
	Headers  map[string]string `json:"headers"`
	Features FeaturesSection   `json:"features"`
	Paths    PathsSection      `json:"paths"`
	Runtime  RuntimeSection    `json:"runtime"`
}

func LoadEssentialsWithFallback(paths []string) (*EssentialsConfig, error) {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}

		cfg, err := loadEssentialsFromPath(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			return nil, fmt.Errorf("failed to load essentials.json from %s: %w", path, err)
		}

		return cfg, nil
	}

	if len(embeddedEssentialsJSON) == 0 {
		return nil, fmt.Errorf("no essentials.json found and embedded essentials is empty")
	}

	var cfg EssentialsConfig
	if err := json.Unmarshal(embeddedEssentialsJSON, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse embedded essentials.json: %w", err)
	}

	cfg.X.Network = normalizeNetwork(cfg.X.Network)
	return &cfg, nil

}

func loadEssentialsFromPath(path string) (*EssentialsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg EssentialsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse essentials.json: %w", err)
	}
	cfg.X.Network = normalizeNetwork(cfg.X.Network)
	return &cfg, nil
}

func normalizeNetwork(network string) string {
	if strings.TrimSpace(network) == "" {
		return "https://x.com"
	}
	return network
}

func (c *EssentialsConfig) HTTPTimeout() time.Duration {
	if c == nil {
		return 15 * time.Second
	}
	if c.Runtime.TimeoutSeconds <= 0 {
		return 15 * time.Second
	}
	return time.Duration(c.Runtime.TimeoutSeconds) * time.Second
}

func (c *EssentialsConfig) GraphQLURL(key string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("nil config")
	}
	if c.GraphQL.Operations == nil {
		return "", fmt.Errorf("graphql.operations is empty")
	}
	op, ok := c.GraphQL.Operations[key]
	if !ok || strings.TrimSpace(op.Path) == "" {
		return "", fmt.Errorf("unknown graphql operation: %s", key)
	}
	base := strings.TrimRight(c.X.Network, "/") + "/i/api/graphql"
	return base + "/" + op.Path, nil
}

func (c *EssentialsConfig) FeatureJSONFor(key string) (string, error) {
	if c == nil {
		return "{}", nil
	}
	src := c.featureSource(key)
	data, err := json.Marshal(src)
	if err != nil {
		return "{}", err
	}
	return string(data), nil
}

func (c *EssentialsConfig) featureSource(key string) any {
	switch key {
	case "user_by_screen_name":
		return c.Features.User
	case "user_media":
		return c.Features.Media
	case "tweet_detail":
		return map[string]bool{
			"rweb_video_screen_enabled":                                               false,
			"profile_label_improvements_pcf_label_in_post_enabled":                    true,
			"responsive_web_profile_redirect_enabled":                                 false,
			"rweb_tipjar_consumption_enabled":                                         false,
			"verified_phone_label_enabled":                                            false,
			"creator_subscriptions_tweet_preview_api_enabled":                         true,
			"responsive_web_graphql_timeline_navigation_enabled":                      true,
			"responsive_web_graphql_skip_user_profile_image_extensions_enabled":       false,
			"premium_content_api_read_enabled":                                        false,
			"communities_web_enable_tweet_community_results_fetch":                    true,
			"c9s_tweet_anatomy_moderator_badge_enabled":                               true,
			"responsive_web_grok_analyze_button_fetch_trends_enabled":                 false,
			"responsive_web_grok_analyze_post_followups_enabled":                      true,
			"responsive_web_jetfuel_frame":                                            true,
			"responsive_web_grok_share_attachment_enabled":                            true,
			"articles_preview_enabled":                                                true,
			"responsive_web_edit_tweet_api_enabled":                                   true,
			"graphql_is_translatable_rweb_tweet_is_translatable_enabled":              true,
			"view_counts_everywhere_api_enabled":                                      true,
			"longform_notetweets_consumption_enabled":                                 true,
			"responsive_web_twitter_article_tweet_consumption_enabled":                true,
			"tweet_awards_web_tipping_enabled":                                        false,
			"responsive_web_grok_show_grok_translated_post":                           false,
			"responsive_web_grok_analysis_button_from_backend":                        true,
			"creator_subscriptions_quote_tweet_preview_enabled":                       false,
			"freedom_of_speech_not_reach_fetch_enabled":                               true,
			"standardized_nudges_misinfo":                                             true,
			"tweet_with_visibility_results_prefer_gql_limited_actions_policy_enabled": true,
			"longform_notetweets_rich_text_read_enabled":                              true,
			"longform_notetweets_inline_media_enabled":                                true,
			"responsive_web_grok_image_annotation_enabled":                            true,
			"responsive_web_grok_imagine_annotation_enabled":                          true,
			"responsive_web_grok_community_note_auto_translation_is_enabled":          false,
			"responsive_web_enhance_cards_enabled":                                    false,
		}
	default:
		return c.Features.User
	}
}

func (c *EssentialsConfig) BuildRequestHeaders(req *http.Request, ref string) {
	if c == nil || req == nil {
		return
	}
	httpx.ApplyConfiguredHeaders(req)
	c.applyConfiguredHeaders(req)
	c.applyRefererHeader(req, ref)
	c.applyAuthHeaders(req)
	c.applyCookieHeader(req)
}

func (c *EssentialsConfig) applyConfiguredHeaders(req *http.Request) {
	for key, value := range c.Headers {
		if value == "" {
			continue
		}
		if strings.EqualFold(key, "cookie") {
			continue
		}
		req.Header.Set(key, value)
	}
}

func (c *EssentialsConfig) applyRefererHeader(req *http.Request, ref string) {
	if ref == "" {
		return
	}
	req.Header.Set("Referer", ref)
}

func (c *EssentialsConfig) applyAuthHeaders(req *http.Request) {
	if c.Auth.Bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.Auth.Bearer)
	}
	if c.Auth.Cookies.Ct0 != "" {
		req.Header.Set("x-csrf-token", c.Auth.Cookies.Ct0)
	}
}

func (c *EssentialsConfig) applyCookieHeader(req *http.Request) {
	cookieHeader := c.buildCookieHeader()
	if cookieHeader == "" {
		return
	}
	req.Header.Set("Cookie", cookieHeader)
}

func (c *EssentialsConfig) buildCookieHeader() string {
	var parts []string
	if c.Auth.Cookies.GuestID != "" {
		parts = append(parts, "guest_id="+c.Auth.Cookies.GuestID)
	}
	if c.Auth.Cookies.AuthToken != "" {
		parts = append(parts, "auth_token="+c.Auth.Cookies.AuthToken)
	}
	if c.Auth.Cookies.Ct0 != "" {
		parts = append(parts, "ct0="+c.Auth.Cookies.Ct0)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}

type BrowserCookie struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	Path   string `json:"path"`
	Secure bool   `json:"secure"`
}

var ErrCookieFileMissing = errors.New("cookie file missing")

func executableDir() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if rp, err := filepath.EvalSymlinks(p); err == nil && strings.TrimSpace(rp) != "" {
		p = rp
	}
	d := filepath.Dir(p)
	if strings.TrimSpace(d) == "" || d == "." {
		return ""
	}
	return d
}

func preferredCookiePathFor(name string) string {
	d := executableDir()
	if d == "" {
		return name
	}
	return filepath.Join(d, name)
}

func preferredCookiePath() string {
	return preferredCookiePathFor("cookies.txt")
}

func uniquePaths(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func cookiePathCandidates(input string) []string {
	p := strings.TrimSpace(input)
	d := executableDir()

	addVariants := func(x string) []string {
		x = strings.TrimSpace(x)
		if x == "" {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(x))
		if ext == "" {
			return []string{x, x + ".txt", x + ".json"}
		}

		base := strings.TrimSuffix(x, ext)
		if ext == ".json" {
			return []string{x, base + ".txt"}
		}
		if ext == ".txt" {
			return []string{x, base + ".json"}
		}

		return []string{x}
	}

	if p != "" {
		raw := make([]string, 0, 6)
		for _, v := range addVariants(p) {
			raw = append(raw, v)
			if d != "" && !filepath.IsAbs(v) {
				raw = append(raw, filepath.Join(d, v))
			}
		}
		return uniquePaths(raw)
	}

	raw := make([]string, 0, 10)
	if d != "" {
		raw = append(raw, filepath.Join(d, "cookies.txt"))
		raw = append(raw, filepath.Join(d, "cookies.json"))
	}
	raw = append(raw, "cookies.txt")
	raw = append(raw, "cookies.json")
	if d != "" {
		raw = append(raw, filepath.Join(d, "config", "cookies.txt"))
		raw = append(raw, filepath.Join(d, "config", "cookies.json"))
	}
	raw = append(raw, filepath.Join("config", "cookies.txt"))
	raw = append(raw, filepath.Join("config", "cookies.json"))
	return uniquePaths(raw)

}

func (c *EssentialsConfig) ValidateRequiredCookies(cookiePath string) error {
	if c == nil {
		return fmt.Errorf("nil config")
	}

	missing := make([]string, 0, 2)
	if strings.TrimSpace(c.Auth.Cookies.AuthToken) == "" {
		missing = append(missing, "auth_token")
	}
	if strings.TrimSpace(c.Auth.Cookies.Ct0) == "" {
		missing = append(missing, "ct0")
	}

	if len(missing) == 0 {
		return nil
	}

	p := strings.TrimSpace(cookiePath)
	if p == "" {
		p = preferredCookiePathFor("cookies.txt")
	}

	alt := preferredCookiePathFor("cookies.json")

	return fmt.Errorf(
		"%w\n\nAuthentication required.\n\n"+
			"Missing cookies: %s\n\n"+
			"Why this is needed:\n"+
			"X blocks most media access unless the session is logged in.\n\n"+
			"How to fix:\n"+
			"1) Log in to https://x.com in your browser\n"+
			"2) Export cookies as JSON (using Cookie-Editor or similar)\n"+
			"3) Save the file as:\n  %s\n  or %s\n"+
			"4) Run xdl again\n\n"+
			"This is required only once per account (until cookies expire).",
		ErrCookieFileMissing,
		strings.Join(missing, ", "),
		p,
		alt,
	)

}

func ApplyCookiesFromFile(cfg *EssentialsConfig, path string) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}

	candidates := cookiePathCandidates(path)

	for _, candidate := range candidates {
		cookies, err := loadBrowserCookies(candidate)
		if err != nil {
			if errors.Is(err, ErrCookieFileMissing) {
				continue
			}
			return err
		}

		cfg.applyBrowserCookies(cookies)

		if err := cfg.ValidateRequiredCookies(candidate); err != nil {
			return err
		}

		return nil
	}

	expectedTxt := preferredCookiePathFor("cookies.txt")
	expectedJSON := preferredCookiePathFor("cookies.json")

	return fmt.Errorf(
		"%w\n\nCookie file not found.\n\n"+
			"Expected location:\n  %s\n  or %s\n\n"+
			"How to fix:\n"+
			"1) Log in to https://x.com in your browser\n"+
			"2) Export cookies as JSON (Cookie-Editor or similar)\n"+
			"3) Save the file as cookies.txt (or cookies.json) in the expected location\n"+
			"4) Run xdl again",
		ErrCookieFileMissing,
		expectedTxt,
		expectedJSON,
	)

}

func loadBrowserCookies(path string) ([]BrowserCookie, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCookieFileMissing
		}
		return nil, fmt.Errorf("failed to read cookie file %q: %w", path, err)
	}

	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, ErrCookieFileMissing
	}

	var cookies []BrowserCookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, fmt.Errorf(
			"failed to parse cookie file %q: invalid JSON format: %w",
			path,
			err,
		)
	}

	if len(cookies) == 0 {
		return nil, ErrCookieFileMissing
	}

	return cookies, err

}

func (c *EssentialsConfig) applyBrowserCookies(cookies []BrowserCookie) {
	for _, cookie := range cookies {
		domain := normalizeDomain(cookie.Domain)
		if !strings.Contains(domain, "x.com") {
			continue
		}
		c.assignCookieValue(cookie)
	}
}

func normalizeDomain(domain string) string {
	return strings.ToLower(strings.TrimSpace(domain))
}

func (c *EssentialsConfig) assignCookieValue(cookie BrowserCookie) {
	switch strings.ToLower(cookie.Name) {
	case "guest_id":
		c.Auth.Cookies.GuestID = cookie.Value
	case "auth_token":
		c.Auth.Cookies.AuthToken = cookie.Value
	case "ct0":
		c.Auth.Cookies.Ct0 = cookie.Value
	}
}

func SaveEssentials(cfg *EssentialsConfig, path string) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("empty essentials path")
	}
	if err := ensureEssentialsDir(path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", " ")
	if err != nil {
		return fmt.Errorf("failed to marshal essentials: %w", err)
	}
	if err := writeEssentialsAtomically(path, data); err != nil {
		return err
	}
	return err
}

func ensureEssentialsDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create essentials dir: %w", err)
	}
	return nil
}

func writeEssentialsAtomically(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("failed to write temporary essentials: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to replace essentials: %w", err)
	}
	return nil
}

func ApplyCookiesFromFileAndPersist(cfg *EssentialsConfig, cookiePath, essentialsPath string) error {
	if err := ApplyCookiesFromFile(cfg, cookiePath); err != nil {
		return err
	}
	if strings.TrimSpace(essentialsPath) == "" {
		return nil
	}
	return SaveEssentials(cfg, essentialsPath)
}
