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

### Phase 1: Core Infrastructure ✅ COMPLETED
- [x] Project structure initialization
- [x] MEMORY_BANK.md creation
- [x] Database migrations (users, access_keys, buckets, blobs, objects)
- [x] Domain models (Go structs)
- [x] Repository interfaces
- [x] Storage interfaces
- [x] Crypto utilities (AES-256-GCM)
- [x] Configuration loading (Viper) - `internal/config/config.go`
- [x] Logging setup (Zerolog)

### Phase 2: IAM & Authentication ✅ COMPLETED
- [x] AWS v4 signature parsing - `internal/auth/parser.go`
- [x] Signature verification (HMAC-SHA256) - `internal/auth/signature_v4.go`
- [x] Auth middleware integration - `internal/auth/middleware.go`
- [x] PostgreSQL repositories - `internal/repository/postgres/`
- [x] Redis cache implementation - `internal/cache/redis/`
- [x] Presigned URL verification - `internal/auth/middleware.go` (handlePresignedV4)
- [x] IAM Service layer - `internal/service/iam_service.go`
- [x] User Service layer - `internal/service/user_service.go`
- [x] Presigned URL generation service - `internal/service/presign_service.go`
- [x] AccessKeyStore adapter for auth middleware integration

### Phase 3: Bucket Operations ✅ COMPLETED
- [x] CreateBucket - `internal/service/bucket_service.go`, `internal/handler/bucket_handler.go`
- [x] DeleteBucket - `internal/service/bucket_service.go`, `internal/handler/bucket_handler.go`
- [x] ListBuckets - `internal/service/bucket_service.go`, `internal/handler/bucket_handler.go`
- [x] HeadBucket - `internal/service/bucket_service.go`, `internal/handler/bucket_handler.go`
- [x] GetBucketVersioning - `internal/service/bucket_service.go`, `internal/handler/bucket_handler.go`
- [x] PutBucketVersioning - `internal/service/bucket_service.go`, `internal/handler/bucket_handler.go`
- [x] HTTP Router - `internal/handler/router.go`
- [x] Server Integration - `cmd/alexander-server/main.go`

### Phase 4: Object Operations ✅ COMPLETED
- [x] PutObject (with CAS deduplication) - `internal/service/object_service.go`, `internal/handler/object_handler.go`
- [x] GetObject - `internal/service/object_service.go`, `internal/handler/object_handler.go`
- [x] HeadObject - `internal/service/object_service.go`, `internal/handler/object_handler.go`
- [x] DeleteObject - `internal/service/object_service.go`, `internal/handler/object_handler.go`
- [x] ListObjects (v1) - `internal/service/object_service.go`, `internal/handler/object_handler.go`
- [x] ListObjectsV2 - `internal/service/object_service.go`, `internal/handler/object_handler.go`
- [x] CopyObject - `internal/service/object_service.go`, `internal/handler/object_handler.go`
- [x] Router Integration - `internal/handler/router.go`
- [x] Server Integration - `cmd/alexander-server/main.go`

### Phase 5: Versioning ✅ COMPLETED
- [x] PutObject with version creation - Versioning enabled bucket'larda yeni version oluşturulur
- [x] GetObject with versionId - `?versionId=xxx` query parametresi ile belirli version getirme
- [x] DeleteObject (create delete marker) - Versioning enabled bucket'larda delete marker oluşturma
- [x] DeleteObject with versionId - Belirli version'ı permanent silme
- [x] ListObjectVersions - `GET /{bucket}?versions` endpoint'i

### Phase 6: Multipart Upload ✅ COMPLETED
> **Community Feedback**: "Without multipart uploads, large files can't be uploaded reliably. This is critical for S3 compatibility."

- [x] InitiateMultipartUpload - `internal/service/multipart_service.go`, `internal/handler/multipart_handler.go`
- [x] UploadPart - `internal/service/multipart_service.go`, `internal/handler/multipart_handler.go`
- [x] CompleteMultipartUpload - `internal/service/multipart_service.go`, `internal/handler/multipart_handler.go`
- [x] AbortMultipartUpload - `internal/service/multipart_service.go`, `internal/handler/multipart_handler.go`
- [x] ListMultipartUploads - `internal/service/multipart_service.go`, `internal/handler/multipart_handler.go`
- [x] ListParts - `internal/service/multipart_service.go`, `internal/handler/multipart_handler.go`

