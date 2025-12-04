# MEMORY_BANK.md — Alexander Storage Project

> **Purpose**: This document serves as the persistent memory and context for the Alexander Storage project. It tracks architectural decisions, implementation progress, and serves as the single source of truth for this enterprise-grade S3-compatible object storage system.

---

## Table of Contents

1. [Architectural Blueprint](#section-1-architectural-blueprint)
2. [Feature Roadmap](#section-2-feature-roadmap)
3. [Decision Log](#section-3-decision-log)
4. [Current Context](#section-4-current-context)
5. [API Reference](#section-5-api-reference)
6. [Database Schema](#section-6-database-schema)

---

## Section 1: Architectural Blueprint

### High-Level System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                               CLIENT REQUEST                                     │
│                      (aws-cli, boto3, terraform, S3 SDKs)                       │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                             AUTH MIDDLEWARE                                      │
│   ┌──────────────┐    ┌───────────────┐    ┌──────────────────────────────────┐ │
│   │ Parse v4 Sig │───▶│ Lookup AccKey │───▶│ Decrypt SecretKey (AES-256-GCM) │ │
│   └──────────────┘    └───────────────┘    └──────────────────────────────────┘ │
│                                                          │                       │
│                                       ┌──────────────────┘                       │
│                                       ▼                                          │
│                        ┌──────────────────────────────┐                         │
│                        │ Verify HMAC-SHA256 Signature │                         │
│                        └──────────────────────────────┘                         │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              API HANDLERS (chi router)                           │
│          Bucket Handlers │ Object Handlers │ Multipart Handlers                 │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                               SERVICES LAYER                                     │
│      BucketService │ ObjectService │ IAMService │ MultipartService │ PresignSvc │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                      ┌─────────────────┼─────────────────┐
                      ▼                 ▼                 ▼
┌────────────────────────────┐ ┌─────────────────┐ ┌─────────────────────────────┐
│       POSTGRESQL           │ │      REDIS      │ │      CAS STORAGE            │
│   ┌──────────────────┐     │ │ ┌─────────────┐ │ │ ┌─────────────────────────┐ │
│   │ users            │     │ │ │ Metadata    │ │ │ │ /data/ab/cd/abcdef...  │ │
│   │ access_keys      │     │ │ │ Cache       │ │ │ │ (2-level sharding)     │ │
│   │ buckets          │     │ │ └─────────────┘ │ │ │                         │ │
│   │ blobs (ref_count)│     │ │ ┌─────────────┐ │ │ │ Interface: Backend      │ │
│   │ objects          │     │ │ │ Distributed │ │ │ │ • Store() → hash        │ │
│   │ multipart_*      │     │ │ │ Lock        │ │ │ │ • Retrieve(hash)        │ │
│   └──────────────────┘     │ │ └─────────────┘ │ │ │ • Delete(hash)          │ │
└────────────────────────────┘ └─────────────────┘ └─────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility |
|-----------|----------------|
| **Auth Middleware** | AWS v4 signature verification, access key lookup, secret decryption |
| **API Handlers** | HTTP request parsing, S3 XML response formatting, error handling |
| **Services Layer** | Business logic, transaction orchestration, validation |
| **Repositories** | Data access abstraction, SQL queries, cache interactions |
| **CAS Storage** | Content-addressable blob storage with deduplication |
| **PostgreSQL** | Persistent metadata storage, ACID transactions, ref counting |
| **Redis** | Metadata caching, distributed locking for concurrent operations |

### Data Flow: Object Upload

```
1. Client sends PUT /bucket/key with Authorization header
2. Auth Middleware:
   a. Parse AWS v4 signature from Authorization header
   b. Extract access_key_id from credentials
   c. Query PostgreSQL for access_key (via repository)
   d. Decrypt secret_key using AES-256-GCM master key
   e. Compute expected signature using HMAC-SHA256
   f. Compare signatures (constant-time comparison)
   g. Attach user context to request
3. Object Handler receives authenticated request
4. ObjectService.PutObject():
   a. Begin PostgreSQL transaction
   b. Stream body to temp file while computing SHA-256 hash
   c. UPSERT into blobs table (increment ref_count atomically)
   d. Check if blob file exists (deduplication)
   e. If versioned bucket: mark previous version is_latest=false
   f. INSERT new object record
   g. If replacing non-versioned: decrement old blob ref_count
   h. Move temp file to storage (if not duplicate)
   i. Commit transaction
5. Return success response with ETag and VersionID
```

---

## Section 2: Feature Roadmap

### Phase 1: Core Infrastructure (Current)
- [x] Project structure initialization
- [x] MEMORY_BANK.md creation
- [ ] Database migrations (users, access_keys, buckets, blobs, objects)
- [ ] Domain models (Go structs)
- [ ] Repository interfaces
- [ ] Storage interfaces
- [ ] Crypto utilities (AES-256-GCM)
- [ ] Configuration loading (Viper)
- [ ] Logging setup (Zerolog)

### Phase 2: IAM & Authentication
- [ ] AWS v4 signature parsing
- [ ] Signature verification (HMAC-SHA256)
- [ ] Access key management (create, list, revoke)
- [ ] User management (create, authenticate)
- [ ] Auth middleware integration
- [ ] Presigned URL generation
- [ ] Presigned URL verification

### Phase 3: Bucket Operations
- [ ] CreateBucket
- [ ] DeleteBucket
- [ ] ListBuckets
- [ ] HeadBucket
- [ ] GetBucketVersioning
- [ ] PutBucketVersioning

### Phase 4: Object Operations (Non-Versioned)
- [ ] PutObject (with CAS deduplication)
- [ ] GetObject
- [ ] HeadObject
- [ ] DeleteObject
- [ ] ListObjects (v1)
- [ ] ListObjectsV2
- [ ] CopyObject

### Phase 5: Versioning
- [ ] PutObject with version creation
- [ ] GetObject with versionId
- [ ] DeleteObject (create delete marker)
- [ ] DeleteObject with versionId
- [ ] ListObjectVersions

### Phase 6: Multipart Upload
- [ ] InitiateMultipartUpload
- [ ] UploadPart
- [ ] CompleteMultipartUpload
- [ ] AbortMultipartUpload
- [ ] ListMultipartUploads
- [ ] ListParts

### Phase 7: Operations & Observability
- [ ] Garbage collection for orphan blobs
- [ ] Prometheus metrics
- [ ] Health check endpoints
- [ ] Request tracing
- [ ] Rate limiting

### Phase 8: Advanced Features (Future)
- [ ] Bucket policies
- [ ] Object lifecycle rules
- [ ] Cross-region replication
- [ ] Server-side encryption
- [ ] Object locking (WORM)

---

## Section 3: Decision Log

### Decision 1: Content-Addressable Storage (CAS)

**Date**: 2024-12-04  
**Status**: ✅ Approved  

**Context**: Need to store object data efficiently with potential for deduplication.

**Decision**: Use SHA-256 content hashes as storage identifiers.

**Rationale**:
- **Deduplication**: Identical files stored once, saving disk space
- **Integrity**: Hash serves as built-in checksum
- **Simplicity**: File location derived from content, not metadata
- **Scalability**: Easy to distribute across storage nodes by hash prefix

**Implementation**:
- Store files at `/data/{first2chars}/{next2chars}/{full_sha256_hash}`
- Track reference count in `blobs` table
- Delete physical file only when ref_count reaches 0

---

### Decision 2: PostgreSQL for Metadata

**Date**: 2024-12-04  
**Status**: ✅ Approved  

**Context**: Need reliable metadata storage with ACID guarantees.

**Decision**: Use PostgreSQL with pgx driver.

**Rationale**:
- **ACID Transactions**: Critical for ref_count atomicity
- **Partial Indexes**: Enable efficient single-table versioning
- **JSONB**: Flexible metadata storage
- **Mature**: Battle-tested, excellent tooling
- **pgx Driver**: High performance, pure Go

**Alternatives Considered**:
- CockroachDB: Overkill for initial deployment
- MySQL: Less flexible indexing options
- MongoDB: ACID limitations for our use case

---

### Decision 3: Single-Table Versioning with Partial Index

**Date**: 2024-12-04  
**Status**: ✅ Approved  

**Context**: Need to support S3-compatible object versioning.

**Decision**: Store all versions in single `objects` table with `is_latest` flag.

**Rationale**:
- **Performance**: No JOINs for common "get latest" queries
- **Simplicity**: Single table to query/update
- **Partial Index**: `CREATE UNIQUE INDEX ... WHERE is_latest = TRUE` ensures only one latest per key

**Trade-offs**:
- Table grows with version history (mitigated by archival policies)

---

### Decision 4: AES-256-GCM for Secret Key Encryption

**Date**: 2024-12-04  
**Status**: ✅ Approved  

**Context**: Secret keys must be stored securely but decrypted at runtime for signature verification.

**Decision**: Encrypt secret keys with AES-256-GCM using application master key.

**Rationale**:
- **Authenticated Encryption**: GCM provides both confidentiality and integrity
- **Standard**: Well-audited, widely supported
- **Performance**: Hardware acceleration on modern CPUs
- **Runtime Decryption**: Necessary for AWS v4 signature verification

**Implementation**:
- Master key from environment variable (`ALEXANDER_AUTH_MASTER_KEY`)
- 32-byte key (256 bits)
- Random 12-byte nonce per encryption
- Store as: `nonce || ciphertext || tag` (base64 encoded)

---

### Decision 5: 2-Level Directory Sharding

**Date**: 2024-12-04  
**Status**: ✅ Approved  

**Context**: Storing millions of files in a single directory causes filesystem performance issues.

**Decision**: Use first 4 characters of SHA-256 hash for 2-level directory structure.

**Rationale**:
- **Distribution**: 65,536 possible leaf directories (256 × 256)
- **Lookup Performance**: O(1) path computation from hash
- **Filesystem Friendly**: Avoids ext4/NTFS directory limits

**Example**:
```
Hash: abcdef1234567890...
Path: /data/ab/cd/abcdef1234567890...
```

---

### Decision 6: Concatenate Multipart Uploads (Phase 1)

**Date**: 2024-12-04  
**Status**: ✅ Approved  

**Context**: Need to support multipart uploads for large files.

**Decision**: For Phase 1, concatenate parts into single file on completion.

**Rationale**:
- **Simplicity**: Single blob entry, standard retrieval
- **Compatibility**: Works with existing CAS deduplication

**Future Optimization**:
- Composite blob references (store parts separately, stream on retrieval)
- Would save disk I/O during completion
- Requires more complex retrieval logic

---

### Decision 7: AWS v4 Signature Implementation

**Date**: 2024-12-04  
**Status**: ✅ Approved  

**Context**: Need S3 API compatibility with standard tools (aws-cli, boto3, terraform).

**Decision**: Implement full AWS Signature Version 4 algorithm based on MinIO patterns.

**Rationale**:
- **Ecosystem Compatibility**: Works with all S3 tools
- **Security**: HMAC-SHA256 is cryptographically strong
- **Standard**: Well-documented algorithm

**Implementation Reference**: MinIO's `pkg/auth` package

---

## Section 4: Current Context

### Active Development Phase
**Phase 1: Core Infrastructure**

### Current Task
Creating database migrations and domain models

### Last Updated
2024-12-04

### Files Modified This Session
- `go.mod` - Project module definition
- `Makefile` - Build and development commands
- `Dockerfile` - Container build definition
- `.gitignore` - Git ignore rules
- `configs/config.yaml.example` - Configuration template
- `configs/docker-compose.yaml` - Local development stack
- `cmd/alexander-server/main.go` - Server entry point
- `cmd/alexander-admin/main.go` - Admin CLI entry point
- `cmd/alexander-migrate/main.go` - Migration tool entry point
- `MEMORY_BANK.md` - This file

### Pending Tasks
1. Create database migrations (`migrations/postgres/000001_init.up.sql`)
2. Implement domain models (`internal/domain/*.go`)
3. Implement crypto utilities (`internal/pkg/crypto/aes.go`)
4. Define storage interfaces (`internal/storage/interfaces.go`)
5. Define repository interfaces (`internal/repository/interfaces.go`)
6. Create auth types and constants (`internal/auth/*.go`)

### Known Issues
None currently.

### Technical Debt
None currently.

---

## Section 5: API Reference

### S3-Compatible Endpoints (Planned)

#### Bucket Operations
| Method | Path | Operation |
|--------|------|-----------|
| PUT | `/{bucket}` | CreateBucket |
| DELETE | `/{bucket}` | DeleteBucket |
| GET | `/` | ListBuckets |
| HEAD | `/{bucket}` | HeadBucket |
| GET | `/{bucket}?versioning` | GetBucketVersioning |
| PUT | `/{bucket}?versioning` | PutBucketVersioning |

#### Object Operations
| Method | Path | Operation |
|--------|------|-----------|
| PUT | `/{bucket}/{key}` | PutObject |
| GET | `/{bucket}/{key}` | GetObject |
| HEAD | `/{bucket}/{key}` | HeadObject |
| DELETE | `/{bucket}/{key}` | DeleteObject |
| GET | `/{bucket}?list-type=2` | ListObjectsV2 |
| GET | `/{bucket}?versions` | ListObjectVersions |

#### Multipart Operations
| Method | Path | Operation |
|--------|------|-----------|
| POST | `/{bucket}/{key}?uploads` | InitiateMultipartUpload |
| PUT | `/{bucket}/{key}?partNumber={n}&uploadId={id}` | UploadPart |
| POST | `/{bucket}/{key}?uploadId={id}` | CompleteMultipartUpload |
| DELETE | `/{bucket}/{key}?uploadId={id}` | AbortMultipartUpload |
| GET | `/{bucket}?uploads` | ListMultipartUploads |
| GET | `/{bucket}/{key}?uploadId={id}` | ListParts |

---

## Section 6: Database Schema

### Entity Relationship Diagram

```
┌──────────────┐       ┌──────────────────┐
│    users     │       │   access_keys    │
├──────────────┤       ├──────────────────┤
│ id (PK)      │◄──────│ user_id (FK)     │
│ username     │       │ id (PK)          │
│ email        │       │ access_key_id    │
│ password_hash│       │ encrypted_secret │
│ is_active    │       │ is_active        │
│ is_admin     │       │ expires_at       │
│ created_at   │       │ created_at       │
└──────────────┘       └──────────────────┘
       │
       │ owner_id
       ▼
┌──────────────┐       ┌──────────────────┐
│   buckets    │       │      blobs       │
├──────────────┤       ├──────────────────┤
│ id (PK)      │       │ content_hash(PK) │◄─────────┐
│ owner_id(FK) │       │ size             │          │
│ name (UQ)    │       │ storage_path     │          │
│ region       │       │ ref_count        │          │
│ versioning   │       │ created_at       │          │
│ created_at   │       └──────────────────┘          │
└──────────────┘                                     │
       │                                             │
       │ bucket_id                      content_hash │
       ▼                                             │
┌─────────────────────────────────────────────────────┐
│                      objects                        │
├─────────────────────────────────────────────────────┤
│ id (PK)                                             │
│ bucket_id (FK)                                      │
│ key                                                 │
│ version_id (UQ per bucket+key when is_latest)      │
│ is_latest ──────► PARTIAL UNIQUE INDEX             │
│ is_delete_marker                                   │
│ content_hash (FK) ─────────────────────────────────┘
│ size                                                │
│ content_type                                        │
│ etag                                                │
│ metadata (JSONB)                                    │
│ created_at                                          │
└─────────────────────────────────────────────────────┘

┌──────────────────┐       ┌──────────────────┐
│ multipart_uploads│       │   upload_parts   │
├──────────────────┤       ├──────────────────┤
│ id (PK)          │◄──────│ upload_id (FK)   │
│ bucket_id (FK)   │       │ id (PK)          │
│ key              │       │ part_number      │
│ initiated_at     │       │ content_hash     │
│ expires_at       │       │ size             │
│ metadata (JSONB) │       │ etag             │
└──────────────────┘       │ created_at       │
                           └──────────────────┘
```

### Key Indexes

```sql
-- Ensure only one "latest" version per bucket+key
CREATE UNIQUE INDEX idx_objects_latest 
    ON objects (bucket_id, key) 
    WHERE is_latest = TRUE;

-- Efficient bucket listing (only latest, non-deleted)
CREATE INDEX idx_objects_bucket_list 
    ON objects (bucket_id, key, created_at DESC) 
    WHERE is_latest = TRUE AND is_delete_marker = FALSE;

-- Find orphan blobs for garbage collection
CREATE INDEX idx_blobs_orphan 
    ON blobs (ref_count, created_at) 
    WHERE ref_count = 0;

-- Active access key lookup
CREATE INDEX idx_access_keys_lookup 
    ON access_keys (access_key_id) 
    WHERE is_active = TRUE;
```

---

## Appendix: Quick Reference

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ALEXANDER_AUTH_MASTER_KEY` | 64-char hex (32 bytes) for AES-256 | Required |
| `ALEXANDER_DATABASE_HOST` | PostgreSQL host | localhost |
| `ALEXANDER_DATABASE_PORT` | PostgreSQL port | 5432 |
| `ALEXANDER_REDIS_HOST` | Redis host | localhost |
| `ALEXANDER_REDIS_PORT` | Redis port | 6379 |
| `ALEXANDER_STORAGE_FILESYSTEM_BASE_PATH` | Blob storage path | /data |

### Generate Master Key

```bash
openssl rand -hex 32
```

### Common Commands

```bash
# Build all binaries
make build

# Run server
make run

# Run migrations
make migrate-up

# Start local dev environment
make docker-up

# Run tests
make test
```

---

*Last Updated: 2024-12-04*
