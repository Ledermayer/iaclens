terraform {
  backend "local" {}
  required_version = ">= 1.9"
  required_providers {
    aws = {
      source = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}
provider "aws" {
  region = "eu-west-1"
}
module "storage" {
  source = "../.."
  name = "iaclens-example-dev"
  tags = { environment = "dev" }
}
