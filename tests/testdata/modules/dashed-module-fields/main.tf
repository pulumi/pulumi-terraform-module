terraform {
  required_providers {
    google-beta = {
      source  = "hashicorp/google-beta"
      version = ">= 6.19, < 7"
    }
  }
}

variable "dashed-input" {
    type = string
    default = "default-value"
}

output "dashed-output" {
    value = var.dashed-input
}