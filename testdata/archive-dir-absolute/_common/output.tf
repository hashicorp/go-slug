locals {
    files = fileset("${path.module}/extra-files", "*.sh")
}

output "scripts" {
    value = local.files
}
