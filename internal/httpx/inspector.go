package httpx

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type InspectorTransport struct {
	base          http.RoundTripper
	logger        *log.Logger
	hideSensitive bool
}

func NewInspectorTransport(base http.RoundTripper, logPath string, hideSensitive bool) (http.RoundTripper, error) {
	if base == nil {
		base = http.DefaultTransport
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	logger := log.New(f, "[http-inspector] ", log.LstdFlags|log.Lmicroseconds)

	return &InspectorTransport{
		base:          base,
		logger:        logger,
		hideSensitive: hideSensitive,
	}, nil
}

func (t *InspectorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	var reqBodyCopy []byte
	if req.Body != nil {
		buf, err := io.ReadAll(req.Body)
		if err == nil {
			reqBodyCopy = buf
			req.Body = io.NopCloser(bytes.NewReader(buf))
		} else {
			req.Body = io.NopCloser(bytes.NewReader(nil))
		}
	}

	resp, err := t.base.RoundTrip(req)
	duration := time.Since(start)

	ct := req.Header.Get("Content-Type")
	if strings.Contains(ct, "json") && len(reqBodyCopy) > 0 {
		method := req.Method
		urlStr := req.URL.String()

		headersCopy := make(map[string][]string, len(req.Header))
		for k, vals := range req.Header {
			copied := make([]string, len(vals))
			copy(copied, vals)
			headersCopy[k] = copied
		}

		if t.hideSensitive {
			delete(headersCopy, "Authorization")
			delete(headersCopy, "Cookie")
			delete(headersCopy, "X-Csrf-Token")
		}

		headersJSON, _ := json.Marshal(headersCopy)

		t.logger.Printf(
			"REQUEST %s %s (%s)\nHeaders: %s\nBody: %s\n",
			method,
			urlStr,
			duration.String(),
			string(headersJSON),
			string(reqBodyCopy),
		)
	}

	return resp, err
}
