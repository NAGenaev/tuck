package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// snapshot save <file> | snapshot restore <file> --yes
func dispatchSnapshot(c *client, args []string) {
	if len(args) < 2 {
		fatalf("usage: snapshot save <file> | snapshot restore <file> --yes")
	}
	switch args[0] {
	case "save":
		resp, err := c.do("GET", "/v1/sys/snapshot", nil)
		if err != nil {
			fatalf("request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			fatalf("HTTP %d: %s", resp.StatusCode, b)
		}
		f, err := os.OpenFile(args[1], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			fatalf("%v (an existing file is never overwritten)", err)
		}
		n, err := io.Copy(f, resp.Body)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			fatalf("write %s: %v", args[1], err)
		}
		fmt.Printf("snapshot saved to %s (%d bytes); the data inside is encrypted\n", args[1], n)
	case "restore":
		yes := false
		for _, a := range args[2:] {
			if a == "--yes" {
				yes = true
			}
		}
		if !yes {
			fatalf("restore replaces ALL data of the cluster with the snapshot; repeat with --yes. Afterwards every node is sealed and must be unsealed with the key shares of the cluster the snapshot came from")
		}
		data, err := os.ReadFile(args[1])
		if err != nil {
			fatalf("%v", err)
		}
		resp, err := c.doRaw("POST", "/v1/sys/cluster/restore?confirm=true", bytes.NewReader(data))
		if err != nil {
			fatalf("request: %v", err)
		}
		printJSON(mustJSON(resp, 200))
	default:
		fatalf("usage: snapshot save <file> | snapshot restore <file> --yes")
	}
}
