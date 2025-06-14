variable "name" {
    type = string
}

output "greeting" {
    value = "Goodbye, ${var.name}!"
}