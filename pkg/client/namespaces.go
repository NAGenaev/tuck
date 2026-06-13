package client

import "context"

// Namespace describes a Tuck namespace.
type Namespace struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at,omitempty"`
}

// Namespaces returns a client for namespace management.
func (c *Client) Namespaces() *NamespaceClient { return &NamespaceClient{c: c} }

// NamespaceClient manages Tuck namespaces.
type NamespaceClient struct{ c *Client }

// Create creates a new namespace.
func (n *NamespaceClient) Create(ctx context.Context, name string) (*Namespace, error) {
	var ns Namespace
	if err := n.c.post(ctx, "/v1/sys/namespaces", map[string]string{"name": name}, &ns); err != nil {
		return nil, err
	}
	return &ns, nil
}

// Get returns a namespace by name.
func (n *NamespaceClient) Get(ctx context.Context, name string) (*Namespace, error) {
	var ns Namespace
	return &ns, n.c.get(ctx, "/v1/sys/namespaces/"+name, &ns)
}

// Delete removes a namespace and all its data.
func (n *NamespaceClient) Delete(ctx context.Context, name string) error {
	return n.c.doDelete(ctx, "/v1/sys/namespaces/"+name)
}

// List returns all namespace names.
func (n *NamespaceClient) List(ctx context.Context) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	return resp.Keys, n.c.doList(ctx, "/v1/sys/namespaces/", &resp)
}
