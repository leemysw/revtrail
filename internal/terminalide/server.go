// INPUT: 编辑器的当前选区。
// OUTPUT: Server：本机服务，向 Claude Code / Codex 提供选区。
// POS: 终端代理与编辑器的连接。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package terminalide

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"golang.org/x/net/websocket"
	"revtrail/internal/lsp"
)

type Selection struct {
	Range    lsp.Range `json:"selection"`
	Text     string    `json:"text"`
	FilePath string    `json:"filePath"`
}

type Server struct {
	Port       string
	codexToken string
	launchDir  string
	http       *http.Server
	lock       string
	done       chan struct{}
	mu         sync.Mutex
	current    Selection
	changed    chan struct{}
}

// Start returns a working Codex bridge even if Claude discovery fails; in
// that case the caller can report the error and still use the server.
func Start(root string) (*Server, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
	token := rand.Text()
	s := &Server{Port: port, codexToken: rand.Text(), done: make(chan struct{}), changed: make(chan struct{})}
	socket := websocket.Server{Handler: s.serve}
	s.http = &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/codex/selection" {
			s.codexSelection(w, r)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Claude-Code-Ide-Authorization")), []byte(token)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		socket.ServeHTTP(w, r)
	})}
	err = s.registerClaude(root, token)
	go s.http.Serve(listener)
	return s, err
}

// registerClaude advertises the running bridge to Claude Code. Codex uses
// the terminal environment and does not need a Claude configuration directory.
func (s *Server) registerClaude(root, token string) error {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".claude")
	}
	dir = filepath.Join(dir, "ide")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	lock := filepath.Join(dir, s.Port+".lock")
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	err = json.NewEncoder(f).Encode(map[string]any{"pid": os.Getpid(), "ideName": "Revtrail", "workspaceFolders": []string{root}, "transport": "ws", "runningInWindows": runtime.GOOS == "windows", "authToken": token})
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(lock)
		return err
	}
	s.lock = lock
	return nil
}

func (s *Server) Close() {
	if s.launchDir != "" {
		os.RemoveAll(s.launchDir)
	}
	if s.lock != "" {
		os.Remove(s.lock)
	}
	close(s.done)
	s.http.Close()
}

func (s *Server) Update(selection Selection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = selection
	close(s.changed)
	s.changed = make(chan struct{})
}

func (s *Server) serve(conn *websocket.Conn) {
	defer conn.Close()
	disconnected := make(chan struct{})
	defer close(disconnected)
	ready := make(chan struct{})
	go func() {
		defer conn.Close()
		select {
		case <-s.done:
			return
		case <-disconnected:
			return
		case <-ready:
		}
		// Claude sends ide_connected before its prompt has mounted the selection
		// listener. Coalesce any startup changes into the initial snapshot.
		select {
		case <-s.done:
			return
		case <-disconnected:
			return
		case <-time.After(400 * time.Millisecond):
		}
		for {
			s.mu.Lock()
			value, changed := s.current, s.changed
			s.mu.Unlock()
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := websocket.JSON.Send(conn, map[string]any{"jsonrpc": "2.0", "method": "selection_changed", "params": value}); err != nil {
				return
			}
			select {
			case <-s.done:
				return
			case <-disconnected:
				return
			case <-changed:
			}
		}
	}()
	var connected sync.Once
	for {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				ProtocolVersion string `json:"protocolVersion"`
			} `json:"params"`
		}
		if err := websocket.JSON.Receive(conn, &req); err != nil {
			return
		}

		if req.Method == "ide_connected" {
			connected.Do(func() { close(ready) })
		}
		if len(req.ID) == 0 {
			continue
		}
		reply := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		switch req.Method {
		case "initialize":
			reply["result"] = map[string]any{"protocolVersion": req.Params.ProtocolVersion, "capabilities": map[string]any{"tools": struct{}{}}, "serverInfo": map[string]string{"name": "Revtrail", "version": "1"}}
		case "tools/list":
			reply["result"] = map[string]any{"tools": []any{}}
		case "ping":
			reply["result"] = struct{}{}
		default:
			reply["error"] = map[string]any{"code": -32601, "message": "Method not found"}
		}
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := websocket.JSON.Send(conn, reply); err != nil {
			return
		}
	}
}
