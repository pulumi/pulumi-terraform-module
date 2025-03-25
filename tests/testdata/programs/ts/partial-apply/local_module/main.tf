resource "aws_iam_role" "this" {
  name_prefix = var.name_prefix
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Sid    = ""
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      },
    ]
  })
}

data "aws_caller_identity" "current" {}

locals {
  fail_arn    = "arn:aws:iam::aws:policy/ReadOnlyAccessFAIL"
  success_arn = "arn:aws:iam::aws:policy/ReadOnlyAccess"
}

resource "aws_iam_role_policy_attachment" "this" {
  role       = aws_iam_role.this.name
  policy_arn = var.should_fail ? local.fail_arn : local.success_arn
}
