package lsp

import (
	"context"
	"io"
)

// Connect makes a client of a server already reached through r and w, for a test, and initializes it.
func Connect(ctx context.Context, r io.Reader, w io.WriteCloser, opts Options) (*Client, error) {
	c := newClient(w, r, opts)
	c.stdin = w
	go func() {
		<-c.conn.Done()
		c.finish(c.conn.Err())
	}()
	if err := c.initialize(ctx); err != nil {
		w.Close()
		return nil, err
	}
	return c, nil
}
