resource "aws_s3_bucket" "tf-test-bucket" {
  bucket = "${var.prefix}-tf-test-bucket"
  tags = {
    TestTag = var.tagvalue
  }
}
