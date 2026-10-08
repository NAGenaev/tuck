terraform {
  required_providers {
    tuck = {
      source  = "registry.terraform.io/NAGenaev/tuck"
      version = "~> 1.34"
    }
  }
}

# Provider — all attributes fall back to env vars:
#   TUCK_ADDR      (default: http://127.0.0.1:8200)
#   TUCK_TOKEN
#   TUCK_NAMESPACE
provider "tuck" {
  addr  = "http://127.0.0.1:8200"
  token = var.tuck_token
}

variable "tuck_token" {
  description = "Tuck root or admin token"
  type        = string
  sensitive   = true
}

# ── Namespaces ────────────────────────────────────────────────────────────────

resource "tuck_namespace" "team_a" {
  name = "team-a"
}

# ── Secret Engine Mounts ──────────────────────────────────────────────────────

resource "tuck_mount" "team_a_kv" {
  path        = "team-a-kv/"
  type        = "kv"
  description = "KV secrets for team-a"
}

# ── Policies ──────────────────────────────────────────────────────────────────

resource "tuck_policy" "readonly" {
  name = "readonly"
  rules_json = jsonencode([
    { path = "db/*", capabilities = 1 },       # read
    { path = "services/*", capabilities = 9 }, # read + list
  ])
}

resource "tuck_policy" "app_writer" {
  name = "app-writer"
  rules_json = jsonencode([
    { path = "app/*", capabilities = 7 },     # read + write + create
  ])
}

# ── Token Roles ───────────────────────────────────────────────────────────────

resource "tuck_token_role" "app" {
  name      = "app"
  policies  = [tuck_policy.app_writer.name]
  ttl       = "24h"
  max_ttl   = "168h"
  renewable = true
}

# ── AppRole ───────────────────────────────────────────────────────────────────

resource "tuck_approle_role" "backend" {
  name               = "backend"
  policies           = [tuck_policy.app_writer.name]
  token_ttl          = "1h"
  secret_id_ttl      = "24h"
  secret_id_num_uses = 10
}

output "backend_role_id" {
  value       = tuck_approle_role.backend.role_id
  description = "RoleID for the backend AppRole (combine with a SecretID to authenticate)"
  sensitive   = false
}
