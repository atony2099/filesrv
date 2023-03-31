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

// Config holds the configuration values
type Config struct {
	startingPort int
	dir          string
}

// FileWatcher watches for file changes
type FileWatcher struct {
	watcher *fsnotify.Watcher
}

// WebSocketClients holds connected WebSocket clients
type WebSocketClients struct {
	clients map[*websocket.Conn]bool
}

// Server represents an HTTP server
type Server struct {
	server *http.Server
}

func main() {

	cfg := parseFlags()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port, err := findAvailablePort(cfg.startingPort)
	if err != nil {
		log.Fatal("Could not find an available port")
	}

	fileWatcher, err := NewFileWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer fileWatcher.Close()

	webSocketClients := NewWebSocketClients()
	server := NewServer(cfg.dir, port, webSocketClients)

	go server.Run(ctx)
	go fileWatcher.WatchFiles(ctx, cfg.dir, webSocketClients)

	url := fmt.Sprintf("http://localhost:%d", port)
	if err := browser.OpenURL(url); err != nil {
		log.Printf("Failed to open browser: %v", err)
	}

	// Wait for exit signal
	waitForExitSignal()
	log.Println("Received exit signal, shutting down server...")

}

func waitForExitSignal() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	<-signalChan
}

func parseFlags() Config {
	var cfg Config
	flag.IntVar(&cfg.startingPort, "port", 8080, "Starting port for the HTTP server")
	flag.StringVar(&cfg.dir, "dir", "./", "Directory to serve")
	flag.Parse()
	return cfg
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

		serveFileWithWebSocketInjection(w, r, dir)
	}

}

func serveFileWithWebSocketInjection(w http.ResponseWriter, r *http.Request, dir string) {

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

func NewWebSocketClients() *WebSocketClients {
	return &WebSocketClients{
		clients: make(map[*websocket.Conn]bool),
	}
}

// refreshBrowser sends refresh messages to connected WebSocket clients
func (wsc *WebSocketClients) refreshBrowser() {
	for client := range wsc.clients {
		err := client.WriteMessage(websocket.TextMessage, []byte("reload"))
		if err != nil {
			log.Printf("WebSocket error: %v", err)
			client.Close()
			delete(wsc.clients, client)
		}
	}
}

func (wsc *WebSocketClients) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	wsc.addClient(conn)
	defer wsc.removeClient(conn)

	wsc.keepWebSocketConnectionAlive(conn)

}

func (wsc *WebSocketClients) addClient(conn *websocket.Conn) {
	wsc.clients[conn] = true
}

func (wsc *WebSocketClients) removeClient(conn *websocket.Conn) {
	delete(wsc.clients, conn)
}

func (wsc *WebSocketClients) keepWebSocketConnectionAlive(conn *websocket.Conn) {
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// NewFileWatcher creates a new FileWatcher instance
func NewFileWatcher() (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &FileWatcher{watcher: watcher}, nil
}

// Close closes the FileWatcher instance
func (fw *FileWatcher) Close() {
	fw.watcher.Close()
}

// WatchFiles watches for file changes in the given directory
func (fw *FileWatcher) WatchFiles(ctx context.Context, dir string, wsClients *WebSocketClients) {
	go func() {
		for {
			select {
			case event := <-fw.watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Println("Modified file:", event.Name)
					wsClients.refreshBrowser()
				}
			case err := <-fw.watcher.Errors:
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
		if !info.IsDir() && isHTMLorCSSorJSFile(path) {
			err = fw.watcher.Add(path)
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

func isHTMLorCSSorJSFile(filePath string) bool {
	ext := filepath.Ext(filePath)
	return ext == ".html" || ext == ".css" || ext == ".js"
}

// NewServer creates a new Server instance
func NewServer(dir string, port int, wsClients *WebSocketClients) *Server {
	server := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: http.DefaultServeMux,
	}

	http.HandleFunc("/", handleRequest(dir))
	http.HandleFunc("/ws", wsClients.handleWebSocket)

	return &Server{server: server}
}

// Run starts the HTTP server
func (s *Server) Run(ctx context.Context) {
	go func() {
		err := s.server.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	log.Printf("Serving on: http://localhost%s\n", s.server.Addr)

	<-ctx.Done()

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.server.Shutdown(ctxShutdown)
}
