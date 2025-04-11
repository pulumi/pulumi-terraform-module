variable "name_prefix" {
  type        = string
  description = "Prefix to use for the name of the S3 bucket"
}

variable "enabled" {
  type        = bool
  description = "Whether to create the S3 bucket"
  default     = true
}
