package main

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestServer(dir string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRequest(dir))
	mux.HandleFunc("/ws", handleWebSocket)
	return mux
}

func TestHandleRequest(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "test-server")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	indexContent := "<!DOCTYPE html><html><head></head><body>Hello, World!</body></html>"
	err = ioutil.WriteFile(filepath.Join(tmpDir, "index.html"), []byte(indexContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	server := createTestServer(tmpDir)
	testServer := httptest.NewServer(server)
	defer testServer.Close()

	resp, err := http.Get(testServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	expectedBody := "<!DOCTYPE html><html><head></head><body>Hello, World!<script>let socket = new WebSocket(\"ws://\" + location.host + \"/ws\");socket.onmessage = function(event) {if (event.data === \"reload\") {location.reload();}};</script></body></html>"

	got := strings.ReplaceAll(string(body), "\n", "")
	got = strings.ReplaceAll(got, "\t", "")
	want := strings.ReplaceAll(expectedBody, "\n", "")
	want = strings.ReplaceAll(want, "\t", "")

	if got != want {
		t.Errorf("Unexpected response body:\nGot: %s\nWant: %s", string(body), expectedBody)
	}
}

func TestFindAvailablePort(t *testing.T) {
	tests := []struct {
		startingPort int
		expectedPort int
	}{
		{8080, 8080},
		{8081, 8081},
		{0, -1}, // test when no ports are available
	}

	for _, tt := range tests {
		port, _ := findAvailablePort(tt.startingPort)
		if port != tt.expectedPort {
			t.Errorf("findAvailablePort(%d) = %d, expected %d", tt.startingPort, port, tt.expectedPort)
		}
	}
}
