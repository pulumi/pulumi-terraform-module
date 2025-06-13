variable "required_string" {
    description = "required string"
    type = string
}

variable "optional_string_with_default" {
    description = "optional string with default"
    type    = string
    default = "default_value"
}

variable "optional_string_without_default" {
    description = "optional string without default"
    type = string
    default = null
}

variable "required_string_using_nullable_false" {
    type = string
    nullable = false
}

variable "optional_string_using_nullable_true" {
    type = string
    nullable = true
}

variable "required_boolean" {
    type = bool
}

variable "optional_boolean_with_default" {
    type    = bool
    default = true
}

variable "required_number" {
    type = number
}

variable "optional_number_with_default" {
    type    = number
    default = 42
}

variable "required_list_of_strings" {
    type = list(string)
}

variable "optional_list_of_strings_with_default" {
    description = "optional list of strings with default"
    type    = list(string)
    default = []
}

variable "optional_list_of_strings_without_default" {
    description = "optional list of strings without default"
    type = list(string)
    default = null
}

variable "required_map_of_strings" {
    type = map(string)
}

variable "optional_map_of_strings_with_default" {
    description = "optional map of strings with default"
    type    = map(string)
    default = {}
}