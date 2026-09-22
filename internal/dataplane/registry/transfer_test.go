package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadIsBoundedAndSendsNoCloudAuthorization(t *testing.T) {
	payload := []byte("fixture-package")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.Header.Get("Content-Type") != "application/gzip" || request.Header.Get("Authorization") != "" {
			t.Errorf("unsafe request method=%s headers=%v", request.Method, request.Header)
		}
		body, _ := io.ReadAll(request.Body)
		if !bytes.Equal(body, payload) {
			t.Errorf("body=%q", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	result, err := New(server.Client()).Upload(context.Background(), server.URL+"/upload?signature=private", bytes.NewReader(append(payload, []byte("ignored")...)), int64(len(payload)), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(payload)
	if result.Bytes != int64(len(payload)) || result.SHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("result=%+v", result)
	}
}

func TestDownloadVerifiesDigestAndBound(t *testing.T) {
	payload := []byte("fixture-package")
	want := sha256.Sum256(payload)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	var output bytes.Buffer
	result, err := New(server.Client()).Download(context.Background(), server.URL+"/download?signature=private", hex.EncodeToString(want[:]), &output, int64(len(payload)))
	if err != nil || !bytes.Equal(output.Bytes(), payload) || result.Bytes != int64(len(payload)) {
		t.Fatalf("result=%+v output=%q err=%v", result, output.Bytes(), err)
	}

	output.Reset()
	_, err = New(server.Client()).Download(context.Background(), server.URL, strings.Repeat("0", 64), &output, int64(len(payload)))
	var transfer *Error
	if !errors.As(err, &transfer) || transfer.Kind != DigestMismatch {
		t.Fatalf("digest error=%v", err)
	}
	output.Reset()
	_, err = New(server.Client()).Download(context.Background(), server.URL, hex.EncodeToString(want[:]), &output, int64(len(payload)-1))
	if !errors.As(err, &transfer) || transfer.Kind != SizeExceeded {
		t.Fatalf("size error=%v", err)
	}
}

func TestTransferRejectsUnsafeURLAndRedirectWithoutLeakingURL(t *testing.T) {
	client := New(nil)
	secret := "private-signature"
	_, err := client.Upload(context.Background(), "http://example.test/upload?signature="+secret, strings.NewReader("x"), 1, 1)
	var transfer *Error
	if !errors.As(err, &transfer) || transfer.Kind != InvalidURL || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe URL error=%v", err)
	}

	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("redirect followed") }))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	_, err = New(redirect.Client()).Upload(context.Background(), redirect.URL+"?signature="+secret, strings.NewReader("x"), 1, 1)
	if !errors.As(err, &transfer) || transfer.Kind != HTTPStatus || transfer.StatusCode != http.StatusTemporaryRedirect || strings.Contains(err.Error(), secret) {
		t.Fatalf("redirect error=%v", err)
	}
}
