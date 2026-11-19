// Package spotify provides the API client, catalog synchronization, and playback
// for Spotify online streaming.
package spotify

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Transport rate-limits requests to Spotify API, retrying on 429 (Too Many Requests)
// and coalescing concurrent identical GET requests via singleflight.
type Transport struct {
	base      http.RoundTripper
	rate      float64 // Tokens added per second.
	burst     float64 // Maximum token capacity.
	tokens    float64
	lastCheck time.Time
	mu        sync.Mutex
	group     singleflight.Group
}

// NewTransport creates a new rate-limited Transport with token bucket limits and base transport.
func NewTransport(rateLimit float64, burst int, base http.RoundTripper) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	if rateLimit <= 0 {
		rateLimit = 3.0
	}
	if burst <= 0 {
		burst = 5
	}
	return &Transport{
		base:      base,
		rate:      rateLimit,
		burst:     float64(burst),
		tokens:    float64(burst),
		lastCheck: time.Now(),
	}
}

// waitToken blocks until at least one token is available in the bucket.
func (t *Transport) waitToken(ctx context.Context) error {
	for {
		t.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(t.lastCheck).Seconds()
		t.lastCheck = now
		t.tokens += elapsed * t.rate
		if t.tokens > t.burst {
			t.tokens = t.burst
		}

		if t.tokens >= 1.0 {
			t.tokens -= 1.0
			t.mu.Unlock()
			return nil
		}

		needed := (1.0 - t.tokens) / t.rate
		t.mu.Unlock()

		waitDur := time.Duration(needed * float64(time.Second))
		if waitDur < 5*time.Millisecond {
			waitDur = 5 * time.Millisecond
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDur):
		}
	}
}

// RoundTrip executes the HTTP request with rate limiting, 429 retries, and request coalescing.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only coalesce concurrent bodyless GET requests.
	if req.Method == http.MethodGet && (req.Body == nil || req.Body == http.NoBody) {
		key := req.URL.String()
		res, err, _ := t.group.Do(key, func() (any, error) {
			resp, err := t.roundTripWithRetry(req)
			if err != nil {
				return nil, err
			}
			// Duplicate body so multiple readers can consume it concurrently.
			bodyBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return nil, err
			}
			return &cachedResponse{
				header:     resp.Header.Clone(),
				status:     resp.Status,
				statusCode: resp.StatusCode,
				proto:      resp.Proto,
				protoMajor: resp.ProtoMajor,
				protoMinor: resp.ProtoMinor,
				body:       bodyBytes,
			}, nil
		})
		if err != nil {
			return nil, err
		}
		cr := res.(*cachedResponse)
		return cr.toHTTPResponse(req), nil
	}

	return t.roundTripWithRetry(req)
}

type cachedResponse struct {
	header     http.Header
	status     string
	statusCode int
	proto      string
	protoMajor int
	protoMinor int
	body       []byte
}

func (cr *cachedResponse) toHTTPResponse(req *http.Request) *http.Response {
	return &http.Response{
		Status:        cr.status,
		StatusCode:    cr.statusCode,
		Proto:         cr.proto,
		ProtoMajor:    cr.protoMajor,
		ProtoMinor:    cr.protoMinor,
		Header:        cr.header.Clone(),
		Body:          io.NopCloser(bytes.NewReader(cr.body)),
		ContentLength: int64(len(cr.body)),
		Request:       req,
	}
}

// roundTripWithRetry executes the request, waiting for tokens and backing off on 429 status.
func (t *Transport) roundTripWithRetry(req *http.Request) (*http.Response, error) {
	const maxRetries = 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := t.waitToken(req.Context()); err != nil {
			return nil, err
		}

		resp, err := t.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusTooManyRequests || attempt == maxRetries {
			return resp, nil
		}

		// Read Retry-After header or use default safe backoff.
		backoff := 2 * time.Second
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			if secs, err := strconv.Atoi(retryAfter); err == nil && secs > 0 {
				backoff = time.Duration(secs) * time.Second
			}
		}

		// Add jitter between 100ms and 500ms to avoid reconnect thundering herd.
		jitter := time.Duration(100+rand.Intn(400)) * time.Millisecond
		backoff += jitter

		// Close previous response body.
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(backoff):
		}
	}

	return nil, errors.New("rate limit retries exhausted")
}
