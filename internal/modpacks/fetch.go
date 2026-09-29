package modpacks

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const fetchUserAgent = "agmonetti/mcTUI/1.0.0 (https://github.com/agmonetti/mcTUI)"
const maxFetchRedirects = 5

var fetchHosts = map[string]bool{
	"cdn.modrinth.net": true,
	"cdn.modrinth.com": true,
	"github.com": true,
	"raw.githubusercontent.com": true,
	"gitlab.com": true,
}

// AllowedURL checks an initial modpack download URL. Redirects are checked
// separately, including their origin, before any connection is made.
func AllowedURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !safeURL(u) || !fetchHosts[u.Host] {
		return errors.New("unsafe modpack download URL")
	}
	return nil
}

func safeURL(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && u.User == nil && u.Opaque == "" &&
		u.Host != "" && u.Host == u.Hostname() && u.Port() == "" &&
		u.Fragment == "" && u.Host == strings.ToLower(u.Host)
}

func allowedFetchRedirect(u *url.URL, previous string) error {
	if !safeURL(u) {
		return errors.New("unsafe modpack redirect")
	}
	if fetchHosts[u.Host] || u.Host == "release-assets.githubusercontent.com" &&
		(previous == "github.com" || previous == "release-assets.githubusercontent.com") {
		return nil
	}
	return errors.New("unsafe modpack redirect")
}

// Fetch downloads to a private temporary file in dir; the caller owns that
// file on success. A zero expectedSize means the size is unknown. maxSize must
// be positive, including for a response without a Content-Length header.
// URLs are alternatives, never a reason to bypass a failed safety check.
func Fetch(ctx context.Context, client *http.Client, urls []string, hashes Hashes, expectedSize, maxSize int64, dir string) (string, error) {
	return fetchWithPolicy(ctx, client, urls, hashes, expectedSize, maxSize, dir, nil, nil)
}

// fetchWithPolicy allows tests to supply both a trusted local TLS transport and
// an explicit local URL policy. Fetch itself always uses the production policy.
func fetchWithPolicy(ctx context.Context, client *http.Client, urls []string, hashes Hashes, expectedSize, maxSize int64, dir string, allowed func(*url.URL, string) error, trustedTransport http.RoundTripper) (string, error) {
	if maxSize <= 0 || expectedSize < 0 || expectedSize > maxSize {
		return "", errors.New("invalid modpack download size limit")
	}
	if len(urls) == 0 {
		return "", errors.New("no modpack download URLs")
	}
	shaOne, err := decodeFetchHash(hashes.SHA1, sha1.Size)
	if err != nil {
		return "", fmt.Errorf("invalid SHA-1: %w", err)
	}
	shaFiveTwelve, err := decodeFetchHash(hashes.SHA512, sha512.Size)
	if err != nil {
		return "", fmt.Errorf("invalid SHA-512: %w", err)
	}
	if allowed == nil {
		allowed = func(u *url.URL, previous string) error {
			if previous == "" {
				return AllowedURL(u.String())
			}
			return allowedFetchRedirect(u, previous)
		}
	}
	for _, raw := range urls {
		u, parseErr := url.Parse(raw)
		if parseErr != nil || allowed(u, "") != nil {
			return "", errors.New("unsafe modpack download URL")
		}
	}

	var base http.Client
	if client != nil {
		base = *client
	}
	if trustedTransport != nil {
		base.Transport = trustedTransport
	} else {
		transport, transportErr := secureFetchTransport(base.Transport)
		if transportErr != nil {
			return "", transportErr
		}
		base.Transport = transport
		defer transport.CloseIdleConnections()
	}
	base.Jar = nil
	base.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxFetchRedirects || allowed(req.URL, via[len(via)-1].URL.Host) != nil {
			return errors.New("unsafe modpack redirect")
		}
		// net/http otherwise forwards the previous URL's query as Referer.
		req.Header.Del("Referer")
		return nil
	}

	var lastErr error
	for _, raw := range urls {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		name, err := fetchOne(ctx, &base, raw, shaOne, shaFiveTwelve, expectedSize, maxSize, dir)
		if err == nil {
			return name, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		lastErr = err
	}
	return "", fmt.Errorf("all modpack download sources failed: %w", lastErr)
}

