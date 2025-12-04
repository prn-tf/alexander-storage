# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial project structure and architecture
- AWS Signature V4 authentication
  - Request signing verification
  - Presigned URL generation and verification
  - Access key management
- User management service
  - User creation with bcrypt password hashing
  - User authentication
  - User activation/deactivation
- IAM service for access key operations
  - Create, list, activate, deactivate, and delete access keys
  - AES-256-GCM encryption for secret key storage
  - Automatic expired key cleanup
- PostgreSQL repositories for all domain entities
  - Users, access keys, buckets, blobs, objects, multipart uploads
  - Connection pooling with pgx
- Redis caching layer (optional)
  - Metadata caching
  - Distributed locking support
- Configuration management with Viper
  - YAML file configuration
  - Environment variable overrides
- Content-addressable storage (CAS) with filesystem backend
  - SHA-256 based deduplication
  - Two-level directory sharding
  - Reference counting for blob management
- Structured logging with zerolog
- Database migrations with golang-migrate
- Docker and Docker Compose support

### Security

- AES-256-GCM encryption for secret key storage
- Bcrypt password hashing for user passwords
- AWS Signature V4 request authentication
- Constant-time signature comparison

## [0.1.0] - TBD

Initial release (planned)

### Planned Features

- Bucket operations (CreateBucket, DeleteBucket, ListBuckets, HeadBucket)
- Object operations (PutObject, GetObject, HeadObject, DeleteObject)
- Object versioning support
- Multipart upload support
- Prometheus metrics endpoint
- Health check endpoints

---

## Version History

### Versioning Scheme

We use [Semantic Versioning](https://semver.org/):

- **MAJOR**: Incompatible API changes
- **MINOR**: New functionality (backwards compatible)
- **PATCH**: Bug fixes (backwards compatible)

### Pre-1.0 Development

During pre-1.0 development:
- The API is not considered stable
- Minor versions may include breaking changes
- Patch versions are for bug fixes only

### Release Process

1. Update CHANGELOG.md with release notes
2. Update version in relevant files
3. Create a git tag: `git tag v0.1.0`
4. Push tag: `git push origin v0.1.0`
5. GitHub Actions will build and publish release artifacts
