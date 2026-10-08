package main

import (
	"encoding/json"
	"flag"
)

// team onboard <json|-> [--dry-run]   |   team sync [name] [--dry-run]
func dispatchTeam(c *client, args []string) {
	if len(args) < 1 {
		fatalf("usage: team onboard <json|-> [--dry-run] | team sync [name] [--dry-run]")
	}
	switch args[0] {
	case "onboard":
		if len(args) < 2 {
			fatalf("team onboard requires a JSON document (or - for stdin)")
		}
		var spec map[string]any
		if err := json.Unmarshal([]byte(readJSONArg(args[1])), &spec); err != nil {
			fatalf("invalid JSON: %v", err)
		}
		fs := flag.NewFlagSet("team onboard", flag.ExitOnError)
		dry := fs.Bool("dry-run", false, "show what would be created, change nothing")
		_ = fs.Parse(args[2:])
		if *dry {
			spec["dry_run"] = true
		}
		resp, err := c.do("POST", "/v1/sys/teams", spec)
		if err != nil {
			fatalf("request: %v", err)
		}
		want := 201
		if *dry {
			want = 200
		}
		printJSON(mustJSON(resp, want))
	case "sync":
		fs := flag.NewFlagSet("team sync", flag.ExitOnError)
		dry := fs.Bool("dry-run", false, "show which policies would change, change nothing")
		rest := args[1:]
		name := ""
		if len(rest) > 0 && len(rest[0]) > 0 && rest[0][0] != '-' {
			name, rest = rest[0], rest[1:]
		}
		_ = fs.Parse(rest)
		resp, err := c.do("POST", "/v1/sys/teams/sync", map[string]any{"name": name, "dry_run": *dry})
		if err != nil {
			fatalf("request: %v", err)
		}
		printJSON(mustJSON(resp, 200))
	default:
		fatalf("usage: team onboard <json|-> [--dry-run] | team sync [name] [--dry-run]")
	}
}
