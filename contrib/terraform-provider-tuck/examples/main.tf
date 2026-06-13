terraform {
  required_providers {
    tuck = {
      source  = "registry.terraform.io/NAGenaev/tuck"
      version = "~> 1.29"
    }
  }
}

# Provider configuration — all attributes fall back to env vars:
#   TUCK_ADDR      (default: http://127.0.0.1:8200)
#   TUCK_TOKEN
#   TUCK_NAMESPACE
provider "tuck" {
  addr  = "http://127.0.0.1:8200"
  token = var.tuck_token
}

variable "tuck_token" {
  description = "Tuck root or auth token"
  type        = string
  sensitive   = true
}

# ── Resources ────────────────────────────────────────────────────────────────

resource "tuck_kv_secret" "db_password" {
  path  = "db/password"
  value = "s3cr3t!"
}

resource "tuck_kv_secret" "api_key" {
  path  = "services/api-key"
  value = "my-api-key-value"
}

resource "tuck_policy" "readonly" {
  name = "readonly"
  rules_json = jsonencode([
    {
      path         = "db/*"
      capabilities = 1  # read
    },
    {
      path         = "services/*"
      capabilities = 9  # read + list
    }
  ])
}

# ── Data sources ─────────────────────────────────────────────────────────────

# Read back a secret written elsewhere.
data "tuck_kv_secret" "db_password" {
  path = "db/password"
}

output "db_password_value" {
  value     = data.tuck_kv_secret.db_password.value
  sensitive = true
}
