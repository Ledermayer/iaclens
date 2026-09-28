terraform {
  required_version = ">= 1.9"
  required_providers {
    aws = {
      source = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}
variable "cidr" {
  type        = string
  description = "IPv4 CIDR supplied by the caller for this network."
}
resource "aws_vpc" "this" {
  cidr_block = var.cidr
}
output "vpc_id" {
  description = "Network ID for the calling configuration."
  value       = aws_vpc.this.id
}
