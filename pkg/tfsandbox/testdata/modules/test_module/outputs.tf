output "output1" {
  value = terraform_data.example.output
}

output "sensitive_output" {
  value     = terraform_data.example.output
  sensitive = true
}

output "statically_known" {
  value = "static value"
}