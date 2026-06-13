package client

import "context"

// CubbyholeClient provides access to the per-token cubbyhole secret space.
// Each token has its own isolated cubbyhole that is destroyed when the token
// is revoked.
type CubbyholeClient struct{ c *Client }

// Cubbyhole returns a client for cubbyhole operations.
func (c *Client) Cubbyhole() *CubbyholeClient { return &CubbyholeClient{c: c} }

// Get reads the value stored at the cubbyhole path.
// Returns (nil, nil) if the path does not exist.
func (ch *CubbyholeClient) Get(ctx context.Context, path string) ([]byte, error) {
	var resp struct {
		Value string `json:"value"`
	}
	if err := ch.c.get(ctx, "/v1/cubbyhole/"+trimSlash(path), &resp); err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return []byte(resp.Value), nil
}

// Put stores value at the cubbyhole path.
func (ch *CubbyholeClient) Put(ctx context.Context, path string, value []byte) error {
	return ch.c.putRaw(ctx, "/v1/cubbyhole/"+trimSlash(path), value)
}

// Delete removes a cubbyhole entry.
func (ch *CubbyholeClient) Delete(ctx context.Context, path string) error {
	return ch.c.doDelete(ctx, "/v1/cubbyhole/"+trimSlash(path))
}

// List returns the keys stored under the given cubbyhole prefix.
func (ch *CubbyholeClient) List(ctx context.Context, prefix string) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	if err := ch.c.doList(ctx, "/v1/cubbyhole/"+trimSlash(prefix), &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}
