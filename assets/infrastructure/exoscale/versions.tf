terraform {
  required_version = ">= 1.10.0"
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "~> 2.6"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.1"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
    cloudinit = {
      source  = "hashicorp/cloudinit"
      version = "~> 2.3"
    }
    exoscale = {
      source  = "exoscale/exoscale"
      version = "~> 0.68"
    }
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}