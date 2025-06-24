# example showing an input called name is automatically set
variable "name" {
    type    = string
    default = ""
}

output "name" {
    value = var.name
}