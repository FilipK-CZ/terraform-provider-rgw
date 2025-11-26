terraform {
  required_providers {
    rgw = {
      source = "startnext/rgw"
    }
  }
}

provider "rgw" {
  endpoint   = "https://rgw.internal.startnext.org"
  access_key = "35VN9RRGP4MEKHNYVXX2"
  secret_key = "kVMOCgSSJYLS5p4tevRb8UoW0Ak5rLCPi5vcLj1k"
}

resource "rgw_user" "test" {
  username     = "test"
  display_name = "Karl Johann Schubert"
}

# Example user with quotas enabled
resource "rgw_user" "test_with_quota" {
  username     = "test-quota"
  display_name = "Test User With Quota"
  
  # User quota: limit total storage to 1GB
  user_quota {
    enabled     = true
    max_size_kb = 1048576  # 1GB in KB
    max_objects = -1       # unlimited objects
  }
  
  # Bucket quota: limit each bucket to 500MB and 10000 objects
  bucket_quota {
    enabled     = true
    max_size_kb = 512000   # 500MB in KB
    max_objects = 10000
  }
}

resource "rgw_bucket" "test" {
  name = "test"
}

resource "rgw_bucket_policy" "test" {
  bucket = rgw_bucket.test.name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        AWS = [
          "arn:aws:iam::${rgw_user.test.tenant != null ? rgw_user.test.tenant : ""}:user/${rgw_user.test.username}"
        ]
      }
      Action = [
        "s3:ListBucket",
        "s3:DeleteObject",
        "s3:GetObject",
        "s3:PutObject",
        "s3:AbortMultipartUpload",
        "s3:ListAllMyBuckets"
      ]
      Resource = [
        "arn:aws:s3:::${rgw_bucket.test.name}/*",
        "arn:aws:s3:::${rgw_bucket.test.name}",
      ]
    }]
  })
}