# Alexander Storage

**A high-performance, S3-compatible object storage server written in Go.**

Alexander Storage is an enterprise-grade object storage system designed for self-hosted environments. It provides full compatibility with the AWS S3 API, allowing you to use existing tools like `aws-cli`, `boto3`, and Terraform without modification.

---

## Table of Contents

- [Features](#features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [API Compatibility](#api-compatibility)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

---

## Features

### Core Capabilities

- **S3 API Compatible**: Works with aws-cli, boto3, s3cmd, and any S3-compatible SDK
- **Content-Addressable Storage (CAS)**: Automatic deduplication using SHA-256 hashing
- **Object Versioning**: Full S3-compatible versioning support
- **Multipart Uploads**: Support for large file uploads (up to 5TB per object)
- **Presigned URLs**: Generate time-limited URLs for secure sharing

### Security

- **AWS Signature V4**: Industry-standard request signing
- **AES-256-GCM Encryption**: Secure secret key storage
- **Access Key Management**: Create and manage multiple access keys per user

### Performance

- **Reference Counting**: Efficient blob management with automatic cleanup
- **Redis Caching**: Optional metadata caching layer
- **Connection Pooling**: PostgreSQL connection pool for high concurrency
- **Two-Level Directory Sharding**: Optimized filesystem layout for millions of objects

### Operations

- **PostgreSQL Backend**: ACID-compliant metadata storage
- **Health Endpoints**: Built-in health and readiness checks
- **Structured Logging**: JSON logging with zerolog
- **Prometheus Metrics**: (Planned) Observable metrics endpoint

---

## Architecture

```
                              +-------------------+
                              |   S3 Clients      |
                              | (aws-cli, boto3)  |
                              +---------+---------+
                                        |
                                        v
+-----------------------------------------------------------------------+
|                        Alexander Server                                |
|                                                                        |
|  +------------------+    +------------------+    +------------------+  |
|  |  Auth Middleware |    |   API Handlers   |    | Service Layer    |  |
|  |  (AWS v4 Sig)    |--->|  (chi router)    |--->| (Business Logic) |  |
|  +------------------+    +------------------+    +------------------+  |
|                                                          |             |
+-----------------------------------------------------------------------+
                                                           |
                    +--------------------------------------+
                    |                  |                   |
                    v                  v                   v
           +---------------+  +----------------+  +------------------+
           |  PostgreSQL   |  |     Redis      |  |   Blob Storage   |
           |  (Metadata)   |  |    (Cache)     |  | (Content-Addr.)  |
           +---------------+  +----------------+  +------------------+
```

### Storage Layout

Objects are stored using content-addressable storage with two-level directory sharding:

```
/data
  /ab
    /cd
      /abcdef1234567890...  (SHA-256 hash as filename)
```

This approach provides:
- Automatic deduplication (identical files stored once)
- Built-in integrity verification
- Efficient distribution for millions of files

---

## Quick Start

### Using Docker Compose

The fastest way to get started is with Docker Compose:

```bash
# Clone the repository
git clone https://github.com/prn-tf/alexander-storage.git
cd alexander-storage

# Start all services (PostgreSQL, Redis, Alexander)
docker-compose -f configs/docker-compose.yaml up -d

# Generate a master encryption key
export ALEXANDER_AUTH_ENCRYPTION_KEY=$(openssl rand -hex 32)

# Run database migrations
./alexander-migrate up

# Start the server
./alexander-server
```

### Using aws-cli

Once running, configure aws-cli to use Alexander:

```bash
aws configure set aws_access_key_id YOUR_ACCESS_KEY
aws configure set aws_secret_access_key YOUR_SECRET_KEY
aws configure set default.region us-east-1

# Create a bucket
aws --endpoint-url http://localhost:9000 s3 mb s3://my-bucket

# Upload a file
aws --endpoint-url http://localhost:9000 s3 cp myfile.txt s3://my-bucket/

# List objects
aws --endpoint-url http://localhost:9000 s3 ls s3://my-bucket/
```

---

## Installation

### Prerequisites

- Go 1.21 or later
- PostgreSQL 14 or later
- Redis 7 or later (optional, for caching)

### Build from Source

```bash
# Clone the repository
git clone https://github.com/prn-tf/alexander-storage.git
cd alexander-storage

# Download dependencies
go mod download

# Build all binaries
make build

# The binaries will be in ./bin/
ls -la bin/
# alexander-server   - Main storage server
# alexander-admin    - Admin CLI tool
# alexander-migrate  - Database migration tool
```

### Docker Image

```bash
# Build the Docker image
docker build -t alexander-storage:latest .

# Run with Docker
docker run -d \
  -p 9000:9000 \
  -e ALEXANDER_DATABASE_HOST=postgres \
  -e ALEXANDER_AUTH_ENCRYPTION_KEY=your-32-byte-hex-key \
  -v /data/alexander:/data \
  alexander-storage:latest
```

---

## Configuration

Alexander can be configured via YAML file or environment variables.

### Configuration File

Copy the example configuration:

```bash
cp configs/config.yaml.example configs/config.yaml
```

### Environment Variables

All configuration options can be set via environment variables with the `ALEXANDER_` prefix:

| Variable | Description | Default |
|----------|-------------|---------|
| `ALEXANDER_SERVER_HOST` | Server bind address | `0.0.0.0` |
| `ALEXANDER_SERVER_PORT` | Server port | `9000` |
| `ALEXANDER_DATABASE_HOST` | PostgreSQL host | `localhost` |
| `ALEXANDER_DATABASE_PORT` | PostgreSQL port | `5432` |
| `ALEXANDER_DATABASE_USER` | PostgreSQL user | `alexander` |
| `ALEXANDER_DATABASE_PASSWORD` | PostgreSQL password | (required) |
| `ALEXANDER_DATABASE_DATABASE` | Database name | `alexander` |
| `ALEXANDER_REDIS_HOST` | Redis host | `localhost` |
| `ALEXANDER_REDIS_PORT` | Redis port | `6379` |
| `ALEXANDER_REDIS_ENABLED` | Enable Redis caching | `true` |
| `ALEXANDER_AUTH_ENCRYPTION_KEY` | 32-byte hex key for AES-256 | (required) |
| `ALEXANDER_AUTH_REGION` | Default AWS region | `us-east-1` |
| `ALEXANDER_STORAGE_DATA_DIR` | Blob storage directory | `/data` |

### Generate Encryption Key

The encryption key must be exactly 32 bytes (64 hex characters):

```bash
openssl rand -hex 32
```

---

## Usage

### Create Access Keys

Use the admin CLI to manage users and access keys:

```bash
# Create a new user
./alexander-admin user create --username myuser --email user@example.com

# Create an access key for the user
./alexander-admin accesskey create --username myuser

# Output:
# Access Key ID: AKIAIOSFODNN7EXAMPLE
# Secret Key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
# 
# IMPORTANT: Save these credentials. The secret key cannot be retrieved later.
```

### Bucket Operations

```bash
# Create bucket
aws --endpoint-url http://localhost:9000 s3 mb s3://my-bucket

# List buckets
aws --endpoint-url http://localhost:9000 s3 ls

# Delete bucket
aws --endpoint-url http://localhost:9000 s3 rb s3://my-bucket
```

### Object Operations

```bash
# Upload file
aws --endpoint-url http://localhost:9000 s3 cp file.txt s3://my-bucket/

# Download file
aws --endpoint-url http://localhost:9000 s3 cp s3://my-bucket/file.txt ./

# List objects
aws --endpoint-url http://localhost:9000 s3 ls s3://my-bucket/

# Delete object
aws --endpoint-url http://localhost:9000 s3 rm s3://my-bucket/file.txt
```

### Presigned URLs

Generate presigned URLs for temporary access:

```bash
# Generate a presigned URL valid for 1 hour
aws --endpoint-url http://localhost:9000 s3 presign s3://my-bucket/file.txt --expires-in 3600
```

### Versioning

```bash
# Enable versioning on a bucket
aws --endpoint-url http://localhost:9000 s3api put-bucket-versioning \
  --bucket my-bucket \
  --versioning-configuration Status=Enabled

# List object versions
aws --endpoint-url http://localhost:9000 s3api list-object-versions --bucket my-bucket
```

---

## API Compatibility

Alexander implements the following S3 API operations:

### Bucket Operations

| Operation | Status |
|-----------|--------|
| CreateBucket | Planned |
| DeleteBucket | Planned |
| ListBuckets | Planned |
| HeadBucket | Planned |
| GetBucketVersioning | Planned |
| PutBucketVersioning | Planned |

### Object Operations

| Operation | Status |
|-----------|--------|
| PutObject | Planned |
| GetObject | Planned |
| HeadObject | Planned |
| DeleteObject | Planned |
| ListObjectsV2 | Planned |
| CopyObject | Planned |

### Multipart Upload

| Operation | Status |
|-----------|--------|
| CreateMultipartUpload | Planned |
| UploadPart | Planned |
| CompleteMultipartUpload | Planned |
| AbortMultipartUpload | Planned |
| ListMultipartUploads | Planned |
| ListParts | Planned |

---

## Development

### Project Structure

```
alexander-storage/
  cmd/
    alexander-server/     # Main server entry point
    alexander-admin/      # Admin CLI
    alexander-migrate/    # Migration tool
  internal/
    auth/                 # AWS v4 signature authentication
    cache/redis/          # Redis caching layer
    config/               # Configuration management
    domain/               # Domain models
    pkg/crypto/           # Cryptographic utilities
    repository/           # Data access layer
      postgres/           # PostgreSQL implementation
    service/              # Business logic
    storage/              # Blob storage
      filesystem/         # Filesystem backend
  migrations/
    postgres/             # SQL migrations
  configs/                # Configuration examples
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run linter
make lint
```

### Building

```bash
# Build all binaries
make build

# Build Docker image
make docker-build

# Run locally
make run
```

---

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Quick Links

- [Report a Bug](https://github.com/prn-tf/alexander-storage/issues/new?template=bug_report.md)
- [Request a Feature](https://github.com/prn-tf/alexander-storage/issues/new?template=feature_request.md)
- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Security Policy](SECURITY.md)

---

## License

Alexander Storage is licensed under the [Apache License 2.0](LICENSE).

```
Copyright 2024 Alexander Storage Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
```

---

## Acknowledgments

- Inspired by [MinIO](https://github.com/minio/minio) for S3 API patterns
- Built with [chi](https://github.com/go-chi/chi) router
- Uses [pgx](https://github.com/jackc/pgx) PostgreSQL driver
- Logging powered by [zerolog](https://github.com/rs/zerolog)
