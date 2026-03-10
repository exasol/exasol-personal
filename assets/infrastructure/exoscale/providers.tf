provider "exoscale" {}

# AWS provider aliased for Exoscale SOS (S3-compatible object storage).
# Credentials are passed via EXOSCALE_API_KEY / EXOSCALE_API_SECRET env vars
# mapped to AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY.
provider "aws" {
  alias  = "sos"
  region = var.zone

  endpoints {
    s3 = "https://sos-${var.zone}.exo.io"
  }

  # Disable AWS-specific features that don't apply to Exoscale SOS
  skip_credentials_validation = true
  skip_region_validation      = true
  skip_requesting_account_id  = true
  skip_metadata_api_check     = true
}