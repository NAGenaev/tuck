package client

import "context"

// MountClient manages secret engine mounts.
type MountClient struct{ c *Client }

// Mounts returns a client for secret engine mount management.
func (c *Client) Mounts() *MountClient { return &MountClient{c: c} }

// Mount describes a mounted secret engine.
type Mount struct {
	Path        string `json:"path"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Accessor    string `json:"accessor,omitempty"`
	Builtin     bool   `json:"builtin"`
}

// List returns all currently mounted secret engines.
func (m *MountClient) List(ctx context.Context) ([]Mount, error) {
	var resp struct {
		Mounts []Mount `json:"mounts"`
	}
	if err := m.c.get(ctx, "/v1/sys/mounts", &resp); err != nil {
		return nil, err
	}
	return resp.Mounts, nil
}

// Enable mounts a secret engine at the given path.
func (m *MountClient) Enable(ctx context.Context, path, engineType, description string) error {
	return m.c.post(ctx, "/v1/sys/mounts/"+path, map[string]string{
		"type":        engineType,
		"description": description,
	}, nil)
}

// Disable unmounts a secret engine at the given path.
func (m *MountClient) Disable(ctx context.Context, path string) error {
	return m.c.doDelete(ctx, "/v1/sys/mounts/"+path)
}
