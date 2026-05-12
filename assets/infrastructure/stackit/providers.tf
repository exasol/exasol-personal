provider "stackit" {
  default_region        = var.region
  enable_beta_resources = true
}

provider "minio" {
  alias = "bootstrap"

  minio_server   = "object.storage.${var.region}.onstackit.cloud"
  minio_user     = stackit_objectstorage_credential.bootstrap_assets.access_key
  minio_password = stackit_objectstorage_credential.bootstrap_assets.secret_access_key
  minio_region   = var.region
  minio_ssl      = true

  s3_compat_mode = true
}
