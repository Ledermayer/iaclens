terraform {
  required_version = ">= 1.9"
  required_providers {
    aws = {
      source = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}
# Deliberately omit nullable on inputs so company-nullability reports a failure.
variable "name" {
  type        = string
  description = "Unique name supplied by the caller for the storage bucket."
}
# The custom nullability rule applies to this input too.
variable "tags" {
  type        = map(string)
  default     = {}
  description = "Tags supplied by the calling configuration."
}
resource "aws_s3_bucket" "this" {
  bucket = var.name
  tags   = var.tags
}
output "bucket_id" {
  description = "Bucket ID for use by the calling configuration."
  value       = aws_s3_bucket.this.id
}