func decodeFetchHash(raw string, length int) ([]byte, error) {
	if raw == "" {
		return nil, nil
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != length {
		return nil, errors.New("incorrect hash encoding")
	}
	return decoded, nil
}

func fetchOne(ctx context.Context, client *http.Client, raw string, expectedSHA1, expectedSHA512 []byte, expectedSize, maxSize int64, dir string) (name string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", errors.New("invalid modpack download request")
	}
	req.Header.Set("User-Agent", fetchUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		// net/http errors include the complete URL, including private query tokens.
		return "", errors.New("modpack download request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("modpack download HTTP %d", resp.StatusCode)
	}
	if strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return "", errors.New("modpack download returned HTML")
	}
	if resp.ContentLength > maxSize || expectedSize > 0 && resp.ContentLength >= 0 && resp.ContentLength != expectedSize {
		return "", errors.New("modpack download size mismatch")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create modpack download directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".mctui-download-*")
	if err != nil {
		return "", fmt.Errorf("create modpack download temporary file: %w", err)
	}
	defer func() {
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	var one, fiveTwelve hash.Hash
	writer := io.Writer(tmp)
	if expectedSHA1 != nil {
		one = sha1.New()
		writer = io.MultiWriter(writer, one)
	}
	if expectedSHA512 != nil {
		fiveTwelve = sha512.New()
		writer = io.MultiWriter(writer, fiveTwelve)
	}
	written, copyErr := io.CopyBuffer(writer, io.LimitReader(resp.Body, maxSize+1), make([]byte, 32<<10))
	if ctx.Err() != nil {
		err = ctx.Err()
	} else if copyErr != nil {
		if errors.Is(copyErr, io.ErrUnexpectedEOF) {
			err = errors.New("truncated modpack download")
		} else {
			err = errors.New("modpack download stream failed")
		}
	} else if written > maxSize {
		err = errors.New("modpack download size limit exceeded")
	} else if resp.ContentLength >= 0 && written != resp.ContentLength {
		err = errors.New("truncated modpack download")
	} else if expectedSize > 0 && written != expectedSize {
		err = errors.New("modpack download size mismatch")
	} else if !matchFetchHash(one, expectedSHA1) || !matchFetchHash(fiveTwelve, expectedSHA512) {
		err = errors.New("modpack download hash mismatch")
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	return filepath.Clean(tmp.Name()), nil
}

func matchFetchHash(h hash.Hash, expected []byte) bool {
	return expected == nil || subtle.ConstantTimeCompare(h.Sum(nil), expected) == 1
}

// A fresh transport, rather than a clone of a caller-controlled one, prevents
// proxies, registered alternate protocols and TLS/dial hooks from bypassing
// validation of the address actually used for each connection.
func secureFetchTransport(original http.RoundTripper) (*http.Transport, error) {
	if original != nil {
		if _, ok := original.(*http.Transport); !ok {
			return nil, errors.New("modpack downloads require an HTTP transport")
		}
	}
	transport := &http.Transport{
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || port != "443" || !fetchHosts[host] && host != "release-assets.githubusercontent.com" {
			return nil, errors.New("unsafe modpack download destination")
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, errors.New("modpack download DNS lookup failed")
		}
		var dialer net.Dialer
		for _, address := range addresses {
			ip, ok := netip.AddrFromSlice(address.IP)
			if !ok || !publicFetchIP(ip) {
				continue
			}
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
		return nil, errors.New("modpack download has no reachable public address")
	}
	return transport, nil
}

var blockedFetchIPs = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001::/32"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"),
}

func publicFetchIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, block := range blockedFetchIPs {
		if block.Contains(ip) {
			return false
		}
	}
	return true
}
