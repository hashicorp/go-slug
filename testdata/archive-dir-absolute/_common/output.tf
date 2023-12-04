# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

locals {
    files = fileset("${path.module}/extra-files", "*.sh")
}

output "scripts" {
    value = local.files
}
