# Terraform Provider for Ceph RadosGW

A Terraform provider for managing resources in Ceph RadosGW (RADOS Gateway), enabling infrastructure-as-code for S3-compatible object storage.

## Features

This provider allows you to manage the following Ceph RadosGW resources:

- **Users** - Create and manage S3/Swift users with quotas and capabilities
- **Buckets** - Create and manage storage buckets
- **Bucket Policies** - Define and enforce bucket-level access policies

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.18 (for development)
- Ceph RadosGW instance with admin API access

## Installation

### Terraform Registry (Recommended)

Add the provider to your Terraform configuration:

```hcl
terraform {
  required_providers {
    rgw = {
      source  = "your-namespace/rgw"
      version = "~> 1.0"
    }
  }
}
```

### Local Installation

For local development or testing:

```bash
export PROVIDER_VERSION=1.0.0
export BINARY_ARCH=darwin_arm64  # Change based on your architecture

CGO_ENABLED=0 go build \
  -o ~/.terraform.d/plugins/terraform.local/local/rgw/${PROVIDER_VERSION}/${BINARY_ARCH}/terraform-provider-rgw_v${PROVIDER_VERSION} \
  -ldflags="-X 'main.Version=${PROVIDER_VERSION}'" \
  main.go
```

Common architectures:
- `darwin_arm64` - macOS Apple Silicon
- `darwin_amd64` - macOS Intel
- `linux_amd64` - Linux x86_64
- `linux_arm64` - Linux ARM64

Add the following to your `~/.terraformrc`:

```hcl
provider_installation {
  filesystem_mirror {
    path = "/Users/YOUR_USERNAME/.terraform.d/plugins"
  }
  direct {
    exclude = ["terraform.local/*/*"]
  }
}
```

## Usage

### Provider Configuration

```hcl
provider "rgw" {
  endpoint   = "https://rgw.example.com"
  access_key = var.rgw_access_key
  secret_key = var.rgw_secret_key
}
```

#### Configuration Options

| Argument | Required | Description | Environment Variable |
|----------|----------|-------------|---------------------|
| `endpoint` | Yes | RGW Admin API endpoint URL | `TF_PROVIDER_RGW_ENDPOINT` |
| `access_key` | Yes | Admin access key | `TF_PROVIDER_RGW_ACCESS_KEY` |
| `secret_key` | Yes | Admin secret key | `TF_PROVIDER_RGW_SECRET_KEY` |

**Security Note:** Store credentials in environment variables or use a secure secrets management solution rather than hardcoding them in configuration files.

### Example: Creating a User

```hcl
resource "rgw_user" "app_user" {
  username     = "application"
  display_name = "Application Service Account"
  email        = "admin@example.com"
}

output "app_access_key" {
  value     = rgw_user.app_user.access_key
  sensitive = true
}

output "app_secret_key" {
  value     = rgw_user.app_user.secret_key
  sensitive = true
}
```

### Example: User with Quotas

```hcl
resource "rgw_user" "limited_user" {
  username     = "limited-user"
  display_name = "User with Storage Limits"
  
  # User-level quota: 10GB total storage
  user_quota {
    enabled     = true
    max_size_kb = 10485760  # 10GB in KB
    max_objects = -1        # Unlimited objects
  }
  
  # Per-bucket quota: 1GB per bucket
  bucket_quota {
    enabled     = true
    max_size_kb = 1048576   # 1GB in KB
    max_objects = 100000
  }
}
```

### Example: Bucket and Policy

```hcl
resource "rgw_bucket" "app_bucket" {
  name = "application-data"
}

resource "rgw_bucket_policy" "app_policy" {
  bucket = rgw_bucket.app_bucket.name
  
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        AWS = ["arn:aws:iam:::user/${rgw_user.app_user.username}"]
      }
      Action = [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ]
      Resource = [
        "arn:aws:s3:::${rgw_bucket.app_bucket.name}",
        "arn:aws:s3:::${rgw_bucket.app_bucket.name}/*"
      ]
    }]
  })
}
```

## Resources

### rgw_user

Manages Ceph RadosGW users. See [documentation](docs/resources/user.md) for full schema.

### rgw_bucket

Manages storage buckets. See [documentation](docs/resources/bucket.md) for full schema.

**Import Example:**
```bash
terraform import rgw_bucket.example my-bucket-name
```

### rgw_bucket_policy

Manages bucket access policies. See [documentation](docs/resources/bucket_policy.md) for full schema.

## Development

### Building from Source

1. Clone the repository
2. Navigate to the repository directory
3. Build the provider:

```bash
go install
```

### Running Tests

```bash
# Unit tests
go test ./...

# Acceptance tests (creates real resources)
make testacc
```

**Warning:** Acceptance tests create actual resources in your Ceph cluster and may incur costs or consume storage.

### Generating Documentation

```bash
go generate
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes with appropriate tests
4. Submit a pull request

## License

This project is licensed under the Mozilla Public License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

This project is a fork of the original [terraform-provider-rgw](https://github.com/startnext/terraform-provider-rgw) created by Startnext. We extend our gratitude to the original authors and contributors for their foundational work.
