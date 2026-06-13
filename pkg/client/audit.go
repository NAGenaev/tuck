package client

import "context"

// AuditSink describes a registered audit sink.
type AuditSink struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	Options map[string]string `json:"options,omitempty"`
	Errors  int               `json:"errors"`
}

// Audit returns a client for audit sink management.
func (c *Client) Audit() *AuditClient { return &AuditClient{c: c} }

// AuditClient manages Tuck audit sinks.
type AuditClient struct{ c *Client }

// EnableWebhook registers (or updates) a webhook audit sink.
// timeoutSec 0 defaults to 5 seconds on the server.
func (a *AuditClient) EnableWebhook(ctx context.Context, name, url string, timeoutSec int) error {
	return a.c.putJSON(ctx, "/v1/sys/audit/webhook/"+name, map[string]any{
		"url":         url,
		"timeout_sec": timeoutSec,
	})
}

// Disable removes a named audit sink.
func (a *AuditClient) Disable(ctx context.Context, name string) error {
	return a.c.doDelete(ctx, "/v1/sys/audit/"+name)
}

// List returns all registered audit sinks with their error counts.
func (a *AuditClient) List(ctx context.Context) ([]AuditSink, error) {
	var resp struct {
		Sinks []AuditSink `json:"sinks"`
	}
	if err := a.c.doList(ctx, "/v1/sys/audit/", &resp); err != nil {
		return nil, err
	}
	return resp.Sinks, nil
}
