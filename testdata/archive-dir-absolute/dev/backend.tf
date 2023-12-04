# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

terraform {
  remote "backend" {
    hostname     = "foobar.terraform.io"
    organization = "hashicorp"

    workspaces {
      name = "dev-service-02"
    }
  }
}
