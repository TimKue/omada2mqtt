// Package opnsense is a small client for the OPNsense API. It reads the
// Dnsmasq DHCP leases (and, later, static host mappings) so their hostnames can
// be pushed into other systems. Authentication is HTTP Basic with an API
// key/secret pair (System > Access > Users > API keys).
package opnsense

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a single OPNsense instance. It is safe for concurrent use.
type Client struct {
	baseURL    string
	key        string
	secret     string
	httpClient *http.Client
}

// Option customises a Client.
type Option func(*Client)

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithInsecureTLS disables TLS certificate verification, for the self-signed
// certificate OPNsense ships with. Do not use it over the internet.
func WithInsecureTLS() Option {
	return func(c *Client) {
		c.httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // opt-in for local self-signed certs
		}
	}
}

// NewClient builds a client for the OPNsense at baseURL (e.g.
// "https://192.168.178.1"). key and secret are an API key pair.
func NewClient(baseURL, key, secret string, opts ...Option) *Client {
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		key:        key,
		secret:     secret,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

const leasesSearchPath = "/api/dnsmasq/leases/search"

// Leases returns the current Dnsmasq DHCP leases.
func (c *Client) Leases(ctx context.Context) ([]Lease, error) {
	var resp leaseSearch
	if err := c.get(ctx, leasesSearchPath, &resp); err != nil {
		return nil, err
	}
	return resp.Rows, nil
}

// RawLeases returns the unprocessed JSON of the leases search, for discovering
// fields not yet modelled in Lease.
func (c *Client) RawLeases(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.get(ctx, leasesSearchPath, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// get performs an authenticated GET and unmarshals the JSON body into out.
func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(c.key, c.secret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
