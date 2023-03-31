package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Existing test cases

func TestServeFileWithWebSocketInjection(t *testing.T) {
	// Create a temporary HTML file
	root := "./"
	tempFile, err := ioutil.TempFile(root, "test-*.html")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	// Write content to the temporary HTML file
	htmlContent := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Test HTML</title>
</head>
<body>
	<h1>Hello, World!</h1>
</body>
</html>`
	_, err = tempFile.WriteString(htmlContent)
	if err != nil {
		t.Fatal(err)
	}
	tempFile.Close()

	// Create a test request
	req, err := http.NewRequest("GET", "/"+filepath.Base(tempFile.Name()), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a response recorder
	rr := httptest.NewRecorder()

	// Call serveFileWithWebSocketInjection
	serveFileWithWebSocketInjection(rr, req, root)

	// Check if the WebSocket injection code is present in the response body
	injectedContent := rr.Body.String()

	if !strings.Contains(injectedContent, "let socket = new WebSocket") {
		t.Error("WebSocket injection code not found in the served HTML file")
	}
}

func TestParseFlags(t *testing.T) {
	cfg := parseFlags()

	if cfg.startingPort != 8080 {
		t.Errorf("Expected startingPort to be 8080, but got %d", cfg.startingPort)
	}

	if cfg.dir != "./" {
		t.Errorf("Expected dir to be './', but got %s", cfg.dir)
	}
}

func TestRunServer(t *testing.T) {
	// Create a new temporary directory to serve files from
	dir, err := ioutil.TempDir("", "fileserver-test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(dir)

	// Create a new file to serve
	filePath := filepath.Join(dir, "test.html")
	err = ioutil.WriteFile(filePath, []byte("Hello, World!"), 0644)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Start the server in a separate goroutine
	cfg := Config{startingPort: 8080, dir: dir}
	wsc := NewWebSocketClients()
	server := NewServer(cfg.dir, cfg.startingPort, wsc)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Run(ctx)

	// Wait for the server to start up
	time.Sleep(100 * time.Millisecond)

	// Make a request to the server and verify the response
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/test.html", cfg.startingPort))
	if err != nil {
		t.Fatalf("Failed to make HTTP request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code to be %v, but got %v", http.StatusOK, resp.StatusCode)
	}

	expectedContent := "Hello, World!"
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if !strings.Contains(string(body), expectedContent) {
		t.Errorf("Expected body to contain '%v', but got '%v'", expectedContent, string(body))
	}

}
