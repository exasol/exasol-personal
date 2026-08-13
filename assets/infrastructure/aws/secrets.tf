resource "random_password" "db" {
  length      = 8
  special     = false
  min_upper   = 1
  min_lower   = 1
  min_numeric = 1
}

resource "random_password" "adminui" {
  length      = 8
  special     = false
  min_upper   = 1
  min_lower   = 1
  min_numeric = 1
}

# AI Lab secure-configuration-storage (SCS) master password.
# Only created when AI Lab is requested, so a plain deployment carries no AI Lab
# secrets in state or cloud-init.
resource "random_password" "ai_lab_scs" {
  count       = var.with_ai_lab ? 1 : 0
  length      = 16
  special     = false
  min_upper   = 1
  min_lower   = 1
  min_numeric = 1
}

# AI Lab Jupyter access password.
resource "random_password" "ai_lab_jupyter" {
  count       = var.with_ai_lab ? 1 : 0
  length      = 16
  special     = false
  min_upper   = 1
  min_lower   = 1
  min_numeric = 1
}

# AI Lab BucketFS bucket read/write password.
resource "random_password" "ai_lab_bfs" {
  count       = var.with_ai_lab ? 1 : 0
  length      = 16
  special     = false
  min_upper   = 1
  min_lower   = 1
  min_numeric = 1
}

resource "tls_private_key" "tls_ca_key" {
  algorithm   = "ECDSA"
  ecdsa_curve = "P256"
}

resource "tls_private_key" "tls_key" {
  algorithm   = "ECDSA"
  ecdsa_curve = "P256"
}
