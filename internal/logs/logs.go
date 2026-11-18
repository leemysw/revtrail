// INPUT: 语言服务器、工具与 App 的输出。
// OUTPUT: Store：最新条目的环形缓冲。
// POS: Output 面板的数据。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package logs

import (
	"log/slog"
	"sync"
	"time"
)

// maxEntries is how many entries the log keeps; older ones fall off.
const maxEntries = 1000

// maxMessage is the longest message kept, so that a runaway server cannot
// fill the window with one line.
const maxMessage = 8192

// Entry is one line of output.
type Entry struct {
	At      time.Time
	Level   slog.Level
	Source  string // the language server or tool that said it
	Message string
}

// Store is a log shared by the UI thread, which reads it, and background
// work, which writes it.
type Store struct {
	mu      sync.Mutex
	entries []Entry
}

// Add appends an entry, dropping the oldest beyond maxEntries.
func (s *Store) Add(level slog.Level, source, message string) {
	if len(message) > maxMessage {
		message = message[:maxMessage] + "…"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, Entry{time.Now(), level, source, message})
	if len(s.entries) > maxEntries {
		s.entries = append([]Entry(nil), s.entries[len(s.entries)-maxEntries:]...)
	}
}

// Snapshot returns the entries, oldest first, for a frame to draw.
func (s *Store) Snapshot() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Entry(nil), s.entries...)
}

// Clear empties the log.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
}
