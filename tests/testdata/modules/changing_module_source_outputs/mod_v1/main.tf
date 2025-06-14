variable "name" {
    type = string
}

output "greeting" {
    value = "Hello, ${var.name}!"
}