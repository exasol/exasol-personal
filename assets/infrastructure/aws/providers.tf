# Capture creation timestamp once and keep it stable across all applies
resource "time_static" "deployment_created" {}

provider "aws" {
  default_tags {
    tags = {
      # Conventional resource tags
      ManagedBy  = "opentofu"
      Project    = "exasol-personal"
      Deployment = local.deployment_id
      CreatedAt  = time_static.deployment_created.rfc3339
      # Note: Owner tag is applied to individual resources in main.tf 
      # using data.aws_caller_identity.current to avoid circular dependency
    }
  }
}

