// Package omada is a small, dependency-free client for the TP-Link Omada
// Open API. It uses the OAuth2 client-credentials ("Client Mode") flow and is
// intended to run locally against a self-hosted controller.
//
// The exact endpoint paths are declared as constants below so they are easy to
// cross-check against the controller's built-in "Online API Document"
// (Settings > Platform Integration > Open API, top-right).
package omada

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Endpoint paths. tokenPath is version-less; data APIs live under apiV1.
const (
	tokenPath = "/openapi/authorize/token" //nolint:gosec // path, not a credential
	apiV1     = "/openapi/v1"
)

// tokenLeeway is subtracted from the reported token lifetime so we refresh
// slightly early and never present an almost-expired token.
const tokenLeeway = 60 * time.Second

// Client talks to a single Omada controller. It is safe for concurrent use.
type Client struct {
	baseURL      string
	omadacID     string
	clientID     string
	clientSecret string
	httpClient   *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// Option customises a Client.
type Option func(*Client)

// WithHTTPClient overrides the default HTTP client (e.g. to inject a custom
// transport or timeout).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithInsecureTLS disables TLS certificate verification. Local Omada
// controllers ship a self-signed certificate, so this is commonly required for
// on-prem use. Do not use it against a controller reachable over the internet.
func WithInsecureTLS() Option {
	return func(c *Client) {
		c.httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // opt-in for local self-signed certs
		}
	}
}

