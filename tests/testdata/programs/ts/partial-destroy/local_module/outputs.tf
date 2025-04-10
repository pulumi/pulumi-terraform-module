output "bucket_name" {
  value = try(aws_s3_bucket.bucket[0].id, "")
}

output "bucket_arn" {
  value = try(aws_s3_bucket.bucket[0].arn, "")
}
