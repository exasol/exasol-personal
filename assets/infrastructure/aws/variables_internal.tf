# Variables required for terraform/tofu integration of an infrastructure preset with the Exasol Personal launcher.
# Ensure all deployment artifacts which required for Exasol Personal launcher integration are written to this directory
# These are:
#  deployment-<id>.json  -- meta information about the deployed infrastructure
#  scecrets-<id>.json -- secrets to access the database services
#  <id>.pem  -- a ssh key to access all deployed hosts
#
# A change here might break the interaction with the Exasol Personal launcher CLI
# The Exasol Personal launcher expected some files to be present at the root of the deployment directory

variable "infrastructure_artifact_dir" {
  description = "Directory where deployment artifacts for the Exasol Personal launcher (JSON, PEM) are written"
  type        = string
  default     = ".."
}

variable "installation_preset_dir" {
  description = "Directory where installation preset can be found"
  type        = string  
  default     = ".."
}