resource "random_integer" "r0" {
  min = 1
  max = 16
  keepers = {
    # Generate a new integer each time a keeper input changes.
    keeper = var.keeper
  }
}

resource "random_integer" "r" {
  min = 1
  max = 16

  lifecycle {
    replace_triggered_by = [
      random_integer.r0.result
    ]
  }
}
