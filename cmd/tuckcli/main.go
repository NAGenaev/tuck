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
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// Version is set at build time via -ldflags "-X main.Version=x.y.z".
var Version = "dev"

// client is a thin HTTP wrapper for the Tuck API.
type client struct {
	addr      string
	token     string
	namespace string
	insecure  bool
	http      *http.Client
}

func newClient(addr, token, ns string, insecure bool) *client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 — gated by explicit --insecure flag
	}
	return &client{
		addr:      strings.TrimRight(addr, "/"),
		token:     token,
		namespace: ns,
		insecure:  insecure,
		http:      &http.Client{Transport: tr, Timeout: 30 * time.Second},
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
	req, err := http.NewRequest(method, c.addr+path, bodyReader) // #nosec G704 — CLI tool; addr is user-supplied server URL
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("X-Tuck-Token", c.token)
	}
	if c.namespace != "" {
		req.Header.Set("X-Tuck-Namespace", c.namespace)
	}
	return c.http.Do(req) // #nosec G704
}

func (c *client) doRaw(method, path string, bodyReader io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.addr+path, bodyReader) // #nosec G704 — CLI tool; addr is user-supplied server URL
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("X-Tuck-Token", c.token)
	}
	return c.http.Do(req) // #nosec G704
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

// ---- token self ops ----

