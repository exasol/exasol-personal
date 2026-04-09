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
