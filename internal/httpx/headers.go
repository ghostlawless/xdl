package httpx

import (
	"bufio"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type HeaderEntry struct {
	Name  string
	Value string
}

var (
	gh   []HeaderEntry
	once sync.Once
)

func read(p string) ([]HeaderEntry, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var hs []HeaderEntry
	s := bufio.NewScanner(f)
	for s.Scan() {
		l := strings.TrimSpace(s.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		ps := strings.SplitN(l, ":", 2)
		if len(ps) != 2 {
			return nil, errors.New("invalid header line: " + l)
		}
		n := strings.TrimSpace(ps[0])
		v := strings.TrimSpace(ps[1])
		if n == "" || v == "" {
			return nil, errors.New("invalid header line: " + l)
		}
		hs = append(hs, HeaderEntry{Name: n, Value: v})
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return hs, nil
}

func defp() string {
	if env := os.Getenv("XDL_HEADERS_FILE"); env != "" {
		return env
	}
	return filepath.Join("config", "headers_browser_default.txt")
}

func initOnce() {
	once.Do(func() {
		p := defp()
		h, err := read(p)
		if err != nil {
			gh = nil
			return
		}
		gh = h
	})
}

func ApplyConfiguredHeaders(req *http.Request) {
	if req == nil {
		return
	}
	initOnce()
	if len(gh) == 0 {
		return
	}
	for _, h := range gh {
		nl := strings.ToLower(h.Name)
		if strings.HasPrefix(h.Name, ":") {
			continue
		}
		if nl == "host" || nl == "content-length" {
			continue
		}
		if nl == "cookie" ||
			nl == "authorization" ||
			nl == "x-csrf-token" ||
			nl == "x-twitter-auth-type" ||
			nl == "x-client-transaction-id" ||
			nl == "x-xp-forwarded-for" ||
			nl == "user-agent" ||
			nl == "referer" {
			continue
		}
		req.Header.Set(h.Name, h.Value)
	}
}
