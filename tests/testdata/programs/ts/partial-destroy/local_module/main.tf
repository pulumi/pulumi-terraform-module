resource "aws_s3_bucket" "bucket" {
  count         = var.enabled ? 1 : 0
  bucket_prefix = var.name_prefix
  force_destroy = false
}

resource "aws_s3_bucket" "another_bucket" {
  count         = var.enabled ? 1 : 0
  bucket_prefix = "another-${var.name_prefix}"
}
