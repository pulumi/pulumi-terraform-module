resource "local_file" "hello" {
  content  = "Hello, World!"
  filename = "${var.pwd}/hello.txt"
}
