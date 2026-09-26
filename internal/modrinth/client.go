package modrinth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const UserAgent = "agmonetti/mcTUI/1.0.0 (https://github.com/agmonetti/mcTUI)"
const maxJSON = 10 << 20
const maxJAR = 250 << 20
const apiURL = "https://api.modrinth.com/v2"

type RateLimit struct {
	Limit, Remaining int
	Reset            time.Duration
}
type RateError struct{ Wait time.Duration }

func (e RateError) Error() string { return fmt.Sprintf("Modrinth rate limit: retry after %s", e.Wait) }

type Client struct {
	HTTP  *http.Client
	mu    sync.Mutex
	rate  RateLimit
	until time.Time
}

func NewClient() *Client { return &Client{} }
func allowed(u *url.URL, cdn bool) bool {
	if u == nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Opaque != "" {
		return false
	}
	if cdn {
		return u.Host == "cdn.modrinth.net"
	}
	return u.Host == "api.modrinth.com" && (u.Path == "/v2" || strings.HasPrefix(u.Path, "/v2/"))
}
func (c *Client) httpClient() *http.Client {
	base := c.HTTP
	if base == nil {
		base = http.DefaultClient
	}
	copy := *base
	copy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !allowed(req.URL, via[0].URL.Host == "cdn.modrinth.net") {
			return fmt.Errorf("unsafe Modrinth redirect: %s", req.URL)
		}
		return nil
	}
	return &copy
}
func rateSeconds(v string) time.Duration {
	if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
		return time.Duration(n * float64(time.Second))
	}
	if t, err := http.ParseTime(v); err == nil && time.Until(t) > 0 {
		return time.Until(t)
	}
	return 0
}
func (c *Client) request(ctx context.Context, method, target string, body []byte, cdn bool, timeout time.Duration) (*http.Response, error) {
	u, err := url.Parse(target)
	if err != nil || !allowed(u, cdn) {
		return nil, fmt.Errorf("unsafe Modrinth URL: %s", target)
	}
	c.mu.Lock()
	until := c.until
	c.mu.Unlock()
	if wait := time.Until(until); wait > 0 {
		return nil, RateError{wait}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &closingBody{ReadCloser: resp.Body, cancel: cancel}
	if !allowed(resp.Request.URL, cdn) {
		resp.Body.Close()
		return nil, fmt.Errorf("unsafe Modrinth response URL")
	}
	limit, _ := strconv.Atoi(resp.Header.Get("X-Ratelimit-Limit"))
	remaining, _ := strconv.Atoi(resp.Header.Get("X-Ratelimit-Remaining"))
	reset := rateSeconds(resp.Header.Get("X-Ratelimit-Reset"))
	c.mu.Lock()
	c.rate = RateLimit{limit, remaining, reset}
	if resp.StatusCode == http.StatusTooManyRequests {
		wait := rateSeconds(resp.Header.Get("Retry-After"))
		if wait == 0 {
			wait = reset
		}
		if wait == 0 {
			wait = time.Second
		}
		c.until = time.Now().Add(wait)
		c.mu.Unlock()
		resp.Body.Close()
		return nil, RateError{wait}
	}
	c.mu.Unlock()
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("Modrinth HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

type closingBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *closingBody) Close() error { err := b.ReadCloser.Close(); b.cancel(); return err }
func (c *Client) Rate() RateLimit   { c.mu.Lock(); defer c.mu.Unlock(); return c.rate }
func (c *Client) json(ctx context.Context, method, path string, input, output any) error {
	var b []byte
	if input != nil {
		var err error
		b, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	resp, err := c.request(ctx, method, apiURL+path, b, false, 15*time.Second)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.ContentLength > maxJSON {
		return fmt.Errorf("Modrinth JSON too large")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxJSON+1))
	if err != nil {
		return err
	}
	if len(data) > maxJSON {
		return fmt.Errorf("Modrinth JSON too large")
	}
	if err = json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("invalid Modrinth JSON: %w", err)
	}
	return nil
}
func (c *Client) batch(ctx context.Context, path string, input, output any) error {
	for attempt := 0; attempt < 3; attempt++ {
		err := c.json(ctx, "POST", path, input, output)
		var limited RateError
		if !errors.As(err, &limited) || attempt == 2 {
			return err
		}
		wait := max(limited.Wait, time.Duration(1<<attempt)*time.Second)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}