func cmdTokenLookupSelf(c *client) {
	resp, err := c.do("GET", "/v1/auth/token/lookup-self", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdTokenRenewSelf(c *client, ttl string) {
	body := map[string]string{}
	if ttl != "" {
		body["ttl"] = ttl
	}
	resp, err := c.do("POST", "/v1/auth/token/renew-self", body)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

// ---- dynamic credentials ----

func cmdDynCreds(c *client, engine, role string) {
	resp, err := c.do("POST", "/v1/"+engine+"/creds/"+role, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

// ---- pki ----

func cmdPKIIssue(c *client, role, cn, ttl string, altNames []string) {
	body := map[string]any{"common_name": cn}
	if ttl != "" {
		body["ttl"] = ttl
	}
	if len(altNames) > 0 {
		body["alt_names"] = strings.Join(altNames, ",")
	}
	resp, err := c.do("POST", "/v1/pki/issue/"+role, body)
	if err != nil {
		fatalf("request: %v", err)
	}
	result := mustJSON(resp, 200)
	if cert, ok := result["certificate"].(string); ok {
		fmt.Print(cert)
		return
	}
	printJSON(result)
}

func cmdPKIRevoke(c *client, serial string) {
	resp, err := c.do("POST", "/v1/pki/revoke/"+serial, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

// ---- transit ----

func cmdTransitEncrypt(c *client, key, plaintext string) {
	if plaintext == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatalf("read stdin: %v", err)
		}
		plaintext = string(data)
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(plaintext))
	resp, err := c.do("POST", "/v1/transit/encrypt/"+key, map[string]string{"plaintext": b64})
	if err != nil {
		fatalf("request: %v", err)
	}
	result := mustJSON(resp, 200)
	if ct, ok := result["ciphertext"].(string); ok {
		fmt.Println(ct)
		return
	}
	printJSON(result)
}

func cmdTransitDecrypt(c *client, key, ciphertext string) {
	if ciphertext == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatalf("read stdin: %v", err)
		}
		ciphertext = strings.TrimSpace(string(data))
	}
	resp, err := c.do("POST", "/v1/transit/decrypt/"+key, map[string]string{"ciphertext": ciphertext})
	if err != nil {
		fatalf("request: %v", err)
	}
	result := mustJSON(resp, 200)
	if pt, ok := result["plaintext"].(string); ok {
		decoded, err := base64.StdEncoding.DecodeString(pt)
		if err != nil {
			fmt.Println(pt)
			return
		}
		fmt.Print(string(decoded))
		return
	}
	printJSON(result)
}

// ---- ssh ----

func cmdSSHSign(c *client, role, pubkeyPath, ttl string) {
	var pubkey string
	if pubkeyPath == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatalf("read stdin: %v", err)
		}
		pubkey = strings.TrimSpace(string(data))
	} else {
		data, err := os.ReadFile(pubkeyPath) // #nosec G304 — user-supplied public key file path in CLI tool
		if err != nil {
			fatalf("read pubkey file %q: %v", pubkeyPath, err)
		}
		pubkey = strings.TrimSpace(string(data))
	}
	body := map[string]string{"public_key": pubkey}
	if ttl != "" {
		body["ttl"] = ttl
	}
	resp, err := c.do("POST", "/v1/ssh/sign/"+role, body)
	if err != nil {
		fatalf("request: %v", err)
	}
	result := mustJSON(resp, 200)
	if cert, ok := result["signed_key"].(string); ok {
		fmt.Print(cert)
		return
	}
	printJSON(result)
}

// ---- totp ----

func cmdTOTPCode(c *client, key string) {
	resp, err := c.do("GET", "/v1/totp/code/"+key, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	result := mustJSON(resp, 200)
	if code, ok := result["code"].(string); ok {
		fmt.Println(code)
		return
	}
	printJSON(result)
}

// ---- auth logins ----

func cmdAuthAppRoleLogin(c *client, roleID, secretID string) {
	resp, err := c.do("POST", "/v1/auth/approle/login", map[string]string{
		"role_id":   roleID,
		"secret_id": secretID,
	})
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdAuthLDAPLogin(c *client, username, password string) {
	resp, err := c.do("POST", "/v1/auth/ldap/login", map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdAuthJWTLogin(c *client, jwt, role string) {
	body := map[string]string{"jwt": jwt}
	if role != "" {
		body["role"] = role
	}
	resp, err := c.do("POST", "/v1/auth/jwt/login", body)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

// ---- GitHub Actions OIDC auth ----

func cmdAuthGitHubLogin(c *client, token, role string) {
	resp, err := c.do("POST", "/v1/auth/github/login", map[string]string{"token": token, "role": role})
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdAuthGitHubRoleCreate(c *client, name, repo, owner, ref, env, workflowRef, actor, audience string, policies []string, ttl string) {
	body := map[string]any{
		"policies": policies,
	}
	if repo != "" {
		body["repository"] = repo
	}
	if owner != "" {
		body["repository_owner"] = owner
	}
	if ref != "" {
		body["ref"] = ref
	}
	if env != "" {
		body["environment"] = env
	}
	if workflowRef != "" {
		body["workflow_ref"] = workflowRef
	}
	if actor != "" {
		body["actor"] = actor
	}
	if audience != "" {
		body["audience"] = audience
	}
	if ttl != "" {
		body["ttl"] = ttl
	}
	resp, err := c.do("PUT", "/v1/auth/github/role/"+name, body)
	if err != nil {
		fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body2, _ := io.ReadAll(resp.Body)
		fatalf("HTTP %d: %s", resp.StatusCode, body2)
	}
	fmt.Printf("GitHub role %q saved\n", name)
}

func cmdAuthGitHubRoleGet(c *client, name string) {
	resp, err := c.do("GET", "/v1/auth/github/role/"+name, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdAuthGitHubRoleDelete(c *client, name string) {
	resp, err := c.do("DELETE", "/v1/auth/github/role/"+name, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		fatalf("HTTP %d: %s", resp.StatusCode, body)
	}
	fmt.Printf("GitHub role %q deleted\n", name)
}

func cmdAuthGitHubRoleList(c *client) {
	resp, err := c.do("LIST", "/v1/auth/github/role/", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

// ---- identity ----

func cmdIdentityEntityCreate(c *client, name string, policies []string) {
	body := map[string]any{"name": name, "policies": policies}
	resp, err := c.do("POST", "/v1/identity/entity", body)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityEntityGet(c *client, id string) {
	resp, err := c.do("GET", "/v1/identity/entity/id/"+id, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityEntityGetName(c *client, name string) {
	resp, err := c.do("GET", "/v1/identity/entity/name/"+name, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityEntityUpdate(c *client, id string, policies []string, disabled *bool) {
	body := map[string]any{}
	if len(policies) > 0 {
		body["policies"] = policies
	}
	if disabled != nil {
		body["disabled"] = *disabled
	}
	resp, err := c.do("POST", "/v1/identity/entity/id/"+id, body)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityEntityDelete(c *client, id string) {
	resp, err := c.do("DELETE", "/v1/identity/entity/id/"+id, nil)
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

func cmdIdentityEntityList(c *client) {
	resp, err := c.doRaw("LIST", "/v1/identity/entity/", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityAliasCreate(c *client, entityID, mount, name string) {
	body := map[string]any{"entity_id": entityID, "mount_accessor": mount, "name": name}
	resp, err := c.do("POST", "/v1/identity/entity-alias", body)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityAliasGet(c *client, id string) {
	resp, err := c.do("GET", "/v1/identity/entity-alias/id/"+id, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityAliasDelete(c *client, id string) {
	resp, err := c.do("DELETE", "/v1/identity/entity-alias/id/"+id, nil)
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

func cmdIdentityAliasList(c *client) {
	resp, err := c.doRaw("LIST", "/v1/identity/entity-alias/id/", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityGroupCreate(c *client, name string, policies, memberEntities, memberGroups []string) {
	body := map[string]any{
		"name":              name,
		"policies":          policies,
		"member_entity_ids": memberEntities,
		"member_group_ids":  memberGroups,
	}
	resp, err := c.do("POST", "/v1/identity/group", body)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityGroupGet(c *client, id string) {
	resp, err := c.do("GET", "/v1/identity/group/id/"+id, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityGroupGetName(c *client, name string) {
	resp, err := c.do("GET", "/v1/identity/group/name/"+name, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityGroupUpdate(c *client, id string, policies, memberEntities, memberGroups []string) {
	body := map[string]any{}
	if len(policies) > 0 {
		body["policies"] = policies
	}
	if len(memberEntities) > 0 {
		body["member_entity_ids"] = memberEntities
	}
	if len(memberGroups) > 0 {
		body["member_group_ids"] = memberGroups
	}
	resp, err := c.do("POST", "/v1/identity/group/id/"+id, body)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityGroupDelete(c *client, id string) {
	resp, err := c.do("DELETE", "/v1/identity/group/id/"+id, nil)
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

func cmdIdentityGroupList(c *client) {
	resp, err := c.doRaw("LIST", "/v1/identity/group/", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityGroupAliasCreate(c *client, groupID, mount, name string, meta map[string]string) {
	body := map[string]any{
		"group_id":       groupID,
		"mount_accessor": mount,
		"name":           name,
	}
	if len(meta) > 0 {
		body["metadata"] = meta
	}
	resp, err := c.do("POST", "/v1/identity/group-alias", body)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityGroupAliasGet(c *client, id string) {
	resp, err := c.do("GET", "/v1/identity/group-alias/id/"+id, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityGroupAliasDelete(c *client, id string) {
	resp, err := c.do("DELETE", "/v1/identity/group-alias/id/"+id, nil)
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

func cmdIdentityGroupAliasList(c *client) {
	resp, err := c.doRaw("LIST", "/v1/identity/group-alias/", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityLookupEntity(c *client, id, name, aliasName, aliasMount string) {
	body := map[string]any{}
	switch {
	case id != "":
		body["id"] = id
	case name != "":
		body["name"] = name
	case aliasName != "" && aliasMount != "":
		body["alias_name"] = aliasName
		body["alias_mount_accessor"] = aliasMount
	default:
		fatalf("lookup entity requires --id, --name, or --alias-name + --alias-mount")
	}
	resp, err := c.do("POST", "/v1/identity/lookup/entity", body)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdIdentityLookupGroup(c *client, id, name string) {
	body := map[string]any{}
	switch {
	case id != "":
		body["id"] = id
	case name != "":
		body["name"] = name
	default:
		fatalf("lookup group requires --id or --name")
	}
	resp, err := c.do("POST", "/v1/identity/lookup/group", body)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

// ---- migrate ----

// vaultClient is a thin HTTP client for reading a Vault API endpoint.
type vaultClient struct {
	addr  string
	token string
	http  *http.Client
}

func newVaultClient(addr, token string, insecure bool) *vaultClient {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 — gated by explicit --vault-insecure flag
	}
	return &vaultClient{
		addr:  strings.TrimRight(addr, "/"),
		token: token,
		http:  &http.Client{Transport: tr, Timeout: 30 * time.Second},
	}
}

func (v *vaultClient) do(method, apiPath string) (*http.Response, error) {
	req, err := http.NewRequest(method, v.addr+apiPath, nil) // #nosec G704
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", v.token)
	return v.http.Do(req)
}

func (v *vaultClient) listKeys(apiPath string) ([]string, error) {
	resp, err := v.do("LIST", apiPath)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, nil
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("vault LIST %s: HTTP %d: %s", apiPath, resp.StatusCode, body)
	}
	var result struct {
		Data struct {
			Keys []string `json:"keys"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("vault LIST parse: %v", err)
	}
	return result.Data.Keys, nil
}

func (v *vaultClient) readKV(apiPath string) (map[string]any, error) {
	resp, err := v.do("GET", apiPath)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("vault GET %s: HTTP %d: %s", apiPath, resp.StatusCode, body)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("vault GET parse: %v", err)
	}
	return result, nil
}

// extractKVData pulls the secret fields from a Vault KV response.
// KV v2 nests them at data.data; KV v1 has them directly at data.
func extractKVData(result map[string]any, kvVersion int) map[string]any {
	dataRaw, _ := result["data"]
	data, _ := dataRaw.(map[string]any)
	if kvVersion == 2 {
		innerRaw, _ := data["data"]
		inner, _ := innerRaw.(map[string]any)
		return inner
	}
	return data
}

// migrateKVPaths recursively walks Vault KV and writes each secret to Tuck.
func migrateKVPaths(vault *vaultClient, tuck *client, listPath, dataPath, tuckPrefix string, kvVersion int, dryRun bool) (migrated, skipped int) {
	keys, err := vault.listKeys(listPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: list %s: %v\n", listPath, err)
		return
	}
	for _, key := range keys {
		if strings.HasSuffix(key, "/") {
			sub := strings.TrimSuffix(key, "/")
			m, s := migrateKVPaths(vault, tuck,
				listPath+key, dataPath+key, tuckPrefix+sub+"/",
				kvVersion, dryRun)
			migrated += m
			skipped += s
			continue
		}
		result, err := vault.readKV(dataPath + key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: read %s: %v\n", dataPath+key, err)
			skipped++
			continue
		}
		data := extractKVData(result, kvVersion)
		if len(data) == 0 {
			fmt.Fprintf(os.Stderr, "warn: %s: empty or deleted secret, skipping\n", dataPath+key)
			skipped++
			continue
		}
		tuckPath := tuckPrefix + key
		valueBytes, _ := json.Marshal(data)
		if dryRun {
			fmt.Printf("[dry-run] kv put %s  (%d fields)\n", tuckPath, len(data))
			migrated++
			continue
		}
		tResp, err := tuck.doRaw("PUT", "/v1/secret/"+tuckPath, bytes.NewReader(valueBytes))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: tuck put %s: %v\n", tuckPath, err)
			skipped++
			continue
		}
		tResp.Body.Close()
		if tResp.StatusCode != 204 {
			fmt.Fprintf(os.Stderr, "warn: tuck put %s: HTTP %d\n", tuckPath, tResp.StatusCode)
			skipped++
			continue
		}
		fmt.Printf("migrated: %s\n", tuckPath)
		migrated++
	}
	return
}

func cmdMigrateKV(vault *vaultClient, tuck *client, mount, prefix, destPrefix string, kvVersion int, dryRun bool) {
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if destPrefix != "" && !strings.HasSuffix(destPrefix, "/") {
		destPrefix += "/"
	}
	var listPath, dataPath string
	if kvVersion == 2 {
		listPath = "/v1/" + mount + "/metadata/" + prefix
		dataPath = "/v1/" + mount + "/data/" + prefix
	} else {
		listPath = "/v1/" + mount + "/" + prefix
		dataPath = "/v1/" + mount + "/" + prefix
	}
	migrated, skipped := migrateKVPaths(vault, tuck, listPath, dataPath, destPrefix, kvVersion, dryRun)
	fmt.Printf("\nkv migration: %d migrated, %d skipped\n", migrated, skipped)
}

// hclCapsToBitmask converts Vault capabilities to Tuck bitmask.
// Vault: create, read, update, delete, list, sudo, deny
// Tuck:  read=1, write=2, delete=4, list=8, sudo=16, deny=32
func hclCapsToBitmask(capsStr string) int {
	var caps int
	if strings.Contains(capsStr, "read") {
		caps |= 1
	}
	if strings.Contains(capsStr, "create") || strings.Contains(capsStr, "update") {
		caps |= 2
	}
	if strings.Contains(capsStr, "delete") {
		caps |= 4
	}
	if strings.Contains(capsStr, "list") {
		caps |= 8
	}
	if strings.Contains(capsStr, "sudo") {
		caps |= 16
	}
	if strings.Contains(capsStr, "deny") {
		caps |= 32
	}
	return caps
}

var (
	hclPathRe = regexp.MustCompile(`(?s)path\s+"([^"]+)"\s*\{([^}]+)\}`)
	hclCapRe  = regexp.MustCompile(`capabilities\s*=\s*\[([^\]]+)\]`)
)

// hclToTuckRules parses Vault HCL path blocks into Tuck rule objects.
// KV v2 /data/ and /metadata/ path prefixes are stripped for Tuck compatibility.
func hclToTuckRules(hcl string) []map[string]any {
	rules := make([]map[string]any, 0)
	for _, match := range hclPathRe.FindAllStringSubmatch(hcl, -1) {
		vaultPath := match[1]
		block := match[2]
		tuckPath := strings.ReplaceAll(vaultPath, "/data/", "/")
		tuckPath = strings.ReplaceAll(tuckPath, "/metadata/", "/")
		caps := 0
		if capMatch := hclCapRe.FindStringSubmatch(block); capMatch != nil {
			caps = hclCapsToBitmask(capMatch[1])
		}
		rules = append(rules, map[string]any{
			"path":         tuckPath,
			"capabilities": caps,
		})
	}
	return rules
}

func cmdMigratePolicies(vault *vaultClient, tuck *client, dryRun bool) {
	resp, err := vault.do("GET", "/v1/sys/policy")
	if err != nil {
		fatalf("vault list policies: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fatalf("vault list policies: HTTP %d: %s", resp.StatusCode, body)
	}
	var plist struct {
		Policies []string `json:"policies"`
	}
	if err := json.Unmarshal(body, &plist); err != nil {
		fatalf("vault list policies parse: %v", err)
	}

	migrated, skipped := 0, 0
	for _, name := range plist.Policies {
		if name == "root" || name == "default" {
			continue
		}
		resp2, err := vault.do("GET", "/v1/sys/policy/"+name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: read policy %s: %v\n", name, err)
			skipped++
			continue
		}
		pBody, _ := io.ReadAll(resp2.Body)
		resp2.Body.Close()
		if resp2.StatusCode != 200 {
			fmt.Fprintf(os.Stderr, "warn: policy %s: HTTP %d\n", name, resp2.StatusCode)
			skipped++
			continue
		}
		var pResult struct {
			Rules string `json:"rules"`
		}
		if err := json.Unmarshal(pBody, &pResult); err != nil {
			fmt.Fprintf(os.Stderr, "warn: policy %s: parse error: %v\n", name, err)
			skipped++
			continue
		}
		tuckRules := hclToTuckRules(pResult.Rules)
		if len(tuckRules) == 0 {
			fmt.Fprintf(os.Stderr, "warn: policy %s: no path blocks found in HCL (complex syntax?), skipping — review manually:\n%s\n\n", name, pResult.Rules)
			skipped++
			continue
		}
		tuckPolicy := map[string]any{"rules": tuckRules}
		if dryRun {
			tuckJSON, _ := json.MarshalIndent(tuckPolicy, "", "  ")
			fmt.Printf("[dry-run] policy put %s:\n%s\n\n", name, tuckJSON)
			migrated++
			continue
		}
		tResp, err := tuck.do("PUT", "/v1/policy/"+name, tuckPolicy)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: tuck policy put %s: %v\n", name, err)
			skipped++
			continue
		}
		tResp.Body.Close()
		if tResp.StatusCode != 204 {
			fmt.Fprintf(os.Stderr, "warn: tuck policy put %s: HTTP %d\n", name, tResp.StatusCode)
			skipped++
			continue
		}
		fmt.Printf("migrated policy: %s\n", name)
		migrated++
	}
	fmt.Printf("\npolicy migration: %d migrated, %d skipped\n", migrated, skipped)
}

// ---- main ----

func usage() {
	fmt.Fprintf(os.Stderr, `tuckcli %s — Tuck secrets manager CLI

Usage: tuckcli [flags] <command> [args]

Global flags:
  --addr string     Server address (env TUCK_ADDR, default http://127.0.0.1:8200)
  --token string    Bearer token  (env TUCK_TOKEN)
  --insecure        Skip TLS certificate verification

System:
  status                              Show seal status
  unseal <key>                        Submit a Shamir unseal shard
  seal                                Re-seal the server
  rotate                              Rotate the root key
  version                             Print version

KV secrets:
  kv get <path>                       Get a secret
  kv put <path> <value|-stdin>        Set a secret (use '-' for stdin)
  kv delete <path>                    Delete a secret
  kv list [prefix]                    List secrets

Tokens:
  token create [--name=n] [--policy=p ...] [--ttl=24h] [--max-uses=N]
  token get <id>                      Look up a token
  token renew <id> [ttl]              Renew a token
  token revoke <id>                   Revoke a token
  token list                          List all token IDs
  token lookup-self                   Look up the current token
  token renew-self [ttl]              Renew the current token

Token roles:
  token role create --name=<n> [--policy=p ...] [--ttl=24h] [--max-ttl=168h] [--renewable] [--max-uses=N] [--period=4h]
  token role get <name>
  token role delete <name>
  token role list
  token create-role <role> [display-name]

Policies:
  policy get <name>
  policy put <name> <json|-stdin>
  policy delete <name>
  policy list

Dynamic credentials:
  db creds <role>                     Get DB credentials
  aws creds <role>                    Get AWS credentials
  gcp creds <role>                    Get GCP credentials
  azure creds <role>                  Get Azure credentials

PKI:
  pki issue <role> --cn=<name> [--ttl=720h] [--alt-name=x ...]
  pki revoke <serial>

Transit (encryption-as-a-service):
  transit encrypt <key> <plaintext|-stdin>
  transit decrypt <key> <ciphertext|-stdin>

SSH certificates:
  ssh sign <role> <pubkey-file|-stdin> [--ttl=30m]

TOTP:
  totp code <key>                     Get current OTP code

Auth logins:
  auth approle login --role-id=... --secret-id=...
  auth ldap login --username=... --password=...
  auth jwt login --jwt=... [--role=...]

Identity:
  identity entity create --name=<n> [--policy=p ...]
  identity entity get <id>
  identity entity get-name <name>
  identity entity update <id> [--policy=p ...] [--disable] [--enable]
  identity entity delete <id>
  identity entity list

  identity alias create --entity-id=<id> --mount=<m> --name=<n>
  identity alias get <id>
  identity alias delete <id>
  identity alias list

  identity group create --name=<n> [--policy=p ...] [--member-entity=id ...] [--member-group=id ...]
  identity group get <id>
  identity group get-name <name>
  identity group update <id> [--policy=p ...] [--member-entity=id ...] [--member-group=id ...]
  identity group delete <id>
  identity group list

  identity group-alias create --group-id=<id> --mount=<accessor> --name=<external-name>
  identity group-alias get <id>
  identity group-alias delete <id>
  identity group-alias list

  identity lookup entity [--id=...] [--name=...] [--alias-name=... --alias-mount=...]
  identity lookup group [--id=...] [--name=...]

Namespaces (use --namespace=<ns> or TUCK_NAMESPACE to operate inside a namespace):
  namespace create <name>
  namespace get <name>
  namespace delete <name>
  namespace list

Migration (Vault → Tuck):
  migrate kv      --vault-addr=... --vault-token=... [--vault-insecure] [--mount=secret] [--kv-version=2] [--prefix=] [--dest-prefix=] [--dry-run]
  migrate policies --vault-addr=... --vault-token=... [--vault-insecure] [--dry-run]
  migrate all     --vault-addr=... --vault-token=... [--vault-insecure] [--mount=secret] [--kv-version=2] [--prefix=] [--dest-prefix=] [--dry-run]

  Env vars: VAULT_ADDR, VAULT_TOKEN (mirror of Vault CLI conventions)
  Secrets are stored as JSON-encoded Vault KV data maps.
  Policies are converted from Vault HCL path blocks (best-effort); review the output.
`, Version)
	os.Exit(2)
}

// ---- namespace subcommands ----

func cmdNamespaceCreate(c *client, name string) {
	resp, err := c.do("POST", "/v1/sys/namespaces", map[string]string{"name": name})
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 201))
}

func cmdNamespaceGet(c *client, name string) {
	resp, err := c.do("GET", "/v1/sys/namespaces/"+name, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdNamespaceDelete(c *client, name string) {
	resp, err := c.do("DELETE", "/v1/sys/namespaces/"+name, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		fatalf("HTTP %d: %s", resp.StatusCode, body)
	}
	fmt.Printf("namespace %q deleted\n", name)
}

func cmdNamespaceList(c *client) {
	resp, err := c.do("LIST", "/v1/sys/namespaces/", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

// ---- audit sink subcommands ----

func cmdAuditEnableWebhook(c *client, name, url string, timeoutSec int) {
	resp, err := c.do("PUT", "/v1/sys/audit/webhook/"+name, map[string]any{
		"url":         url,
		"timeout_sec": timeoutSec,
	})
	if err != nil {
		fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		fatalf("HTTP %d: %s", resp.StatusCode, body)
	}
	fmt.Printf("audit webhook %q registered\n", name)
}

func cmdAuditDisable(c *client, name string) {
	resp, err := c.do("DELETE", "/v1/sys/audit/"+name, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		fatalf("HTTP %d: %s", resp.StatusCode, body)
	}
	fmt.Printf("audit sink %q disabled\n", name)
}

func cmdAuditList(c *client) {
	resp, err := c.do("LIST", "/v1/sys/audit/", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

// ---- token role subcommands ----

func cmdTokenRoleCreate(c *client, name string, policies []string, ttl, maxTTL string, maxUses int, renewable bool, period string) {
	body := map[string]any{
		"policies":  policies,
		"renewable": renewable,
		"max_uses":  maxUses,
	}
	if ttl != "" {
		body["ttl"] = ttl
	}
	if maxTTL != "" {
		body["max_ttl"] = maxTTL
	}
	if period != "" {
		body["period"] = period
	}
	resp, err := c.do("PUT", "/v1/auth/token/roles/"+name, body)
	if err != nil {
		fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body2, _ := io.ReadAll(resp.Body)
		fatalf("HTTP %d: %s", resp.StatusCode, body2)
	}
	fmt.Printf("role %q created\n", name)
}

func cmdTokenRoleGet(c *client, name string) {
	resp, err := c.do("GET", "/v1/auth/token/roles/"+name, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdTokenRoleDelete(c *client, name string) {
	resp, err := c.do("DELETE", "/v1/auth/token/roles/"+name, nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		fatalf("HTTP %d: %s", resp.StatusCode, body)
	}
	fmt.Printf("role %q deleted\n", name)
}

func cmdTokenRoleList(c *client) {
	resp, err := c.do("LIST", "/v1/auth/token/roles/", nil)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 200))
}

func cmdTokenCreateFromRole(c *client, role, displayName string) {
	var body any
	if displayName != "" {
		body = map[string]string{"display_name": displayName}
	}
	resp, err := c.do("POST", "/v1/auth/token/roles/"+role+"/create", body)
	if err != nil {
		fatalf("request: %v", err)
	}
	printJSON(mustJSON(resp, 201))
}

func main() {
	addr := flag.String("addr", envOr("TUCK_ADDR", "http://127.0.0.1:8200"), "server address")
	token := flag.String("token", os.Getenv("TUCK_TOKEN"), "bearer token")
	ns := flag.String("namespace", envOr("TUCK_NAMESPACE", ""), "namespace (empty = root)")
	insecure := flag.Bool("insecure", false, "skip TLS verification")
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		usage()
	}

	c := newClient(*addr, *token, *ns, *insecure)

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
			fatalf("token requires a subcommand: create, get, renew, revoke, list, lookup-self, renew-self")
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
		case "lookup-self":
			cmdTokenLookupSelf(c)
		case "renew-self":
			ttl := ""
			if len(args) >= 3 {
				ttl = args[2]
			}
			cmdTokenRenewSelf(c, ttl)
		case "role":
			if len(args) < 3 {
				fatalf("token role requires a subcommand: create, get, delete, list")
			}
			switch args[2] {
			case "create":
				fs := flag.NewFlagSet("token role create", flag.ExitOnError)
				name := fs.String("name", "", "role name (required)")
				ttl := fs.String("ttl", "", "default token TTL e.g. 24h")
				maxTTL := fs.String("max-ttl", "", "maximum token TTL e.g. 168h")
				maxUses := fs.Int("max-uses", 0, "max uses per token (0 = unlimited)")
				renewable := fs.Bool("renewable", false, "tokens from this role are renewable")
				period := fs.String("period", "", "renewal period e.g. 4h")
				var policies multiFlag
				fs.Var(&policies, "policy", "policy (may be repeated)")
				_ = fs.Parse(args[3:])
				if *name == "" {
					fatalf("token role create requires --name")
				}
				cmdTokenRoleCreate(c, *name, []string(policies), *ttl, *maxTTL, *maxUses, *renewable, *period)
			case "get":
				if len(args) < 4 {
					fatalf("token role get requires a name")
				}
				cmdTokenRoleGet(c, args[3])
			case "delete":
				if len(args) < 4 {
					fatalf("token role delete requires a name")
				}
				cmdTokenRoleDelete(c, args[3])
			case "list":
				cmdTokenRoleList(c)
			default:
				fatalf("unknown token role subcommand %q", args[2])
			}

		case "create-role":
			if len(args) < 3 {
				fatalf("token create-role requires a role name")
			}
			displayName := ""
			if len(args) >= 4 {
				displayName = args[3]
			}
			cmdTokenCreateFromRole(c, args[2], displayName)

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

	case "db":
		if len(args) < 3 || args[1] != "creds" {
			fatalf("usage: db creds <role>")
		}
		cmdDynCreds(c, "database", args[2])

	case "aws":
		if len(args) < 3 || args[1] != "creds" {
			fatalf("usage: aws creds <role>")
		}
		cmdDynCreds(c, "aws", args[2])

	case "gcp":
		if len(args) < 3 || args[1] != "creds" {
			fatalf("usage: gcp creds <role>")
		}
		cmdDynCreds(c, "gcp", args[2])

	case "azure":
		if len(args) < 3 || args[1] != "creds" {
			fatalf("usage: azure creds <role>")
		}
		cmdDynCreds(c, "azure", args[2])

	case "pki":
		if len(args) < 2 {
			fatalf("pki requires a subcommand: issue, revoke")
		}
		switch args[1] {
		case "issue":
			fs := flag.NewFlagSet("pki issue", flag.ExitOnError)
			cn := fs.String("cn", "", "common name (required)")
			ttl := fs.String("ttl", "", "certificate TTL e.g. 720h")
			var altNames multiFlag
			fs.Var(&altNames, "alt-name", "SAN (may be repeated)")
			_ = fs.Parse(args[3:])
			if len(args) < 3 {
				fatalf("pki issue requires a role name")
			}
			if *cn == "" {
				fatalf("pki issue requires --cn=<common-name>")
			}
			cmdPKIIssue(c, args[2], *cn, *ttl, []string(altNames))
		case "revoke":
			if len(args) < 3 {
				fatalf("pki revoke requires a serial number")
			}
			cmdPKIRevoke(c, args[2])
		default:
			fatalf("unknown pki subcommand %q", args[1])
		}

	case "transit":
		if len(args) < 2 {
			fatalf("transit requires a subcommand: encrypt, decrypt")
		}
		switch args[1] {
		case "encrypt":
			if len(args) < 4 {
				fatalf("transit encrypt requires a key and plaintext (or '-' for stdin)")
			}
			cmdTransitEncrypt(c, args[2], args[3])
		case "decrypt":
			if len(args) < 4 {
				fatalf("transit decrypt requires a key and ciphertext (or '-' for stdin)")
			}
			cmdTransitDecrypt(c, args[2], args[3])
		default:
			fatalf("unknown transit subcommand %q", args[1])
		}

	case "ssh":
		if len(args) < 2 || args[1] != "sign" {
			fatalf("usage: ssh sign <role> <pubkey-file|-stdin> [--ttl=30m]")
		}
		fs := flag.NewFlagSet("ssh sign", flag.ExitOnError)
		ttl := fs.String("ttl", "", "certificate TTL e.g. 30m")
		_ = fs.Parse(args[4:])
		if len(args) < 4 {
			fatalf("ssh sign requires a role and pubkey file")
		}
		cmdSSHSign(c, args[2], args[3], *ttl)

	case "totp":
		if len(args) < 3 || args[1] != "code" {
			fatalf("usage: totp code <key>")
		}
		cmdTOTPCode(c, args[2])

	case "auth":
		if len(args) < 3 {
			fatalf("auth requires a provider and subcommand: approle login, ldap login, jwt login")
		}
		switch args[1] {
		case "approle":
			if len(args) < 3 || args[2] != "login" {
				fatalf("usage: auth approle login --role-id=... --secret-id=...")
			}
			fs := flag.NewFlagSet("auth approle login", flag.ExitOnError)
			roleID := fs.String("role-id", "", "AppRole role ID (required)")
			secretID := fs.String("secret-id", "", "AppRole secret ID (required)")
			_ = fs.Parse(args[3:])
			if *roleID == "" || *secretID == "" {
				fatalf("auth approle login requires --role-id and --secret-id")
			}
			cmdAuthAppRoleLogin(c, *roleID, *secretID)
		case "ldap":
			if len(args) < 3 || args[2] != "login" {
				fatalf("usage: auth ldap login --username=... --password=...")
			}
			fs := flag.NewFlagSet("auth ldap login", flag.ExitOnError)
			username := fs.String("username", "", "LDAP username (required)")
			password := fs.String("password", "", "LDAP password (required)")
			_ = fs.Parse(args[3:])
			if *username == "" || *password == "" {
				fatalf("auth ldap login requires --username and --password")
			}
			cmdAuthLDAPLogin(c, *username, *password)
		case "jwt":
			if len(args) < 3 || args[2] != "login" {
				fatalf("usage: auth jwt login --jwt=... [--role=...]")
			}
			fs := flag.NewFlagSet("auth jwt login", flag.ExitOnError)
			jwt := fs.String("jwt", "", "JWT token (required)")
			role := fs.String("role", "", "JWT role name")
			_ = fs.Parse(args[3:])
			if *jwt == "" {
				fatalf("auth jwt login requires --jwt")
			}
			cmdAuthJWTLogin(c, *jwt, *role)
		case "github":
			if len(args) < 3 {
				fatalf("auth github requires a subcommand: login, role create/get/delete/list")
			}
			switch args[2] {
			case "login":
				fs := flag.NewFlagSet("auth github login", flag.ExitOnError)
				token := fs.String("token", "", "GitHub Actions OIDC token (required)")
				role := fs.String("role", "", "GitHub auth role name (required)")
				_ = fs.Parse(args[3:])
				if *token == "" || *role == "" {
					fatalf("auth github login requires --token and --role")
				}
				cmdAuthGitHubLogin(c, *token, *role)
			case "role":
				if len(args) < 4 {
					fatalf("auth github role requires: create, get, delete, list")
				}
				switch args[3] {
				case "create":
					fs := flag.NewFlagSet("auth github role create", flag.ExitOnError)
					name       := fs.String("name", "", "role name (required)")
					repo       := fs.String("repository", "", "allowed GitHub repository (e.g. myorg/myrepo)")
					owner      := fs.String("repository-owner", "", "allowed repository owner (e.g. myorg)")
					ref        := fs.String("ref", "", "allowed git ref (e.g. refs/heads/main)")
					env        := fs.String("environment", "", "allowed GitHub environment (e.g. production)")
					workflowRef := fs.String("workflow-ref", "", "allowed workflow_ref claim")
					actor      := fs.String("actor", "", "allowed GitHub actor (username)")
					audience   := fs.String("audience", "", "required aud claim (defaults to GitHub issuer)")
					var policies multiFlag
					fs.Var(&policies, "policy", "Tuck policy name (may be repeated)")
					ttl := fs.String("ttl", "1h", "TTL of issued token (e.g. 1h)")
					_ = fs.Parse(args[4:])
					if *name == "" {
						fatalf("auth github role create requires --name")
					}
					if len(policies) == 0 {
						fatalf("auth github role create requires at least one --policy")
					}
					cmdAuthGitHubRoleCreate(c, *name, *repo, *owner, *ref, *env, *workflowRef, *actor, *audience, []string(policies), *ttl)
				case "get":
					if len(args) < 5 {
						fatalf("auth github role get requires a role name")
					}
					cmdAuthGitHubRoleGet(c, args[4])
				case "delete":
					if len(args) < 5 {
						fatalf("auth github role delete requires a role name")
					}
					cmdAuthGitHubRoleDelete(c, args[4])
				case "list":
					cmdAuthGitHubRoleList(c)
				default:
					fatalf("unknown auth github role subcommand %q", args[3])
				}
			default:
				fatalf("unknown auth github subcommand %q", args[2])
			}
		default:
			fatalf("unknown auth provider %q", args[1])
		}

	case "identity":
		if len(args) < 3 {
			fatalf("identity requires a resource and subcommand: entity, alias, group, lookup")
		}
		switch args[1] {
		case "entity":
			switch args[2] {
			case "create":
				fs := flag.NewFlagSet("identity entity create", flag.ExitOnError)
				name := fs.String("name", "", "entity name (required)")
				var policies multiFlag
				fs.Var(&policies, "policy", "policy name (may be repeated)")
				_ = fs.Parse(args[3:])
				if *name == "" {
					fatalf("identity entity create requires --name")
				}
				cmdIdentityEntityCreate(c, *name, []string(policies))
			case "get":
				if len(args) < 4 {
					fatalf("identity entity get requires an id")
				}
				cmdIdentityEntityGet(c, args[3])
			case "get-name":
				if len(args) < 4 {
					fatalf("identity entity get-name requires a name")
				}
				cmdIdentityEntityGetName(c, args[3])
			case "update":
				if len(args) < 4 {
					fatalf("identity entity update requires an id")
				}
				fs := flag.NewFlagSet("identity entity update", flag.ExitOnError)
				var policies multiFlag
				fs.Var(&policies, "policy", "policy name (may be repeated)")
				disable := fs.Bool("disable", false, "disable the entity")
				enable := fs.Bool("enable", false, "enable the entity")
				_ = fs.Parse(args[4:])
				var disabledPtr *bool
				if *disable {
					t := true
					disabledPtr = &t
				} else if *enable {
					f := false
					disabledPtr = &f
				}
				cmdIdentityEntityUpdate(c, args[3], []string(policies), disabledPtr)
			case "delete":
				if len(args) < 4 {
					fatalf("identity entity delete requires an id")
				}
				cmdIdentityEntityDelete(c, args[3])
			case "list":
				cmdIdentityEntityList(c)
			default:
				fatalf("unknown identity entity subcommand %q", args[2])
			}

		case "alias":
			switch args[2] {
			case "create":
				fs := flag.NewFlagSet("identity alias create", flag.ExitOnError)
				entityID := fs.String("entity-id", "", "entity ID (required)")
				mount := fs.String("mount", "", "mount accessor e.g. auth_approle (required)")
				name := fs.String("name", "", "alias name (required)")
				_ = fs.Parse(args[3:])
				if *entityID == "" || *mount == "" || *name == "" {
					fatalf("identity alias create requires --entity-id, --mount, --name")
				}
				cmdIdentityAliasCreate(c, *entityID, *mount, *name)
			case "get":
				if len(args) < 4 {
					fatalf("identity alias get requires an id")
				}
				cmdIdentityAliasGet(c, args[3])
			case "delete":
				if len(args) < 4 {
					fatalf("identity alias delete requires an id")
				}
				cmdIdentityAliasDelete(c, args[3])
			case "list":
				cmdIdentityAliasList(c)
			default:
				fatalf("unknown identity alias subcommand %q", args[2])
			}

		case "group":
			switch args[2] {
			case "create":
				fs := flag.NewFlagSet("identity group create", flag.ExitOnError)
				name := fs.String("name", "", "group name (required)")
				var policies, memberEntities, memberGroups multiFlag
				fs.Var(&policies, "policy", "policy name (may be repeated)")
				fs.Var(&memberEntities, "member-entity", "member entity ID (may be repeated)")
				fs.Var(&memberGroups, "member-group", "member group ID (may be repeated)")
				_ = fs.Parse(args[3:])
				if *name == "" {
					fatalf("identity group create requires --name")
				}
				cmdIdentityGroupCreate(c, *name, []string(policies), []string(memberEntities), []string(memberGroups))
			case "get":
				if len(args) < 4 {
					fatalf("identity group get requires an id")
				}
				cmdIdentityGroupGet(c, args[3])
			case "get-name":
				if len(args) < 4 {
					fatalf("identity group get-name requires a name")
				}
				cmdIdentityGroupGetName(c, args[3])
			case "update":
				if len(args) < 4 {
					fatalf("identity group update requires an id")
				}
				fs := flag.NewFlagSet("identity group update", flag.ExitOnError)
				var policies, memberEntities, memberGroups multiFlag
				fs.Var(&policies, "policy", "policy name (may be repeated)")
				fs.Var(&memberEntities, "member-entity", "member entity ID (may be repeated)")
				fs.Var(&memberGroups, "member-group", "member group ID (may be repeated)")
				_ = fs.Parse(args[4:])
				cmdIdentityGroupUpdate(c, args[3], []string(policies), []string(memberEntities), []string(memberGroups))
			case "delete":
				if len(args) < 4 {
					fatalf("identity group delete requires an id")
				}
				cmdIdentityGroupDelete(c, args[3])
			case "list":
				cmdIdentityGroupList(c)
			default:
				fatalf("unknown identity group subcommand %q", args[2])
			}

		case "group-alias":
			if len(args) < 3 {
				fatalf("identity group-alias requires a subcommand: create, get, delete, list")
			}
			switch args[2] {
			case "create":
				fs := flag.NewFlagSet("identity group-alias create", flag.ExitOnError)
				groupID := fs.String("group-id", "", "Tuck group ID")
				mount := fs.String("mount", "", "auth mount accessor (e.g. auth_ldap)")
				name := fs.String("name", "", "external group name / DN")
				_ = fs.Parse(args[3:])
				if *groupID == "" || *mount == "" || *name == "" {
					fatalf("identity group-alias create requires --group-id, --mount, --name")
				}
				cmdIdentityGroupAliasCreate(c, *groupID, *mount, *name, nil)
			case "get":
				if len(args) < 4 {
					fatalf("identity group-alias get requires an id")
				}
				cmdIdentityGroupAliasGet(c, args[3])
			case "delete":
				if len(args) < 4 {
					fatalf("identity group-alias delete requires an id")
				}
				cmdIdentityGroupAliasDelete(c, args[3])
			case "list":
				cmdIdentityGroupAliasList(c)
			default:
				fatalf("unknown identity group-alias subcommand %q", args[2])
			}

		case "lookup":
			if len(args) < 4 {
				fatalf("identity lookup requires: entity, group")
			}
			switch args[3] {
			case "entity":
				fs := flag.NewFlagSet("identity lookup entity", flag.ExitOnError)
				id := fs.String("id", "", "entity ID")
				name := fs.String("name", "", "entity name")
				aliasName := fs.String("alias-name", "", "alias name")
				aliasMount := fs.String("alias-mount", "", "alias mount accessor")
				_ = fs.Parse(args[4:])
				cmdIdentityLookupEntity(c, *id, *name, *aliasName, *aliasMount)
			case "group":
				fs := flag.NewFlagSet("identity lookup group", flag.ExitOnError)
				id := fs.String("id", "", "group ID")
				name := fs.String("name", "", "group name")
				_ = fs.Parse(args[4:])
				cmdIdentityLookupGroup(c, *id, *name)
			default:
				fatalf("unknown identity lookup target %q", args[3])
			}

		default:
			fatalf("unknown identity resource %q", args[1])
		}

	case "namespace":
		if len(args) < 2 {
			fatalf("namespace requires a subcommand: create, get, delete, list")
		}
		switch args[1] {
		case "create":
			if len(args) < 3 {
				fatalf("namespace create requires a name")
			}
			cmdNamespaceCreate(c, args[2])
		case "get":
			if len(args) < 3 {
				fatalf("namespace get requires a name")
			}
			cmdNamespaceGet(c, args[2])
		case "delete":
			if len(args) < 3 {
				fatalf("namespace delete requires a name")
			}
			cmdNamespaceDelete(c, args[2])
		case "list":
			cmdNamespaceList(c)
		default:
			fatalf("unknown namespace subcommand %q", args[1])
		}

	case "audit":
		if len(args) < 2 {
			fatalf("audit requires a subcommand: enable-webhook, disable, list")
		}
		switch args[1] {
		case "enable-webhook":
			fs := flag.NewFlagSet("audit enable-webhook", flag.ExitOnError)
			name := fs.String("name", "", "sink name (required)")
			url := fs.String("url", "", "webhook URL (required)")
			timeout := fs.Int("timeout", 5, "HTTP timeout in seconds")
			_ = fs.Parse(args[2:])
			if *name == "" || *url == "" {
				fatalf("audit enable-webhook requires --name and --url")
			}
			cmdAuditEnableWebhook(c, *name, *url, *timeout)
		case "disable":
			if len(args) < 3 {
				fatalf("audit disable requires a name")
			}
			cmdAuditDisable(c, args[2])
		case "list":
			cmdAuditList(c)
		default:
			fatalf("unknown audit subcommand %q", args[1])
		}

	case "migrate":
		if len(args) < 2 {
			fatalf("migrate requires a subcommand: kv, policies, all")
		}
		sub := args[1]
		fs := flag.NewFlagSet("migrate "+sub, flag.ExitOnError)
		vaultAddr := fs.String("vault-addr", envOr("VAULT_ADDR", "http://127.0.0.1:8200"), "Vault server address")
		vaultToken := fs.String("vault-token", os.Getenv("VAULT_TOKEN"), "Vault token")
		vaultInsecure := fs.Bool("vault-insecure", false, "skip Vault TLS verification")
		mount := fs.String("mount", "secret", "Vault KV mount name")
		kvVersion := fs.Int("kv-version", 2, "Vault KV engine version (1 or 2)")
		prefix := fs.String("prefix", "", "Vault KV path prefix to migrate (empty = all)")
		destPrefix := fs.String("dest-prefix", "", "Tuck destination path prefix (empty = root)")
		dryRun := fs.Bool("dry-run", false, "print what would be migrated without writing to Tuck")
		_ = fs.Parse(args[2:])
		if *vaultToken == "" {
			fatalf("migrate requires --vault-token or VAULT_TOKEN env var")
		}
		vault := newVaultClient(*vaultAddr, *vaultToken, *vaultInsecure)
		switch sub {
		case "kv":
			cmdMigrateKV(vault, c, *mount, *prefix, *destPrefix, *kvVersion, *dryRun)
		case "policies":
			cmdMigratePolicies(vault, c, *dryRun)
		case "all":
			cmdMigrateKV(vault, c, *mount, *prefix, *destPrefix, *kvVersion, *dryRun)
			cmdMigratePolicies(vault, c, *dryRun)
		default:
			fatalf("unknown migrate subcommand %q", sub)
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

