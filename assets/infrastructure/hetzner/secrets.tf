resource "random_password" "db" {
  length           = 20
  special          = true
  override_special = "!#$%&*-_=+?"
  min_upper        = 2
  min_lower        = 2
  min_numeric      = 2
  min_special      = 2
}

resource "random_password" "adminui" {
  length           = 20
  special          = true
  override_special = "!#$%&*-_=+?"
  min_upper        = 2
  min_lower        = 2
  min_numeric      = 2
  min_special      = 2
}

resource "tls_private_key" "tls_ca_key" {
  algorithm   = "ECDSA"
  ecdsa_curve = "P256"
}

resource "tls_private_key" "tls_key" {
  algorithm   = "ECDSA"
  ecdsa_curve = "P256"
}
