package controlplane

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRegistrySkillPackageServiceOrchestratesControlAndDataPlanes(t *testing.T) {
	payload := []byte("registry-skill-package")
	digest := sha256.Sum256(payload)
	var uploaded []byte
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/upload":
			if request.Header.Get("Authorization") != "" {
				t.Error("Cloud Authorization reached upload URL")
			}
			uploaded, _ = io.ReadAll(request.Body)
			w.WriteHeader(http.StatusNoContent)
			return
		case "/download":
			if request.Header.Get("Authorization") != "" {
				t.Error("Cloud Authorization reached download URL")
			}
			_, _ = w.Write(payload)
			return
		}
		action := request.Header.Get("X-Tc-Action")
		if request.Header.Get("Authorization") == "" {
			t.Errorf("%s control request was unsigned", action)
		}
		response := registryResponseForAction(action)
		switch action {
		case "GetSkillPackageUploadURL":
			response["UploadURL"] = server.URL + "/upload?signature=private"
		case "GetSkillPackageDownloadURL":
			response["DownloadURL"] = server.URL + "/download?signature=private"
			response["SHA256"] = hex.EncodeToString(digest[:])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
	}))
	defer server.Close()
	service := NewService(ServiceConfig{
		Region: "ap-shanghai", ControlEndpoint: server.URL, MonitorEndpoint: server.URL,
		Timeout: time.Second, HTTPClient: server.Client(),
		Credential: func(context.Context) (Credential, error) { return Credential{SecretID: "id", SecretKey: "key"}, nil },
	})
	upload, err := service.UploadRegistrySkillPackage(context.Background(), "reg-12345678", "rec-12345678", "rv-12345678", bytes.NewReader(payload), int64(len(payload)), int64(len(payload)))
	if err != nil || !bytes.Equal(uploaded, payload) || upload.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("upload=%+v bytes=%q err=%v", upload, uploaded, err)
	}
	var output bytes.Buffer
	download, err := service.DownloadRegistrySkillPackage(context.Background(), "reg-12345678", "rec-12345678", &RegistrySelector{VersionID: "rv-12345678"}, &output, int64(len(payload)))
	if err != nil || !bytes.Equal(output.Bytes(), payload) || download.ResolvedVersionID != "rv-12345678" || download.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("download=%+v bytes=%q err=%v", download, output.Bytes(), err)
	}
}