// NewClient builds a client for the controller at baseURL (e.g.
// "https://192.168.178.2:8043"). omadacID, clientID and clientSecret come from
// the Open API application you created in the controller.
func NewClient(baseURL, omadacID, clientID, clientSecret string, opts ...Option) *Client {
	c := &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		omadacID:     omadacID,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ListSites returns the sites visible to the API application. It is the
// simplest read call and doubles as a connectivity/auth smoke test.
func (c *Client) ListSites(ctx context.Context) ([]Site, error) {
	path := fmt.Sprintf("%s/%s/sites?page=1&pageSize=100", apiV1, c.omadacID)
	var p page[Site]
	if err := c.get(ctx, path, &p); err != nil {
		return nil, err
	}
	return p.Data, nil
}

func (c *Client) devicesPath(siteID string) string {
	return fmt.Sprintf("%s/%s/sites/%s/devices?page=1&pageSize=100", apiV1, c.omadacID, siteID)
}

// ListDevices returns the adopted devices in a site. The Open API has returned
// device lists both as a bare array and as a paginated object across versions,
// so we accept either shape.
func (c *Client) ListDevices(ctx context.Context, siteID string) ([]Device, error) {
	var raw json.RawMessage
	if err := c.get(ctx, c.devicesPath(siteID), &raw); err != nil {
		return nil, err
	}

	var arr []Device
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	var p page[Device]
	if err := json.Unmarshal(raw, &p); err == nil {
		return p.Data, nil
	}
	return nil, fmt.Errorf("decode devices: unexpected result shape: %s", truncate(raw, 200))
}

// RawDevices returns the unprocessed JSON of a site's device list. Useful for
// discovering fields not yet modelled in Device.
func (c *Client) RawDevices(ctx context.Context, siteID string) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.get(ctx, c.devicesPath(siteID), &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *Client) clientsPath(siteID string) string {
	return fmt.Sprintf("%s/%s/sites/%s/clients?page=1&pageSize=1000", apiV1, c.omadacID, siteID)
}

// ListClients returns the clients currently connected to a site.
func (c *Client) ListClients(ctx context.Context, siteID string) ([]ClientInfo, error) {
	var p page[ClientInfo]
	if err := c.get(ctx, c.clientsPath(siteID), &p); err != nil {
		return nil, err
	}
	return p.Data, nil
}

// RawClients returns the unprocessed JSON of a site's client list, for
// discovering fields not yet modelled in ClientInfo.
func (c *Client) RawClients(ctx context.Context, siteID string) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.get(ctx, c.clientsPath(siteID), &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// SetClientName sets (or updates) the display name of a client, identified by
// MAC, via PATCH .../clients/{mac}/name with body {"name": ...}. The MAC must be
// in the controller's own format (uppercase, dash-separated), as returned by
// ListClients.
func (c *Client) SetClientName(ctx context.Context, siteID, mac, name string) error {
	path := fmt.Sprintf("%s/%s/sites/%s/clients/%s/name", apiV1, c.omadacID, siteID, url.PathEscape(mac))

	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return fmt.Errorf("marshal name: %w", err)
	}

	token, err := c.ensureToken(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "AccessToken="+token)
	req.Header.Set("Content-Type", "application/json")

	return c.do(req, nil)
}

func (c *Client) switchPortsPoePath(siteID string) string {
	return fmt.Sprintf("%s/%s/sites/%s/switches/ports/poe-info?page=1&pageSize=1000", apiV1, c.omadacID, siteID)
}

// ListSwitchPortsPoe returns the PoE state of every switch port in the site
// (GET /openapi/v1/{omadacId}/sites/{siteId}/switches/ports/poe-info). This is
// where per-port PoE power lives — the device list/detail do not carry it.
func (c *Client) ListSwitchPortsPoe(ctx context.Context, siteID string) ([]SwitchPort, error) {
	var p page[SwitchPort]
	if err := c.get(ctx, c.switchPortsPoePath(siteID), &p); err != nil {
		return nil, err
	}
	return p.Data, nil
}

// RawSwitchPortsPoe returns the unprocessed JSON of the switch PoE port grid,
// for discovering fields not yet modelled in SwitchPort.
func (c *Client) RawSwitchPortsPoe(ctx context.Context, siteID string) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.get(ctx, c.switchPortsPoePath(siteID), &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// RawDevice returns the unprocessed JSON of a single device's detail (by MAC),
// which — unlike the list — carries PoE and per-port data. The Open API exposes
// device detail under resource-type-specific paths that vary, so we try a few
// candidates and return the first that responds, along with the path that hit.
func (c *Client) RawDevice(ctx context.Context, siteID, mac string) (string, json.RawMessage, error) {
	base := fmt.Sprintf("%s/%s/sites/%s", apiV1, c.omadacID, siteID)
	candidates := []string{
		base + "/switches/" + mac,
		base + "/eaps/" + mac,
		base + "/gateways/" + mac,
		base + "/devices/" + mac,
	}
	var lastErr error
	for _, p := range candidates {
		var raw json.RawMessage
		if err := c.get(ctx, p, &raw); err != nil {
			lastErr = err
			continue
		}
		return p, raw, nil
	}
	return "", nil, lastErr
}

// truncate returns at most n bytes of b as a string, for compact error output.
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// get performs an authenticated GET and unmarshals the envelope's result into
// out. It refreshes the access token on demand.
func (c *Client) get(ctx context.Context, path string, out any) error {
	token, err := c.ensureToken(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	// Omada expects the literal prefix "AccessToken=" in the Authorization header.
	req.Header.Set("Authorization", "AccessToken="+token)

	return c.do(req, out)
}

// ensureToken returns a valid access token, fetching a new one if the cached
// token is missing or about to expire.
func (c *Client) ensureToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}

	body, err := json.Marshal(map[string]string{
		"omadacId":      c.omadacID,
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
	})
	if err != nil {
		return "", fmt.Errorf("marshal token request: %w", err)
	}

	url := c.baseURL + tokenPath + "?grant_type=client_credentials"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	var tr tokenResult
	if err := c.do(req, &tr); err != nil {
		return "", fmt.Errorf("authenticate: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("authenticate: empty access token in response")
	}

	c.token = tr.AccessToken
	c.tokenExp = time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second - tokenLeeway)
	return c.token, nil
}

// do sends req, checks the HTTP status and the Omada error envelope, and
// unmarshals result into out.
func (c *Client) do(req *http.Request, out any) error {
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

	var env apiEnvelope[json.RawMessage]
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}
	if env.ErrorCode != 0 {
		return fmt.Errorf("omada API error %d: %s", env.ErrorCode, env.Msg)
	}
	if out == nil || len(env.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Result, out); err != nil {
		return fmt.Errorf("decode result: %w", err)
	}
	return nil
}
