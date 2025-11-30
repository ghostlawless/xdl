package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

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
	DebugEnabled   bool `json:"debug_enabled"`
	TimeoutSeconds int  `json:"timeout_seconds"`
	MaxRetries     int  `json:"max_retries"`
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
	var lastErr error
	for _, p := range paths {
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			lastErr = err
			continue
		}
		var cfg EssentialsConfig
		dec := json.NewDecoder(strings.NewReader(string(data)))
		if err := dec.Decode(&cfg); err != nil {
			return nil, fmt.Errorf("failed to parse essentials.json: %w", err)
		}
		if strings.TrimSpace(cfg.X.Network) == "" {
			cfg.X.Network = "https://x.com"
		}
		return &cfg, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no essentials.json found")
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

func (c *EssentialsConfig) GraphQLURL(operationKey string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("nil config")
	}
	if c.GraphQL.Operations == nil {
		return "", fmt.Errorf("graphql.operations is empty")
	}
	op, ok := c.GraphQL.Operations[operationKey]
	if !ok || strings.TrimSpace(op.Path) == "" {
		return "", fmt.Errorf("unknown graphql operation: %s", operationKey)
	}
	base := "https://x.com/i/api/graphql"
	return base + "/" + op.Path, nil
}

func (c *EssentialsConfig) FeatureJSONFor(operationKey string) (string, error) {
	if c == nil {
		return "{}", nil
	}
	var src any
	switch operationKey {
	case "user_by_screen_name":
		src = c.Features.User
	case "user_media":
		src = c.Features.Media
	default:
		src = c.Features.User
	}
	if src == nil {
		return "{}", nil
	}
	b, err := json.Marshal(src)
	if err != nil {
		return "{}", err
	}
	return string(b), nil
}

func (c *EssentialsConfig) BuildRequestHeaders(req *http.Request, referer string) {
	if c == nil || req == nil {
		return
	}

	for k, v := range c.Headers {
		if v == "" {
			continue
		}
		if strings.EqualFold(k, "cookie") {
			continue
		}
		req.Header.Set(k, v)
	}

	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	if c.Auth.Bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.Auth.Bearer)
	}

	if c.Auth.Cookies.Ct0 != "" {
		req.Header.Set("x-csrf-token", c.Auth.Cookies.Ct0)
	}

	var cookies []string
	if c.Auth.Cookies.GuestID != "" {
		cookies = append(cookies, "guest_id="+c.Auth.Cookies.GuestID)
	}
	if c.Auth.Cookies.AuthToken != "" {
		cookies = append(cookies, "auth_token="+c.Auth.Cookies.AuthToken)
	}
	if c.Auth.Cookies.Ct0 != "" {
		cookies = append(cookies, "ct0="+c.Auth.Cookies.Ct0)
	}
	if len(cookies) > 0 {
		req.Header.Set("Cookie", strings.Join(cookies, "; "))
	}
}

type BrowserCookie struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	Path   string `json:"path"`
	Secure bool   `json:"secure"`
}

func ApplyCookiesFromFile(cfg *EssentialsConfig, cookiePath string) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}
	if strings.TrimSpace(cookiePath) == "" {
		return nil
	}

	data, err := os.ReadFile(cookiePath)
	if err != nil {
		return fmt.Errorf("failed to read cookie file: %w", err)
	}

	var cookies []BrowserCookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return fmt.Errorf("failed to parse cookie file: %w", err)
	}

	for _, c := range cookies {
		d := strings.ToLower(strings.TrimSpace(c.Domain))
		if !strings.Contains(d, "x.com") {
			continue
		}
		switch strings.ToLower(c.Name) {
		case "guest_id":
			cfg.Auth.Cookies.GuestID = c.Value
		case "auth_token":
			cfg.Auth.Cookies.AuthToken = c.Value
		case "ct0":
			cfg.Auth.Cookies.Ct0 = c.Value
		}
	}

	return nil
}
