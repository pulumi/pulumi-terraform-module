output "output1" {
  value = terraform_data.example.output
}

output "sensitive_output" {
  value     = sensitive(terraform_data.example.output)
  sensitive = true
}

output "statically_known" {
  value = "static value"
}

output "number_output" {
  value = var.input_number_var
}

output "nested_sensitive_output" {
  value = {
    A = var.input_var
    B = var.another_input_var
    # This will be unknown during plan
    C = terraform_data.example.output
  }
}
