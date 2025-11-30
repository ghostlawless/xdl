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

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

func Decode(res *http.Response) ([]byte, error) {
	return DecodeWithLimit(res, 0)
}

func DecodeWithLimit(res *http.Response, maxBytes int64) ([]byte, error) {
	if res == nil || res.Body == nil {
		return nil, errors.New("nil response/body")
	}
	defer res.Body.Close()

	reader, closers, err := buildDecoderChain(res.Body, res.Header.Get("Content-Encoding"))
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 {
		reader = io.LimitReader(reader, maxBytes+1)
	}
	body, readErr := io.ReadAll(reader)
	closeErr := closeAll(closers)
	if readErr != nil {
		xlog.LogError("read decoded body", readErr.Error())
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if maxBytes > 0 && int64(len(body)) > maxBytes {
		return nil, errors.New("decoded body exceeds maxBytes")
	}
	return body, nil
}

func StreamDecode(res *http.Response) (io.ReadCloser, error) {
	if res == nil || res.Body == nil {
		return nil, errors.New("nil response/body")
	}
	reader, closers, err := buildDecoderChain(res.Body, res.Header.Get("Content-Encoding"))
	if err != nil {
		_ = res.Body.Close()
		return nil, err
	}
	if len(closers) == 0 && strings.TrimSpace(res.Header.Get("Content-Encoding")) == "" {
		return res.Body, nil
	}
	return &multiCloser{Reader: reader, closers: closers, body: res.Body}, nil
}

func buildDecoderChain(body io.ReadCloser, encHeader string) (io.Reader, []io.Closer, error) {
	reader := io.Reader(body)
	var closers []io.Closer

	encHeader = strings.ToLower(strings.TrimSpace(encHeader))
	if encHeader == "" {
		return reader, closers, nil
	}
	parts := strings.Split(encHeader, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		enc := strings.TrimSpace(parts[i])
		switch enc {
		case "", "identity":
			continue
		case "gzip":
			gr, err := gzip.NewReader(reader)
			if err != nil {
				xlog.LogError("gzip decode", err.Error())
				closeAll(closers)
				return nil, nil, err
			}
			reader = gr
			closers = append(closers, gr)
		case "br":
			reader = brotli.NewReader(reader)
		case "zstd":
			zr, err := zstd.NewReader(reader)
			if err != nil {
				xlog.LogError("zstd decode", err.Error())
				closeAll(closers)
				return nil, nil, err
			}
			reader = zr
			closers = append(closers, closerFunc(func() error { zr.Close(); return nil }))
		case "deflate":
			zr, err := zlib.NewReader(reader)
			if err != nil {
				xlog.LogError("deflate decode", err.Error())
				closeAll(closers)
				return nil, nil, err
			}
			reader = zr
			closers = append(closers, zr)
		default:
			closeAll(closers)
			return nil, nil, errors.New("unsupported content-encoding: " + enc)
		}
	}
	return reader, closers, nil
}

type multiCloser struct {
	io.Reader
	closers []io.Closer
	body    io.Closer
}

func (m *multiCloser) Close() error {
	err := closeAll(m.closers)
	if cerr := m.body.Close(); err == nil && cerr != nil {
		err = cerr
	}
	return err
}

func closeAll(closers []io.Closer) error {
	var first error
	for i := len(closers) - 1; i >= 0; i-- {
		if err := closers[i].Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
