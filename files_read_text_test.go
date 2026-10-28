package agr

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type observedTextBody struct {
	reader *strings.Reader
	read   int
	closed bool
}

func (b *observedTextBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	b.read += n
	return n, err
}

func (b *observedTextBody) Close() error {
	b.closed = true
	return nil
}

func TestReadTextBoundsDecodesAndCloses(t *testing.T) {
	for _, test := range []struct {
		name, content, want string
		limit               int64
		code                ErrorCode
	}{
		{"empty", "", "", 1, ""},
		{"utf8", "你好", "你好", 6, ""},
		{"too large", "abcdef", "", 5, ResourceExhausted},
		{"invalid utf8", string([]byte{0xff}), "", 2, Protocol},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := &observedTextBody{reader: strings.NewReader(test.content)}
			transport := roundTripFuncRoot(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/files" || r.URL.Query().Get("path") != "/config.txt" {
					t.Fatalf("unexpected file request: %s", r.URL)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}, nil
			})
			client := timeoutTestClient(t, metricControl{})
			sandbox := newSandbox(client, "sb-test", newDataPlane("https://runtime.example.test", "token", &http.Client{Transport: transport}))
			defer sandbox.Close()
			got, err := sandbox.Files().ReadText(context.Background(), "/config.txt", ReadOptions{}, test.limit)
			if test.code == "" {
				if err != nil || got != test.want {
					t.Fatalf("ReadText = %q, %v; want %q", got, err, test.want)
				}
			} else {
				var failure *Error
				if !errors.As(err, &failure) || failure.Code != test.code {
					t.Fatalf("ReadText = %q, %v; want %s", got, err, test.code)
				}
			}
			if !body.closed || body.read > int(test.limit)+1 {
				t.Fatalf("stream closed=%v, bytes read=%d, limit=%d", body.closed, body.read, test.limit)
			}
		})
	}
}

func TestReadTextRejectsInvalidLimitBeforeNetworkAndPreservesCancellation(t *testing.T) {
	requests := 0
	ctx, cancel := context.WithCancel(context.Background())
	body := &cancelTextBody{ctx: ctx, cancel: cancel}
	transport := roundTripFuncRoot(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}, nil
	})
	client := timeoutTestClient(t, metricControl{})
	sandbox := newSandbox(client, "sb-test", newDataPlane("https://runtime.example.test", "token", &http.Client{Transport: transport}))
	defer sandbox.Close()
	for _, limit := range []int64{0, -1, 16<<20 + 1} {
		_, err := sandbox.Files().ReadText(context.Background(), "/config.txt", ReadOptions{}, limit)
		var failure *Error
		if !errors.As(err, &failure) || failure.Code != InvalidArgument || requests != 0 {
			t.Fatalf("limit %d: error=%v requests=%d", limit, err, requests)
		}
	}
	_, err := sandbox.Files().ReadText(ctx, "/config.txt", ReadOptions{}, 10)
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != Canceled || !body.closed || requests != 1 {
		t.Fatalf("canceled read = %v, closed=%v requests=%d", err, body.closed, requests)
	}
}

type cancelTextBody struct {
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
}

func (b *cancelTextBody) Read([]byte) (int, error) {
	b.cancel()
	return 0, b.ctx.Err()
}
func (b *cancelTextBody) Close() error {
	b.closed = true
	return nil
}

var _ io.ReadCloser = (*observedTextBody)(nil)
