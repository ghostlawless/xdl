package httpx

import (
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"net/http"
	"strings"

	xlog "github.com/ghostlawless/xdl/internal/log"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

type fnc func() error

func (f fnc) Close() error { return f() }

func Decode(res *http.Response) ([]byte, error) {
	return DecodeWithLimit(res, 0)
}

func DecodeWithLimit(res *http.Response, maxBytes int64) ([]byte, error) {
	if res == nil || res.Body == nil {
		return nil, errors.New("nil response/body")
	}
	defer res.Body.Close()
	r, cs, err := chain(res.Body, res.Header.Get("Content-Encoding"))
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 {
		r = io.LimitReader(r, maxBytes+1)
	}
	b, re := io.ReadAll(r)
	ce := shut(cs)
	if re != nil {
		xlog.LogError("read decoded body", re.Error())
		return nil, re
	}
	if ce != nil {
		return nil, ce
	}
	if maxBytes > 0 && int64(len(b)) > maxBytes {
		return nil, errors.New("decoded body exceeds maxBytes")
	}
	return b, nil
}

func StreamDecode(res *http.Response) (io.ReadCloser, error) {
	if res == nil || res.Body == nil {
		return nil, errors.New("nil response/body")
	}
	r, cs, err := chain(res.Body, res.Header.Get("Content-Encoding"))
	if err != nil {
		_ = res.Body.Close()
		return nil, err
	}
	if len(cs) == 0 && strings.TrimSpace(res.Header.Get("Content-Encoding")) == "" {
		return res.Body, nil
	}
	return &wrap{Reader: r, cls: cs, body: res.Body}, nil
}

func chain(body io.ReadCloser, encHeader string) (io.Reader, []io.Closer, error) {
	r := io.Reader(body)
	var cs []io.Closer
	enc := strings.ToLower(strings.TrimSpace(encHeader))
	if enc == "" {
		return r, cs, nil
	}
	ps := strings.Split(enc, ",")
	for i := len(ps) - 1; i >= 0; i-- {
		e := strings.TrimSpace(ps[i])
		switch e {
		case "", "identity":
			continue
		case "gzip":
			gr, err := gzip.NewReader(r)
			if err != nil {
				xlog.LogError("gzip decode", err.Error())
				_ = shut(cs)
				return nil, nil, err
			}
			r = gr
			cs = append(cs, gr)
		case "br":
			r = brotli.NewReader(r)
		case "zstd":
			zr, err := zstd.NewReader(r)
			if err != nil {
				xlog.LogError("zstd decode", err.Error())
				_ = shut(cs)
				return nil, nil, err
			}
			r = zr
			cs = append(cs, fnc(func() error { zr.Close(); return nil }))
		case "deflate":
			zr, err := zlib.NewReader(r)
			if err != nil {
				xlog.LogError("deflate decode", err.Error())
				_ = shut(cs)
				return nil, nil, err
			}
			r = zr
			cs = append(cs, zr)
		default:
			_ = shut(cs)
			return nil, nil, errors.New("unsupported content-encoding: " + e)
		}
	}
	return r, cs, nil
}

type wrap struct {
	io.Reader
	cls  []io.Closer
	body io.Closer
}

func (m *wrap) Close() error {
	err := shut(m.cls)
	if cerr := m.body.Close(); err == nil && cerr != nil {
		err = cerr
	}
	return err
}

func shut(cs []io.Closer) error {
	var first error
	for i := len(cs) - 1; i >= 0; i-- {
		if err := cs[i].Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
