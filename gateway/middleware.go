package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid" // Added this import for the ID generation
)

type contextKey string

const correlationIDKey contextKey = "correlation_id"

// CorrelationIDMiddleware checks for an existing X-Correlation-ID header
// or generates a new one, ensuring requests can be traced across services.
func CorrelationIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Correlation-ID")
		if id == "" {
			id = uuid.New().String()
		}

		c.Set("correlation_id", id) // Keep this as a string for Gin

		// VIBE FIX: Use the custom typed key for the standard context
		ctx := context.WithValue(c.Request.Context(), correlationIDKey, id)
		c.Request = c.Request.WithContext(ctx)

		c.Header("X-Correlation-ID", id)
		log.Printf("[CorrelationID: %s] %s %s", id, c.Request.Method, c.Request.URL.Path)
		c.Next()
	}
}

// bufferedWriter captures response writes in-memory so the middleware can
// decide whether to send the real response or a timeout response without
// racing with handler writes.
type bufferedWriter struct {
	buf    *bytes.Buffer
	head   http.Header
	status int
	wrote  bool
	closed bool
	mu     sync.RWMutex
}

// newBufferedWriter returns an initialized bufferedWriter used to capture
// response headers and body from handlers without flushing to the client.
func newBufferedWriter() *bufferedWriter {
	return &bufferedWriter{
		buf:    bytes.NewBuffer(nil),
		head:   make(http.Header),
		status: http.StatusOK,
	}
}

// Header returns the local header map for the buffered response.
// We take a read lock while returning to make the intention explicit and
// reduce the window where concurrent readers could race with writers.
func (b *bufferedWriter) Header() http.Header {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.head
}

func (b *bufferedWriter) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, nil
	}
	b.wrote = true
	return b.buf.Write(data)
}

func (b *bufferedWriter) WriteString(s string) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, nil
	}
	b.wrote = true
	return b.buf.WriteString(s)
}

// WriteHeader captures the status code but does not flush to the client.
func (b *bufferedWriter) WriteHeader(statusCode int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.status = statusCode
}

// WriteHeaderNow records that headers are being written now; it does not
// flush to the client but ensures a status is set and marks the writer as
// having written data so subsequent flush respects status.
func (b *bufferedWriter) WriteHeaderNow() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.status == 0 {
		b.status = http.StatusOK
	}
	b.wrote = true
}

// Status returns the status code that the handler set (or 200 by default).
func (b *bufferedWriter) Status() int {
	if b.status == 0 {
		return http.StatusOK
	}
	return b.status
}

// flushTo writes buffered headers and body to the real writer.
func (b *bufferedWriter) flushTo(w http.ResponseWriter) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for k, vv := range b.head {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(b.Status())
	_, _ = w.Write(b.buf.Bytes())
}

