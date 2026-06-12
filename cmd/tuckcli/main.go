// Command tuckcli is the Tuck command-line client.
//
// Configuration (flags take precedence over environment variables):
//
//	TUCK_ADDR   — server address, default http://127.0.0.1:8200
//	TUCK_TOKEN  — bearer token
//
// Usage:
//
//	tuckcli [--addr=…] [--token=…] [--insecure] <command> [args]
package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Version is set at build time via -ldflags "-X main.Version=x.y.z".
var Version = "dev"

// client is a thin HTTP wrapper for the Tuck API.
type client struct {
	addr     string
	token    string
	insecure bool
	http     *http.Client
}

func newClient(addr, token string, insecure bool) *client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return &client{
		addr:     strings.TrimRight(addr, "/"),
		token:    token,
		insecure: insecure,
		http:     &http.Client{Transport: tr, Timeout: 30 * time.Second},
	}
}

func (c *client) do(method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.addr+path, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("X-Tuck-Token", c.token)
	}
	return c.http.Do(req)
}

func (c *client) doRaw(method, path string, bodyReader io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.addr+path, bodyReader)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("X-Tuck-Token", c.token)
	}
	return c.http.Do(req)
}

func mustJSON(resp *http.Response, ok int) map[string]any {
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != ok {
		fatalf("HTTP %d: %s", resp.StatusCode, body)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		fatalf("parse response: %v", err)
	}
	return out
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

// ---- commands ----

func cmdStatus(c *client) {
	resp, err := c.do("GET", "/v1/sys/seal-status", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdUnseal(c *client, key string) {
	resp, err := c.do("POST", "/v1/sys/unseal", map[string]string{"key": key})
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdSeal(c *client) {
	resp, err := c.do("POST", "/v1/sys/seal", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdRotate(c *client) {
	resp, err := c.do("POST", "/v1/sys/rotate", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

// kv subcommands

func cmdKvGet(c *client, path string) {
	resp, err := c.do("GET", "/v1/secret/"+path, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	result := mustJSON(resp, 200)
	// API-1: handle base64-encoded binary values
	if enc, _ := result["encoding"].(string); enc == "base64" {
		fmt.Fprintf(os.Stderr, "note: value is binary (base64-encoded)\n")
	}
	printJSON(result)
}

func cmdKvPut(c *client, path, value string) {
	var bodyReader io.Reader
	if value == "-" {
		bodyReader = os.Stdin
	} else {
		bodyReader = strings.NewReader(value)
	}
	resp, err := c.doRaw("PUT", "/v1/secret/"+path, bodyReader)
	if err != nil {
		fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		fatalf("HTTP %d: %s", resp.StatusCode, body)
	}
	fmt.Println("OK")
}

func cmdKvDelete(c *client, path string) {
	resp, err := c.do("DELETE", "/v1/secret/"+path, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		fatalf("HTTP %d: %s", resp.StatusCode, body)
	}
	fmt.Println("OK")
}

func cmdKvList(c *client, prefix string) {
	resp, err := c.doRaw("LIST", "/v1/secret/"+prefix, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

// token subcommands

func cmdTokenCreate(c *client, name string, policies []string, ttl string, maxUses int) {
	req := map[string]any{
		"display_name": name,
		"policies":     policies,
	}
	if ttl != "" {
		req["ttl"] = ttl
	}
	if maxUses > 0 {
		req["max_uses"] = maxUses
	}
	resp, err := c.do("POST", "/v1/auth/token", req)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 201))
}

func cmdTokenGet(c *client, id string) {
	resp, err := c.do("GET", "/v1/auth/token/"+id, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdTokenRenew(c *client, id, ttl string) {
	body := map[string]string{}
	if ttl != "" {
		body["ttl"] = ttl
	}
	resp, err := c.do("POST", "/v1/auth/token/"+id+"/renew", body)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdTokenRevoke(c *client, id string) {
	resp, err := c.do("DELETE", "/v1/auth/token/"+id, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		fatalf("HTTP %d: %s", resp.StatusCode, body)
	}
	fmt.Println("OK")
}

func cmdTokenList(c *client) {
	resp, err := c.doRaw("LIST", "/v1/auth/token/", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

// policy subcommands

func cmdPolicyGet(c *client, name string) {
	resp, err := c.do("GET", "/v1/policy/"+name, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdPolicyDelete(c *client, name string) {
	resp, err := c.do("DELETE", "/v1/policy/"+name, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		fatalf("HTTP %d: %s", resp.StatusCode, body)
	}
	fmt.Println("OK")
}

func cmdPolicyList(c *client) {
	resp, err := c.doRaw("LIST", "/v1/policy/", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdPolicyPut(c *client, name string, rulesJSON string) {
	var rules any
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		fatalf("invalid JSON: %v", err)
	}
	resp, err := c.do("PUT", "/v1/policy/"+name, map[string]any{"rules": rules})
	if err != nil {
		fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		fatalf("HTTP %d: %s", resp.StatusCode, body)
	}
	fmt.Println("OK")
}

// ---- main ----

func usage() {
	fmt.Fprintf(os.Stderr, `tuckcli %s — Tuck secrets manager CLI

Usage: tuckcli [flags] <command> [args]

Global flags:
  --addr string     Server address (env TUCK_ADDR, default http://127.0.0.1:8200)
  --token string    Bearer token  (env TUCK_TOKEN)
  --insecure        Skip TLS certificate verification

Commands:
  status                          Show seal status
  unseal <key>                    Submit a Shamir unseal shard
  seal                            Re-seal the server
  rotate                          Rotate the root key (re-wraps barrier DEK)
  version                         Print version

  kv get <path>                   Get a secret
  kv put <path> <value|-stdin>    Set a secret (use '-' to read value from stdin)
  kv delete <path>                Delete a secret
  kv list [prefix]                List secrets

  token create [--name=n] [--policy=p ...] [--ttl=24h]
  token get <id>                  Look up a token
  token renew <id> [ttl]          Renew a token's TTL (default 1h)
  token revoke <id>               Revoke a token
  token list                      List all token IDs

  policy get <name>               Get a policy
  policy put <name> <json>        Set a policy (rules JSON array)
  policy delete <name>            Delete a policy
  policy list                     List all policy names
`, Version)
	os.Exit(2)
}

func main() {
	addr := flag.String("addr", envOr("TUCK_ADDR", "http://127.0.0.1:8200"), "server address")
	token := flag.String("token", os.Getenv("TUCK_TOKEN"), "bearer token")
	insecure := flag.Bool("insecure", false, "skip TLS verification")
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		usage()
	}

	c := newClient(*addr, *token, *insecure)

	switch args[0] {
	case "version":
		fmt.Printf("tuckcli %s\n", Version)

	case "status":
		cmdStatus(c)

	case "unseal":
		if len(args) < 2 {
			fatalf("unseal requires a key argument")
		}
		cmdUnseal(c, args[1])

	case "seal":
		cmdSeal(c)

	case "rotate":
		cmdRotate(c)

	case "kv":
		if len(args) < 2 {
			fatalf("kv requires a subcommand: get, put, delete, list")
		}
		switch args[1] {
		case "get":
			if len(args) < 3 {
				fatalf("kv get requires a path")
			}
			cmdKvGet(c, args[2])
		case "put":
			if len(args) < 4 {
				fatalf("kv put requires a path and value (or '-' for stdin)")
			}
			cmdKvPut(c, args[2], args[3])
		case "delete":
			if len(args) < 3 {
				fatalf("kv delete requires a path")
			}
			cmdKvDelete(c, args[2])
		case "list":
			prefix := ""
			if len(args) >= 3 {
				prefix = args[2]
			}
			cmdKvList(c, prefix)
		default:
			fatalf("unknown kv subcommand %q", args[1])
		}

	case "token":
		if len(args) < 2 {
			fatalf("token requires a subcommand: create, get, renew, revoke, list")
		}
		switch args[1] {
		case "create":
			fs := flag.NewFlagSet("token create", flag.ExitOnError)
			name := fs.String("name", "", "display name")
			ttl := fs.String("ttl", "", "TTL e.g. 24h (default never expires)")
			maxUses := fs.Int("max-uses", 0, "max number of uses (0 = unlimited)")
			var policies multiFlag
			fs.Var(&policies, "policy", "policy name (may be repeated)")
			_ = fs.Parse(args[2:])
			cmdTokenCreate(c, *name, []string(policies), *ttl, *maxUses)
		case "get":
			if len(args) < 3 {
				fatalf("token get requires an id")
			}
			cmdTokenGet(c, args[2])
		case "renew":
			if len(args) < 3 {
				fatalf("token renew requires an id")
			}
			ttl := ""
			if len(args) >= 4 {
				ttl = args[3]
			}
			cmdTokenRenew(c, args[2], ttl)
		case "revoke":
			if len(args) < 3 {
				fatalf("token revoke requires an id")
			}
			cmdTokenRevoke(c, args[2])
		case "list":
			cmdTokenList(c)
		default:
			fatalf("unknown token subcommand %q", args[1])
		}

	case "policy":
		if len(args) < 2 {
			fatalf("policy requires a subcommand: get, put, delete, list")
		}
		switch args[1] {
		case "get":
			if len(args) < 3 {
				fatalf("policy get requires a name")
			}
			cmdPolicyGet(c, args[2])
		case "put":
			if len(args) < 4 {
				fatalf("policy put requires a name and rules JSON")
			}
			rulesJSON := args[3]
			if rulesJSON == "-" {
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					fatalf("read stdin: %v", err)
				}
				rulesJSON = string(data)
			}
			cmdPolicyPut(c, args[2], rulesJSON)
		case "delete":
			if len(args) < 3 {
				fatalf("policy delete requires a name")
			}
			cmdPolicyDelete(c, args[2])
		case "list":
			cmdPolicyList(c)
		default:
			fatalf("unknown policy subcommand %q", args[1])
		}

	default:
		fatalf("unknown command %q — run tuckcli --help", args[0])
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// multiFlag is a flag.Value that can be set multiple times.
type multiFlag []string

func (m *multiFlag) String() string  { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

