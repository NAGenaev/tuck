package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func expectOK(resp *http.Response) {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fatalf("HTTP %d: %s", resp.StatusCode, body)
	}
	fmt.Println("OK")
}

func readJSONArg(arg string) string {
	if arg != "-" {
		return arg
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatalf("read stdin: %v", err)
	}
	// Windows PowerShell pipes may prepend a UTF-8 BOM, which is not valid JSON.
	return string(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
}

func dispatchLDAPAuth(c *client, args []string) {
	if len(args) < 1 {
		fatalf("usage: auth ldap login|config|role")
	}
	switch args[0] {
	case "login":
		fs := flag.NewFlagSet("auth ldap login", flag.ExitOnError)
		username := fs.String("username", "", "LDAP username (required)")
		password := fs.String("password", "", "LDAP password (required)")
		ns := fs.String("namespace", "", "namespace to log in to when the user has roles in several (empty = root)")
		_ = fs.Parse(args[1:])
		if *username == "" || *password == "" {
			fatalf("auth ldap login requires --username and --password")
		}
		var nsPtr *string
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "namespace" {
				nsPtr = ns
			}
		})
		ldapLogin(c, *username, *password, nsPtr)
	case "config":
		if len(args) < 2 {
			fatalf("usage: auth ldap config get | put <json|->")
		}
		switch args[1] {
		case "get":
			resp, err := c.do("GET", "/v1/auth/ldap/config", nil)
			if err != nil {
				fatalf("request: %v", err)
			}
			printJSON(mustJSON(resp, 200))
		case "put":
			if len(args) < 3 {
				fatalf("auth ldap config put requires a JSON document (or - for stdin)")
			}
			var cfg any
			if err := json.Unmarshal([]byte(readJSONArg(args[2])), &cfg); err != nil {
				fatalf("invalid JSON: %v", err)
			}
			resp, err := c.do("PUT", "/v1/auth/ldap/config", cfg)
			if err != nil {
				fatalf("request: %v", err)
			}
			expectOK(resp)
		default:
			fatalf("unknown ldap config subcommand %q", args[1])
		}
	case "role":
		dispatchLDAPRole(c, args[1:])
	default:
		fatalf("unknown ldap subcommand %q", args[0])
	}
}

func ldapLogin(c *client, username, password string, namespace *string) {
	body := map[string]any{"username": username, "password": password}
	if namespace != nil {
		body["namespace"] = *namespace
	}
	resp, err := c.do("POST", "/v1/auth/ldap/login", body)
	if err != nil {
		fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusConflict {
		fatalf("this user has roles in several namespaces; repeat with --namespace=<name> (empty = root): %s", data)
	}
	if resp.StatusCode != 200 {
		fatalf("HTTP %d: %s", resp.StatusCode, data)
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		fatalf("parse response: %v", err)
	}
	printJSON(out)
}

func dispatchLDAPRole(c *client, args []string) {
	if len(args) < 1 {
		fatalf("usage: auth ldap role put|get|delete <name> | list")
	}
	if args[0] == "list" {
		resp, err := c.doRaw("LIST", "/v1/auth/ldap/role/", nil)
		if err != nil {
			fatalf("request: %v", err)
		}
		printJSON(mustJSON(resp, 200))
		return
	}
	if len(args) < 2 {
		fatalf("auth ldap role %s requires a name", args[0])
	}
	path := "/v1/auth/ldap/role/" + args[1]
	switch args[0] {
	case "put":
		fs := flag.NewFlagSet("auth ldap role put", flag.ExitOnError)
		groups := fs.String("groups", "", "comma-separated LDAP groups (CN or DN)")
		users := fs.String("users", "", "comma-separated usernames")
		policies := fs.String("policies", "", "comma-separated Tuck policies")
		ttl := fs.String("ttl", "", "token TTL, e.g. 8h")
		tuckNS := fs.String("tuck-namespace", "", "bind issued tokens to this Tuck namespace (empty = root)")
		_ = fs.Parse(args[2:])
		resp, err := c.do("PUT", path, map[string]any{
			"groups": splitCSV(*groups), "users": splitCSV(*users), "policies": splitCSV(*policies),
			"ttl": *ttl, "tuck_namespace": *tuckNS,
		})
		if err != nil {
			fatalf("request: %v", err)
		}
		expectOK(resp)
	case "get":
		resp, err := c.do("GET", path, nil)
		if err != nil {
			fatalf("request: %v", err)
		}
		printJSON(mustJSON(resp, 200))
	case "delete":
		resp, err := c.do("DELETE", path, nil)
		if err != nil {
			fatalf("request: %v", err)
		}
		expectOK(resp)
	default:
		fatalf("unknown ldap role subcommand %q", args[0])
	}
}

func k8sRolePath(cluster, ns, sa string) string {
	if cluster != "" {
		return "/v1/auth/kubernetes/clusters/" + cluster + "/role/" + ns + "/" + sa
	}
	return "/v1/auth/kubernetes/role/" + ns + "/" + sa
}

func dispatchK8sAuth(c *client, args []string) {
	if len(args) < 1 {
		fatalf("usage: auth kubernetes role|clusters|login")
	}
	switch args[0] {
	case "clusters":
		resp, err := c.do("GET", "/v1/auth/kubernetes/clusters", nil)
		if err != nil {
			fatalf("request: %v", err)
		}
		printJSON(mustJSON(resp, 200))
	case "login":
		fs := flag.NewFlagSet("auth kubernetes login", flag.ExitOnError)
		jwt := fs.String("jwt", "", "ServiceAccount token (required)")
		cluster := fs.String("cluster", "", "trusted cluster name (default cluster if empty)")
		_ = fs.Parse(args[1:])
		if *jwt == "" {
			fatalf("auth kubernetes login requires --jwt")
		}
		body := map[string]string{"token": *jwt}
		if *cluster != "" {
			body["cluster"] = *cluster
		}
		resp, err := c.do("POST", "/v1/auth/kubernetes/login", body)
		if err != nil {
			fatalf("request: %v", err)
		}
		printJSON(mustJSON(resp, 200))
	case "role":
		if len(args) < 4 {
			fatalf("usage: auth kubernetes role put|get|delete <k8s-namespace> <service-account> [flags]")
		}
		fs := flag.NewFlagSet("auth kubernetes role", flag.ExitOnError)
		cluster := fs.String("cluster", "", "trusted cluster name (default cluster if empty)")
		policies := fs.String("policies", "", "comma-separated Tuck policies (put)")
		ttl := fs.String("ttl", "", "token TTL, e.g. 24h (put)")
		tuckNS := fs.String("tuck-namespace", "", "bind issued tokens to this Tuck namespace (put; empty = root)")
		_ = fs.Parse(args[4:])
		path := k8sRolePath(*cluster, args[2], args[3])
		switch args[1] {
		case "put":
			resp, err := c.do("PUT", path, map[string]any{
				"policies": splitCSV(*policies), "ttl": *ttl, "tuck_namespace": *tuckNS,
			})
			if err != nil {
				fatalf("request: %v", err)
			}
			expectOK(resp)
		case "get":
			resp, err := c.do("GET", path, nil)
			if err != nil {
				fatalf("request: %v", err)
			}
			printJSON(mustJSON(resp, 200))
		case "delete":
			resp, err := c.do("DELETE", path, nil)
			if err != nil {
				fatalf("request: %v", err)
			}
			expectOK(resp)
		default:
			fatalf("unknown kubernetes role subcommand %q", args[1])
		}
	default:
		fatalf("unknown kubernetes subcommand %q", args[0])
	}
}
