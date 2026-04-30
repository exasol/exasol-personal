resource "tls_self_signed_cert" "tls_ca_cert" {
  private_key_pem = tls_private_key.tls_ca_key.private_key_pem

  subject {
    common_name = "exacluster.local"
  }

  validity_period_hours = 24 * 365 * 10

  is_ca_certificate  = true
  set_subject_key_id = true

  allowed_uses = [
    "cert_signing",
    "key_encipherment",
  ]
}

resource "tls_cert_request" "tls_request" {
  private_key_pem = tls_private_key.tls_key.private_key_pem

  subject {
    common_name = "exacluster.local"
  }

  uris = [
    "*.exacluster.local",
    "exacluster.local",
  ]
}

resource "tls_locally_signed_cert" "tls_cert" {
  cert_request_pem   = tls_cert_request.tls_request.cert_request_pem
  ca_private_key_pem = tls_private_key.tls_ca_key.private_key_pem
  ca_cert_pem        = tls_self_signed_cert.tls_ca_cert.cert_pem

  validity_period_hours = 24 * 365 * 10

  allowed_uses = [
    "key_encipherment",
    "digital_signature",
    "server_auth",
  ]
}

