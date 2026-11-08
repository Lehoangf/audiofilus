// Package spotify proporciona el cliente de API, sincronización y reproducción
// para el servicio de música online de Spotify.
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

// Transport gestiona el ritmo de peticiones hacia la API de Spotify,
// reintentando automáticamente ante códigos 429 (Too Many Requests) y
// agrupando peticiones GET idénticas concurrentes mediante singleflight.
type Transport struct {
	base      http.RoundTripper
	rate      float64 // Tokens añadidos por segundo.
	burst     float64 // Capacidad máxima de tokens.
	tokens    float64
	lastCheck time.Time
	mu        sync.Mutex
	group     singleflight.Group
}

// NewTransport crea un nuevo Transport con límite de tasa de tokens y un transporte base.
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

// waitToken espera hasta que haya al menos un token disponible en el cubo.
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

// RoundTrip ejecuta la petición HTTP con control de tasa, reintento 429 y coalescencia.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Solo agrupamos peticiones GET concurrentes sin cuerpo.
	if req.Method == http.MethodGet && (req.Body == nil || req.Body == http.NoBody) {
		key := req.URL.String()
		res, err, _ := t.group.Do(key, func() (any, error) {
			resp, err := t.roundTripWithRetry(req)
			if err != nil {
				return nil, err
			}
			// Duplicamos el cuerpo para que múltiples lectores puedan consumirlo si concurren.
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

// roundTripWithRetry ejecuta la petición esperando el token y aplicando retroceso ante códigos 429.
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

		// Leer Retry-After o utilizar retroceso seguro por defecto.
		backoff := 2 * time.Second
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			if secs, err := strconv.Atoi(retryAfter); err == nil && secs > 0 {
				backoff = time.Duration(secs) * time.Second
			}
		}

		// Añadir jitter entre 100ms y 500ms para evitar tormentas de reconexión.
		jitter := time.Duration(100+rand.Intn(400)) * time.Millisecond
		backoff += jitter

		// Liberar cuerpo de la respuesta anterior.
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
