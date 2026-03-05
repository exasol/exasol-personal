provider "aws" {
  default_tags {
    tags = {
      # Conventional resource tags
      ManagedBy  = "opentofu"
      Project    = "exasol-personal"
      Deployment = local.deployment_id
      # Must be known at plan time (provider default_tags cannot depend on resources).
      CreatedAt  = var.deployment_created_at
      # Note: Owner tag is applied to individual resources in main.tf 
      # using data.aws_caller_identity.current to avoid circular dependency
    }
  }
}

