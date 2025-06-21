variable "name_prefix" {
  type        = string
  description = "Prefix to use for the name of the IAM role"
}

variable "description" {
  type        = string
  description = "description of the IAM role"
}

variable "should_fail" {
  type        = bool
  description = "Whether the module should fail"
}