**Implementation Details:**
- Multipart uploads stored as `in_progress` with 7-day expiration
- Parts stored with ContentHash for CAS deduplication
- CompleteMultipartUpload assembles parts in order and creates final object
- AbortMultipartUpload cleans up parts and decrements blob references
- Router integration: `?uploads`, `?uploadId=X`, `?partNumber=N`

**API Endpoints:**
```
POST /bucket/key?uploads                    → InitiateMultipartUpload
PUT  /bucket/key?partNumber=N&uploadId=X    → UploadPart
POST /bucket/key?uploadId=X                 → CompleteMultipartUpload
DELETE /bucket/key?uploadId=X               → AbortMultipartUpload
GET  /bucket?uploads                        → ListMultipartUploads
GET  /bucket/key?uploadId=X                 → ListParts
```

### Phase 7: Operations & Observability ✅ COMPLETED
- [x] Garbage collection for orphan blobs - `internal/service/gc_service.go`
- [x] Prometheus metrics - `internal/metrics/metrics.go`
- [x] Health check endpoints - `internal/handler/health.go`
- [x] Request tracing - `internal/middleware/tracing.go`
- [x] Rate limiting - `internal/middleware/ratelimit.go`

**Implementation Details:**

**Garbage Collection:**
- Automatic background GC with configurable interval (default: 1 hour)
- Grace period prevents deleting blobs during active uploads (default: 24 hours)
- Batch processing with configurable size (default: 1000 blobs per run)
- Dry run mode for testing without actual deletion
- Tracks orphan blobs (ref_count = 0) and cleans up both DB and storage

**Prometheus Metrics:**
- Separate metrics server on configurable port (default: 9091)
- HTTP request metrics: total, duration, in-flight, response size
- Storage metrics: operations, duration, bytes transferred
- Auth metrics: attempts, failures with reasons
- GC metrics: runs, blobs deleted, bytes freed, duration
- Rate limiting metrics: requests limited by type

**Health Endpoints:**
```
GET /health     → Full component health with latency
GET /healthz    → Kubernetes liveness probe
GET /readyz     → Kubernetes readiness probe
```
- Component-level status (database, storage)
- Cached responses for efficiency (default: 5s TTL)
- Status levels: healthy, degraded, unhealthy

**Request Tracing:**
- Automatic request ID generation (X-Request-ID header)
- Trace ID propagation for distributed tracing
- S3-compatible headers (x-amz-request-id, x-amz-id-2)
- Structured logging with request context
- Path normalization for low-cardinality metrics

**Rate Limiting:**
- Token bucket algorithm per client IP
- Configurable rate (default: 100 req/s) and burst (default: 200)
- S3-compatible SlowDown error response
- Automatic bucket cleanup for stale clients
- Optional bandwidth limiting support

**Configuration:**
```yaml
metrics:
  enabled: true
  port: 9091
  path: /metrics

rate_limit:
  enabled: true
  requests_per_second: 100
  burst_size: 200

gc:
  enabled: true
  interval: 1h
  grace_period: 24h
  batch_size: 1000
```

### Phase 8: Architecture Improvements (Community Requested)
> **Community Feedback**: "PostgreSQL + Redis is overkill for single-node deployments."

- [ ] Embedded database support (SQLite or BadgerDB)
- [ ] Memory-based locking for single-node mode (eliminate Redis dependency)
- [ ] Single binary deployment mode

### Phase 9: Advanced Features (Future)
- [ ] Bucket policies
- [ ] Object lifecycle rules
- [ ] Cross-region replication
- [ ] Server-side encryption
- [ ] Object locking (WORM)
- [ ] WEB Dashboard (webui)
- [ ] Python and PHP sdk

---

## Section 3: Decision Log

### Decision 1: Content-Addressable Storage (CAS)

**Date**: 2025-12-04  
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

**Date**: 2025-12-04  
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

**Date**: 2025-12-04  
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

**Date**: 2025-12-04  
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

**Date**: 2025-12-04  
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

**Date**: 2025-12-04  
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

**Date**: 2025-12-04  
**Status**: ✅ Approved  