// RequestTimeoutMiddleware applies a context timeout to the request and
// buffers handler output. If the context deadline is exceeded, the middleware
// returns 504 and discards the handler response. This avoids concurrent
// response writes and ensures safe behavior with Gin.
func RequestTimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Choose a deadline that ensures a per-route timeout can shorten any
		// existing deadline but will not extend an earlier (shorter) deadline.
		// This avoids surprising nested timeout behavior while allowing route
		// specific shorter timeouts to take effect.
		var cancel context.CancelFunc
		var ctx context.Context
		if timeout <= 0 {
			// Preserve the existing behavior for zero/negative values.
			ctx, cancel = context.WithTimeout(c.Request.Context(), timeout)
		} else {
			if d, ok := c.Request.Context().Deadline(); ok {
				desired := time.Now().Add(timeout)
				// If an earlier deadline already exists, keep it. Otherwise set
				// a new deadline at the desired point.
				if d.Before(desired) {
					ctx = c.Request.Context()
				} else {
					ctx, cancel = context.WithDeadline(c.Request.Context(), desired)
				}
			} else {
				ctx, cancel = context.WithTimeout(c.Request.Context(), timeout)
			}
		}
		if cancel != nil {
			defer cancel()
		}
		c.Request = c.Request.WithContext(ctx)

		origWriter := c.Writer
		bw := newBufferedWriter()
		// replace the gin writer with a shim that uses bw and keeps orig writer
		c.Writer = &responseWriterShim{bw: bw, orig: origWriter}
		finished := make(chan struct{}, 1)
		// We only send one panic value and immediately select on it; use an
		// unbuffered channel to avoid unnecessary buffering semantics.
		panicChan := make(chan interface{})
		go func() {
			defer func() {
				if r := recover(); r != nil {
					panicChan <- r
				}
			}()
			c.Next()
			close(finished)
		}()
		select {
		case <-finished:
			// Handler finished before deadline: flush buffered response. Do not
			// restore c.Writer here to avoid racing with handler goroutine.
			bw.flushTo(origWriter)
			return
		case p := <-panicChan:
			// Restore the original writer so upstream Recovery middleware writes
			// directly to the real response, then re-panic so Recovery can handle it.
			c.Writer = origWriter
			panic(p)
		case <-ctx.Done():
			// Timeout exceeded — mark buffer closed to prevent further handler
			// writes. Do NOT restore c.Writer here, otherwise a concurrently
			// running handler may write directly to the real writer after the
			// timeout response was already sent (causing panics or corruption).
			bw.mu.Lock()
			bw.closed = true
			bw.mu.Unlock()
			origWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
			origWriter.WriteHeader(504)
			_, _ = origWriter.Write([]byte(`{"error":"Gateway Timeout","message":"Request exceeded maximum allowed time"}`))
			return
		}
	}
}

// responseWriterShim adapts bufferedWriter to satisfy gin.ResponseWriter so
// handlers that call c.Writer/SetHeader interact with the buffered headers
// and body. It forwards writes to the underlying bufferedWriter instance.
type responseWriterShim struct {
	bw   *bufferedWriter
	orig gin.ResponseWriter
}

func (rws *responseWriterShim) Header() http.Header               { return rws.bw.Header() }
func (rws *responseWriterShim) Write(data []byte) (int, error)    { return rws.bw.Write(data) }
func (rws *responseWriterShim) WriteString(s string) (int, error) { return rws.bw.WriteString(s) }
func (rws *responseWriterShim) WriteHeader(statusCode int)        { rws.bw.WriteHeader(statusCode) }
func (rws *responseWriterShim) WriteHeaderNow()                   { rws.bw.WriteHeaderNow() }
func (rws *responseWriterShim) Status() int                       { return rws.bw.Status() }
func (rws *responseWriterShim) Written() bool                     { return rws.bw.wrote }
func (rws *responseWriterShim) Size() int                         { return rws.bw.buf.Len() }
func (rws *responseWriterShim) WriteHeaderNowWithoutLock()        {}

// Flush flushes the response to the client if the underlying writer
// supports http.Flusher. This is a no-op otherwise.
func (rws *responseWriterShim) Flush() {
	if fl, ok := rws.orig.(http.Flusher); ok {
		fl.Flush()
	}
}

// Hijack delegates to the underlying writer if it supports http.Hijacker.
func (rws *responseWriterShim) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rws.orig.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("hijack not supported")
}

// Pusher delegates to the underlying writer if it supports http.Pusher.
func (rws *responseWriterShim) Pusher() http.Pusher {
	if p, ok := rws.orig.(http.Pusher); ok {
		return p
	}
	return nil
}

// CloseNotify delegates to the original writer's CloseNotify when available.
// If the original writer does not support CloseNotify, return a closed channel
// to indicate the connection is not closable via this notification.
func (rws *responseWriterShim) CloseNotify() <-chan bool {
	if rws.orig != nil {
		return rws.orig.CloseNotify()
	}
	ch := make(chan bool)
	close(ch)
	return ch

}
