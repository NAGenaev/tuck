package client

import "context"

// WrappingToken is a single-use token wrapping a secret payload.
type WrappingToken struct {
	Token     string `json:"token"`
	TTL       int    `json:"ttl"`
	ExpiresAt string `json:"expires_at"`
}

// Wrapping returns a client for response-wrapping operations.
func (c *Client) Wrapping() *WrappingClient { return &WrappingClient{c: c} }

// WrappingClient provides response-wrapping (single-use token) operations.
type WrappingClient struct{ c *Client }

// Wrap encodes payload as a single-use wrapping token.
// payload must be a JSON-serialisable value.
func (w *WrappingClient) Wrap(ctx context.Context, payload any) (*WrappingToken, error) {
	var wt WrappingToken
	return &wt, w.c.post(ctx, "/v1/sys/wrapping/wrap", payload, &wt)
}

// Lookup returns metadata about a wrapping token without consuming it.
func (w *WrappingClient) Lookup(ctx context.Context, token string) (*WrappingToken, error) {
	var wt WrappingToken
	return &wt, w.c.post(ctx, "/v1/sys/wrapping/lookup", map[string]string{"token": token}, &wt)
}

// Unwrap consumes a wrapping token and returns the wrapped payload.
// The payload is decoded into out (must be a pointer).
func (w *WrappingClient) Unwrap(ctx context.Context, token string, out any) error {
	return w.c.post(ctx, "/v1/sys/wrapping/unwrap", map[string]string{"token": token}, out)
}

// Revoke invalidates a wrapping token without revealing its payload.
func (w *WrappingClient) Revoke(ctx context.Context, token string) error {
	return w.c.post(ctx, "/v1/sys/wrapping/revoke", map[string]string{"token": token}, nil)
}
