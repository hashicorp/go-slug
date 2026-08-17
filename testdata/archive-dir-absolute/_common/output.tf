# Copyright IBM Corp. 2018, 2026
# SPDX-License-Identifier: MPL-2.0

locals {
    files = fileset("${path.module}/extra-files", "*.sh")
}

output "scripts" {
    value = local.files
}
