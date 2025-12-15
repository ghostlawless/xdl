package scraper

import (
	"encoding/json"
	"strings"
)

func next(b []byte) string {
	var r any
	if err := json.Unmarshal(b, &r); err != nil {
		return ""
	}
	if v := bottom(r); v != "" {
		return v
	}
	return anyc(r)
}

func bottom(v any) string {
	switch t := v.(type) {
	case map[string]any:
		if strings.EqualFold(str(t["cursorType"]), "Bottom") {
			if val := str(t["value"]); val != "" {
				return val
			}
		}
		for _, vv := range t {
			if got := bottom(vv); got != "" {
				return got
			}
		}
	case []any:
		for _, it := range t {
			if got := bottom(it); got != "" {
				return got
			}
		}
	}
	return ""
}

func anyc(v any) string {
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
			if got := anyc(vv); got != "" {
				return got
			}
		}
	case []any:
		for _, it := range t {
			if got := anyc(it); got != "" {
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
