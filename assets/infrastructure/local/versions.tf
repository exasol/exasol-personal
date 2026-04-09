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
  }
}
