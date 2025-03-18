variable "input_var" {
  type    = string
  default = "default"
}

variable "input_number_var" {
  type = number
  // set the default as a big.Float value since
  // we can't pass in a big value
  default = 4222222222222222222
}

variable "another_input_var" {
  type    = string
  default = "default"
}
