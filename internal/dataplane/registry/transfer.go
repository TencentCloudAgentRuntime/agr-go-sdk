// Package registry implements bounded Skill package transfer over service-issued URLs.
package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"net/http"
	"net/url"
)

// ErrorKind classifies a transfer failure without retaining the signed URL.
type ErrorKind string

const (
	InvalidURL     ErrorKind = "INVALID_URL"
	InvalidSize    ErrorKind = "INVALID_SIZE"
	SizeExceeded   ErrorKind = "SIZE_EXCEEDED"
	RequestFailed  ErrorKind = "REQUEST_FAILED"
	HTTPStatus     ErrorKind = "HTTP_STATUS"
	DigestMismatch ErrorKind = "DIGEST_MISMATCH"
)

// Error is safe to format: it never stores the service-issued signed URL or response body.
type Error struct {
	Kind       ErrorKind
	StatusCode int
	cause      error
}

func (e *Error) Error() string { return "registry skill package transfer failed: " + string(e.Kind) }
func (e *Error) Unwrap() error { return e.cause }

// Result reports local streaming evidence.
type Result struct {
	Bytes  int64
	SHA256 string
}

// Client transfers opaque gzip archives without following redirects or adding Cloud headers.
type Client struct{ http *http.Client }

// New copies client configuration and always rejects redirects. It does not own the transport.
func New(client *http.Client) *Client {
	value := http.Client{}
	if client != nil {
		value = *client
	}
	value.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{http: &value}
}

// Upload sends exactly size bytes from body. Bytes after size remain unread.
func (c *Client) Upload(ctx context.Context, address string, body io.Reader, size, max int64) (Result, error) {
	if err := validateURL(address); err != nil {
		return Result{}, err
	}
	if body == nil || size <= 0 || max <= 0 {
		return Result{}, &Error{Kind: InvalidSize}
	}
	if size > max {
		return Result{}, &Error{Kind: SizeExceeded}
	}
	reader := &hashingReader{reader: io.LimitReader(body, size), hash: sha256.New()}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, address, reader)
	if err != nil {
		return Result{}, &Error{Kind: InvalidURL, cause: errors.New("construct upload request")}
	}
	request.ContentLength = size
	request.Header.Set("Content-Type", "application/gzip")
	response, err := c.http.Do(request)
	if err != nil {
		return Result{}, &Error{Kind: RequestFailed, cause: errors.New("execute upload request")}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Result{}, &Error{Kind: HTTPStatus, StatusCode: response.StatusCode}
	}
	if reader.count != size {
		return Result{}, &Error{Kind: InvalidSize}
	}
	return Result{Bytes: reader.count, SHA256: hex.EncodeToString(reader.hash.Sum(nil))}, nil
}

// Download writes at most max bytes and verifies the service-recorded SHA-256. A failed call
// may have written a prefix to dst; callers that need atomic replacement should use a temporary
// destination and rename only after success.
func (c *Client) Download(ctx context.Context, address, expectedSHA256 string, dst io.Writer, max int64) (Result, error) {
	if err := validateURL(address); err != nil {
		return Result{}, err
	}
	if dst == nil || max <= 0 || !validSHA256(expectedSHA256) {
		return Result{}, &Error{Kind: InvalidSize}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return Result{}, &Error{Kind: InvalidURL, cause: errors.New("construct download request")}
	}
	response, err := c.http.Do(request)
	if err != nil {
		return Result{}, &Error{Kind: RequestFailed, cause: errors.New("execute download request")}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return Result{}, &Error{Kind: HTTPStatus, StatusCode: response.StatusCode}
	}
	if response.ContentLength > max {
		return Result{}, &Error{Kind: SizeExceeded}
	}
	digest := sha256.New()
	written, err := io.Copy(io.MultiWriter(dst, digest), io.LimitReader(response.Body, max))
	if err != nil {
		return Result{}, &Error{Kind: RequestFailed, cause: errors.New("read download response")}
	}
	var extra [1]byte
	n, readErr := response.Body.Read(extra[:])
	if n != 0 {
		return Result{}, &Error{Kind: SizeExceeded}
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return Result{}, &Error{Kind: RequestFailed, cause: errors.New("finish download response")}
	}
	actual := hex.EncodeToString(digest.Sum(nil))
	if actual != expectedSHA256 {
		return Result{}, &Error{Kind: DigestMismatch}
	}
	return Result{Bytes: written, SHA256: actual}, nil
}

func validateURL(address string) error {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return &Error{Kind: InvalidURL}
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == hex.EncodeToString(decoded)
}

type hashingReader struct {
	reader io.Reader
	hash   hash.Hash
	count  int64
}

func (r *hashingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		_, _ = r.hash.Write(p[:n])
		r.count += int64(n)
	}
	return n, err
}
