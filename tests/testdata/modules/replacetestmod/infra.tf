resource "random_integer" "r" {
  min = 1
  max = 16
  keepers = {
    # Generate a new integer each time a keeper input changes.
    keeper = var.keeper
  }
}
