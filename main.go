package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"github.com/pkg/browser"
)

var clients = make(map[*websocket.Conn]bool)
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func watchFiles(ctx context.Context, dir string, watcher *fsnotify.Watcher) {

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Println("Modified file:", event.Name)
					refreshBrowser()
				}
			case err := <-watcher.Errors:
				if err != nil {
					log.Println("Watcher error:", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Println("Walker error:", err)
			return err
		}
		if info.IsDir() {
			err = watcher.Add(path)
			if err != nil {
				log.Println("Watcher error:", err)
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal("Walker error:", err)
	}

}

func startServer(ctx context.Context, dir string, port int) {
	server := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: http.DefaultServeMux,
	}

	http.HandleFunc("/", handleRequest(dir))
	http.HandleFunc("/ws", handleWebSocket)

	// Start the server in a separate goroutine
	go func() {
		err := server.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	log.Printf("Serving %s on HTTP port: %d\n", dir, port)

	// Wait for the context to be cancelled
	<-ctx.Done()

	// Shut down the server gracefully
	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctxShutdown)

}

func main() {

	var startingPort int
	var dir string
	flag.IntVar(&startingPort, "port", 8080, "Starting port for the HTTP server")
	flag.StringVar(&dir, "dir", "./", "Directory to serve")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())

	port, err := findAvailablePort(startingPort)
	if err != nil {
		log.Fatal("Could not find an available port")
	}

	watcher, err := fsnotify.NewWatcher()
	defer watcher.Close()
	if err != nil {
		log.Fatal(err)
	}

	go startServer(ctx, dir, port)
	go watchFiles(ctx, dir, watcher)

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Println("Modified file:", event.Name)
					refreshBrowser()
				}
			case err := <-watcher.Errors:
				log.Println("Error:", err)
			}
		}
	}()

	// Watch directory and its subdirectories
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	url := fmt.Sprintf("http://localhost:%d", port)
	if err := browser.OpenURL(url); err != nil {
		log.Printf("Failed to open browser: %v", err)
	}

	// Wait for exit signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	<-signalChan
	log.Println("Received exit signal, shutting down server...")
	cancel()
	time.Sleep(time.Second)

}

func findAvailablePort(startingPort int) (int, error) {

	if startingPort < 1024 {
		return -1, fmt.Errorf("port must be greater than or equal to 1024")
	}

	for i := startingPort; i <= 65535; i++ {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", i))
		if err == nil {
			listener.Close()
			return i, nil
		}
	}
	return -1, fmt.Errorf("could not find an available port")
}

func handleRequest(dir string) func(w http.ResponseWriter, r *http.Request) {

	return func(w http.ResponseWriter, r *http.Request) {
		// Serve files and inject WebSocket code into HTML files
		filePath := filepath.Join(dir, filepath.Clean(r.URL.Path))

		fileInfo, err := os.Stat(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if fileInfo.IsDir() {
			filePath = filepath.Join(filePath, "index.html")
		}

		if strings.HasSuffix(filePath, ".html") {
			content, err := ioutil.ReadFile(filePath)
			if err != nil {
				http.NotFound(w, r)
				return
			}

			// Inject WebSocket code into the HTML file
			injectedContent := strings.Replace(string(content), "</body>", `<script>
let socket = new WebSocket("ws://" + location.host + "/ws");
socket.onmessage = function(event) {
	if (event.data === "reload") {
		location.reload();
	}
};
</script></body>`, 1)

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(injectedContent))
		} else {
			http.ServeFile(w, r, filePath)
		}
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade the HTTP connection to a WebSocket connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	// Add the connection to the clients map
	clients[conn] = true
	defer delete(clients, conn)

	// Keep the WebSocket connection alive by reading messages
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func refreshBrowser() {
	// Send refresh message to connected WebSocket clients
	for client := range clients {
		err := client.WriteMessage(websocket.TextMessage, []byte("reload"))
		if err != nil {
			log.Printf("WebSocket error: %v", err)
			client.Close()
			delete(clients, client)
		}
	}
}
