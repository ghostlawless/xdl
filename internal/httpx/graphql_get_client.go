package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"
)

type GraphQLGetClient struct {
	httpClient     *http.Client
	baseURL        string
	defaultHeaders map[string]string
}

type GraphQLGetOptions struct {
	Path          string
	OperationName string
	Variables     map[string]any
	Features      map[string]any
	Headers       map[string]string
	Timeout       time.Duration
}

type GraphQLGetResponse struct {
	StatusCode int
	Headers    http.Header
	RawBody    []byte
	JSON       any
}

func NewGraphQLGetClient(baseURL string, timeout time.Duration, defaultHeaders map[string]string) *GraphQLGetClient {
	cl := &http.Client{
		Timeout: timeout,
	}

	return &GraphQLGetClient{
		httpClient:     cl,
		baseURL:        trimTrailingSlash(baseURL),
		defaultHeaders: cloneStringMap(defaultHeaders),
	}
}

func (c *GraphQLGetClient) Do(ctx context.Context, opt GraphQLGetOptions) (*GraphQLGetResponse, error) {
	if opt.Path == "" {
		return nil, fmt.Errorf("graphql-get: empty path")
	}

	var variablesJSON, featuresJSON []byte
	var err error

	if len(opt.Variables) > 0 {
		variablesJSON, err = json.Marshal(opt.Variables)
		if err != nil {
			return nil, fmt.Errorf("graphql-get: marshal variables: %w", err)
		}
	}

	if len(opt.Features) > 0 {
		featuresJSON, err = json.Marshal(opt.Features)
		if err != nil {
			return nil, fmt.Errorf("graphql-get: marshal features: %w", err)
		}
	}

	fullURL, err := c.buildURLWithQuery(opt.Path, variablesJSON, featuresJSON)
	if err != nil {
		return nil, fmt.Errorf("graphql-get: build url: %w", err)
	}

	if opt.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opt.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("graphql-get: new request: %w", err)
	}

	for k, v := range c.defaultHeaders {
		req.Header.Set(k, v)
	}

	for k, v := range opt.Headers {
		req.Header.Set(k, v)
	}

	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json, */*;q=0.1")
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("graphql-get: do request: %w", err)
	}
	defer res.Body.Close()

	rawBody, err := readAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("graphql-get: read body: %w", err)
	}

	var jsonBody any
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &jsonBody); err != nil {
			jsonBody = nil
		}
	}

	return &GraphQLGetResponse{
		StatusCode: res.StatusCode,
		Headers:    res.Header.Clone(),
		RawBody:    rawBody,
		JSON:       jsonBody,
	}, nil
}

func (c *GraphQLGetClient) buildURLWithQuery(p string, variablesJSON, featuresJSON []byte) (string, error) {
	base := c.baseURL
	if base == "" {
		return "", fmt.Errorf("empty baseURL")
	}

	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse baseURL: %w", err)
	}

	u.Path = path.Join(u.Path, p)

	q := u.Query()
	if len(variablesJSON) > 0 {
		q.Set("variables", string(variablesJSON))
	}
	if len(featuresJSON) > 0 {
		q.Set("features", string(featuresJSON))
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func cloneStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
