package sources

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

// MaxDownloadBytes bounds a single download. The largest registry today is
// about 2.5 MB, so this leaves generous headroom while making it impossible
// for a redirected or replaced endpoint to stream unbounded data into memory.
const MaxDownloadBytes = 64 << 20 // 64 MiB

// userAgent identifies the client to the publishers. Several of them reject
// requests without one.
const userAgent = "iban.pizza data updater (+https://iban.pizza)"

// Client fetches source files over HTTP.
type Client struct {
	HTTP *http.Client
}

// NewClient returns a Client with timeouts set. The default http.Client has no
// timeout at all, which would let one unresponsive publisher hang an update
// run indefinitely.
func NewClient() *Client {
	return &Client{
		HTTP: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

// Get downloads a URL and returns its body.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %s", url, resp.Status)
	}

	// Deliberately not checking Content-Type. The Czech National Bank serves a
	// perfectly good CSV labelled text/html, so trusting the header would drop
	// valid data.
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxDownloadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxDownloadBytes {
		return nil, fmt.Errorf("GET %s exceeded the %d byte limit", url, MaxDownloadBytes)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("GET %s returned an empty body", url)
	}
	return body, nil
}

// ResolveLink downloads a landing page and returns the first link matching
// pattern, resolved against base.
//
// Some publishers put a content hash in the download path, so the URL changes
// with every release and cannot be hard coded. Hard coding one is how the
// upstream project ended up with dead links.
func (c *Client) ResolveLink(ctx context.Context, pageURL string, pattern *regexp.Regexp) (string, error) {
	page, err := c.Get(ctx, pageURL)
	if err != nil {
		return "", fmt.Errorf("load %s: %w", pageURL, err)
	}

	match := pattern.FindSubmatch(page)
	if match == nil {
		return "", fmt.Errorf("no link matching %s on %s", pattern, pageURL)
	}
	href := string(match[len(match)-1])

	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	resolved, err := req.URL.Parse(href)
	if err != nil {
		return "", fmt.Errorf("resolve %q against %s: %w", href, pageURL, err)
	}
	return resolved.String(), nil
}
