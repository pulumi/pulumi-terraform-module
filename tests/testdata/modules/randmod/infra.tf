resource "random_integer" "priority" {
  min = 1
  max = var.maxlen
  seed = var.randseed
}
