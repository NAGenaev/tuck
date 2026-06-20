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

resource "tuck_mount" "transit" {
  path        = "transit/"
  type        = "transit"
  description = "Encryption-as-a-service for team-a"
}

resource "tuck_mount" "pki" {
  path        = "pki/"
  type        = "pki"
  description = "Internal PKI CA"
}

# ── Policies ──────────────────────────────────────────────────────────────────

resource "tuck_policy" "readonly" {
  name = "readonly"
  rules_json = jsonencode([
    { path = "db/*",       capabilities = 1  },  # read
    { path = "services/*", capabilities = 9  },  # read + list
  ])
}

resource "tuck_policy" "app_writer" {
  name = "app-writer"
  rules_json = jsonencode([
    { path = "app/*",      capabilities = 7  },  # read + write + create
    { path = "transit/*",  capabilities = 5  },  # read + create
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
  name             = "backend"
  policies         = [tuck_policy.app_writer.name]
  token_ttl        = "1h"
  secret_id_ttl    = "24h"
  secret_id_num_uses = 10
}

output "backend_role_id" {
  value       = tuck_approle_role.backend.role_id
  description = "RoleID for the backend AppRole (combine with a SecretID to authenticate)"
  sensitive   = false
}

# ── KV v1 Secrets ─────────────────────────────────────────────────────────────

resource "tuck_kv_secret" "db_password" {
  path  = "db/password"
  value = "s3cr3t!"
}

resource "tuck_kv_secret" "api_key" {
  path  = "services/api-key"
  value = "my-api-key-value"
}

# ── KV v2 Secrets (versioned) ─────────────────────────────────────────────────

resource "tuck_kv_secret_v2" "app_config" {
  path  = "app/config"
  value = jsonencode({
    db_host = "postgres.internal"
    db_port = 5432
  })
}

output "app_config_version" {
  value = tuck_kv_secret_v2.app_config.version
}

# ── Data Sources ──────────────────────────────────────────────────────────────

data "tuck_kv_secret" "db_password" {
  path = tuck_kv_secret.db_password.path
}

data "tuck_kv_secret_v2" "app_config" {
  path = tuck_kv_secret_v2.app_config.path
  # version omitted → reads latest
}

output "db_password_value" {
  value     = data.tuck_kv_secret.db_password.value
  sensitive = true
}

output "app_config_value" {
  value     = data.tuck_kv_secret_v2.app_config.value
  sensitive = true
}
