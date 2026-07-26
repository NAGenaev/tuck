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
    { path = "db/*", capabilities = 1 },       # read
    { path = "services/*", capabilities = 9 }, # read + list
  ])
}

resource "tuck_policy" "app_writer" {
  name = "app-writer"
  rules_json = jsonencode([
    { path = "app/*", capabilities = 7 },     # read + write + create
    { path = "transit/*", capabilities = 5 }, # read + create
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

# ── PKI ───────────────────────────────────────────────────────────────────────

resource "tuck_pki_role" "web" {
  name             = "web-server"
  allowed_domains  = ["example.internal"]
  allow_subdomains = true
  server_flag      = true
  default_ttl      = "720h"
  max_ttl          = "8760h"
}

# ── Transit ───────────────────────────────────────────────────────────────────

resource "tuck_transit_key" "app" {
  name = "app-encryption-key"
  type = "aes256-gcm96"
  # Tuck refuses to delete a Transit key until this is explicitly true —
  # flip it (and apply) before removing the resource, or `terraform destroy`
  # fails on purpose instead of silently discarding key material.
  deletion_allowed = false
}

# ── SSH CA ────────────────────────────────────────────────────────────────────

resource "tuck_ssh_role" "ops" {
  name          = "ops"
  allowed_users = ["ubuntu", "deploy"]
  cert_type     = "user"
  default_ttl   = "1h"
  max_ttl       = "24h"
}

# ── Dynamic Database Secrets ──────────────────────────────────────────────────

resource "tuck_database_connection" "app_postgres" {
  name           = "app-postgres"
  plugin_name    = "postgresql"
  connection_url = "postgres://tuck_admin:{{password}}@postgres.internal:5432/postgres?sslmode=disable"
  database       = "app"
}

resource "tuck_database_role" "app_readonly" {
  name    = "app-readonly"
  db_name = tuck_database_connection.app_postgres.name
  creation_statements = join(" ", [
    "CREATE USER \"{{username}}\" WITH PASSWORD '{{password}}' VALID UNTIL '{{expiry}}';",
    "GRANT CONNECT ON DATABASE \"{{database}}\" TO \"{{username}}\";",
    "GRANT SELECT ON ALL TABLES IN SCHEMA public TO \"{{username}}\";",
  ])
  default_ttl = "1h"
  max_ttl     = "24h"
}

# ── Dynamic Cloud Secrets (AWS / GCP / Azure) ─────────────────────────────────
# Each cloud's config is a singleton (one per Tuck server/namespace) — the
# resource address itself gives it identity, there's no separate name field.

resource "tuck_aws_config" "this" {
  region            = "us-east-1"
  access_key_id     = var.aws_access_key_id
  secret_access_key = var.aws_secret_access_key
}

resource "tuck_aws_role" "readonly" {
  name            = "readonly"
  credential_type = "iam_user"
  policy_arns     = ["arn:aws:iam::aws:policy/ReadOnlyAccess"]
  default_ttl     = "1h"
  max_ttl         = "24h"
}

resource "tuck_gcp_config" "this" {
  credentials_json = var.gcp_credentials_json
}

resource "tuck_gcp_role" "readonly" {
  name                  = "readonly"
  credential_type       = "access_token"
  service_account_email = "readonly@my-project.iam.gserviceaccount.com"
  scopes                = ["https://www.googleapis.com/auth/cloud-platform.read-only"]
  default_ttl           = "1h"
  max_ttl               = "24h"
}

resource "tuck_azure_config" "this" {
  tenant_id     = var.azure_tenant_id
  client_id     = var.azure_client_id
  client_secret = var.azure_client_secret
}

resource "tuck_azure_role" "readonly" {
  name                  = "readonly"
  application_object_id = var.azure_readonly_app_object_id
  application_id        = var.azure_readonly_app_id
  default_ttl           = "1h"
  max_ttl               = "24h"
}

variable "aws_access_key_id" {
  type      = string
  sensitive = true
}
variable "aws_secret_access_key" {
  type      = string
  sensitive = true
}
variable "gcp_credentials_json" {
  type      = string
  sensitive = true
}
variable "azure_tenant_id" {
  type = string
}
variable "azure_client_id" {
  type = string
}
variable "azure_client_secret" {
  type      = string
  sensitive = true
}
variable "azure_readonly_app_object_id" {
  type = string
}
variable "azure_readonly_app_id" {
  type = string
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
  path = "app/config"
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
