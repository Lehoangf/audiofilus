package spotify

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimiterAndBackoff(t *testing.T) {
	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"error":"rate limit"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"ok":true}`)
	}))
	defer ts.Close()

	client := &http.Client{Transport: NewTransport(10, 2, ts.Client().Transport)}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}
	if attempts.Load() < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempts.Load())
	}
}

func TestMissingRetryAfterHeader(t *testing.T) {
	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			// No Retry-After header provided
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"error":"rate limit"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"ok":true}`)
	}))
	defer ts.Close()

	client := &http.Client{Transport: NewTransport(50, 5, ts.Client().Transport)}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}
	if attempts.Load() < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempts.Load())
	}
}

func TestSingleflightCoalescing(t *testing.T) {
	var serverHits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHits.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"data":"ok"}`)
	}))
	defer ts.Close()

	client := &http.Client{Transport: NewTransport(50, 5, ts.Client().Transport)}

	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func() {
			resp, err := client.Get(ts.URL)
			if err == nil {
				resp.Body.Close()
			}
			done <- struct{}{}
		}()
	}
	<-done
	<-done

	if serverHits.Load() != 1 {
		t.Errorf("expected singleflight to coalesce 2 identical concurrent GETs into 1 request, got %d", serverHits.Load())
	}
}
