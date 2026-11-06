package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"streamdrop/internal/apierr"
)

// DefaultTimeout bounds every request made with a zero-value Client.
const DefaultTimeout = 30 * time.Second

// Client is a typed, token-authenticated HTTP client.
type Client struct {
	ep Endpoint
	hc *http.Client
}

// New builds a client. The zero-value DefaultTimeout applies unless an
// http.Client is supplied via WithHTTPClient.
func New(ep Endpoint, opts ...Option) *Client {
	c := &Client{ep: ep, hc: &http.Client{Timeout: DefaultTimeout}}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the default transport/timeout.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.hc = hc }
}

// Dial discovers a running daemon and returns a client for it.
func Dial(dataDir string, opts ...Option) (*Client, error) {
	ep, err := Discover(dataDir)
	if err != nil {
		return nil, err
	}
	return New(*ep, opts...), nil
}

// APIError is a machine-readable error returned by the daemon. The Code is
// stable and matches the apierr catalog; Details is technical and is never
// meant to be shown to end users as primary text.
type APIError struct {
	StatusCode int
	Code       apierr.Code
	Kind       apierr.Kind
	Params     map[string]any
	Details    string
}

func (e *APIError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s (http %d): %s", e.Code, e.StatusCode, e.Details)
	}
	return fmt.Sprintf("%s (http %d)", e.Code, e.StatusCode)
}

// IsCode reports whether the error carries the given code.
func (e *APIError) IsCode(code apierr.Code) bool { return e.Code == code }

// TransportError means the daemon could not be reached (not running, wrong
// port, network failure).
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("streamdrop daemon unreachable: %v", e.Err)
}
func (e *TransportError) Unwrap() error { return e.Err }

func (c *Client) do(ctx context.Context, method, path, query string, body, out any) error {
	u := c.ep.BaseURL + "/api/v1" + path
	if query != "" {
		u += "?" + query
	}
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("client: encode body: %w", err)
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return fmt.Errorf("client: build request: %w", err)
	}
	req.Header.Set("X-StreamDrop-Token", c.ep.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return &TransportError{Err: err}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return &TransportError{Err: fmt.Errorf("read response: %w", err)}
	}

	if resp.StatusCode >= 400 {
		var env apierr.Envelope
		if json.Unmarshal(data, &env) == nil && env.Error.Code != "" {
			return apiErrorFromPayload(resp.StatusCode, env.Error)
		}
		return &APIError{StatusCode: resp.StatusCode, Code: apierr.CodeInternalError, Details: string(data)}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("client: decode response: %w", err)
		}
	}
	return nil
}

// apiErrorFromPayload builds an APIError from a wire payload.
func apiErrorFromPayload(status int, p apierr.Payload) *APIError {
	return &APIError{
		StatusCode: status,
		Code:       p.Code,
		Kind:       p.Kind,
		Params:     p.Params,
		Details:    p.Details,
	}
}

func (c *Client) get(ctx context.Context, path, query string, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, "", body, out)
}

func (c *Client) put(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPut, path, "", body, out)
}

func (c *Client) delete(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodDelete, path, "", nil, out)
}
