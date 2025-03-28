resource "aws_s3_bucket" "bucket" {
  bucket_prefix = var.name_prefix
  force_destroy = false
}

resource "aws_s3_bucket" "another_bucket" {
  bucket_prefix = "another-${var.name_prefix}"
}
