terraform {
  remote "backend" {
    hostname     = "foobar.terraform.io"
    organization = "hashicorp"

    workspaces {
      name = "dev-service-02"
    }
  }
}