**Context**: Need S3 API compatibility with standard tools (aws-cli, boto3, terraform).

**Decision**: Implement full AWS Signature Version 4 algorithm based on MinIO patterns.

**Rationale**:
- **Ecosystem Compatibility**: Works with all S3 tools
- **Security**: HMAC-SHA256 is cryptographically strong
- **Standard**: Well-documented algorithm

**Implementation Reference**: MinIO's `pkg/auth` package

---

### Decision 8: Optional Redis for Single-Node Deployments

**Date**: 2025-12-04  
**Status**: ✅ Approved (Documentation Updated)  

**Context**: Community feedback that PostgreSQL + Redis is "overkill" for simple homelab deployments.

**Decision**: Make Redis optional. Single-node deployments use in-memory locking.

**Rationale**:
- **Simpler Deployment**: `docker run` with just PostgreSQL
- **Lower Resource Usage**: No Redis process for small deployments
- **Scalability Path**: Redis enabled for cluster/HA deployments

**Implementation**:
- `internal/lock` interface abstraction
- `MemoryLocker` for single-node (sync.Mutex)
- `RedisLocker` for distributed deployments
- Config flag: `storage.distributed_mode: true|false`

---

### Decision 9: Future Embedded Database Support

**Date**: 2025-12-04  
**Status**: 🔜 Planned  

**Context**: Community feedback requesting "zero-dependency" single-binary deployment.

**Decision**: Add SQLite or BadgerDB as alternative metadata backend (Phase 8).

**Rationale**:
- **True Zero-Dependency**: Single binary, no external services
- **Homelab Friendly**: `./alexander-server` just works
- **Edge Deployments**: IoT, embedded systems, air-gapped networks

**Implementation Plan**:
- Repository interface already supports this (abstraction exists)
- Add `internal/repository/sqlite/` or `internal/repository/badger/`
- Config: `database.driver: postgres|sqlite|badger`

**Trade-offs**:
- SQLite: Limited concurrent writes, but excellent for read-heavy archival
- BadgerDB: Better write performance, Go-native, but less tooling

---

## Section 4: Current Context

### Active Development Phase
**Phase 8: Architecture Improvements**

### Current Task
Planning next phase: Embedded database support, single-node optimization

### Last Updated
2025-12-04

### Completed Phases
- ✅ Phase 1: Core Infrastructure
- ✅ Phase 2: IAM & Authentication
- ✅ Phase 3: Bucket Operations
- ✅ Phase 4: Object Operations
- ✅ Phase 5: Versioning
- ✅ Phase 6: Multipart Upload
- ✅ Phase 7: Operations & Observability

### Files Modified This Session
- `internal/metrics/metrics.go` - Prometheus metrics definitions
- `internal/middleware/ratelimit.go` - Token bucket rate limiting
- `internal/middleware/tracing.go` - Request tracing and correlation IDs
- `internal/service/gc_service.go` - Garbage collection service
- `internal/handler/health.go` - Enhanced health check endpoints
- `internal/handler/router.go` - Integrated new middleware
- `internal/config/config.go` - Added metrics, rate_limit, gc config sections
- `internal/storage/interfaces.go` - Added HealthCheck method
- `internal/storage/filesystem/storage.go` - Implemented HealthCheck
- `internal/storage/errors.go` - Added IsNotFound helper
- `cmd/alexander-server/main.go` - Wired GC, metrics server, middleware
- `MEMORY_BANK.md` - Updated with Phase 7 completion

### Pending Tasks
1. Embedded database support (SQLite/BadgerDB) - Phase 8
2. Memory-based locking for single-node mode - Phase 8
3. Single binary deployment mode - Phase 8

### Known Issues
None currently.

### Community Feedback Addressed
- [x] Added "Best for: Archival, Backups, Homelabs" to README
- [x] Added Mermaid architecture diagrams
- [x] Clarified io.TeeReader streaming hash in docs
- [x] ~~Marked Multipart Upload as HIGH PRIORITY~~ → COMPLETED
- [x] Documented Redis as optional for single-node
- [x] Added future SQLite/BadgerDB support to roadmap
- [x] Added benchmark section placeholder

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

*Last Updated: 2025-12-04*
