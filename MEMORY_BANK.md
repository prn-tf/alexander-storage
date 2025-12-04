# MEMORY_BANK.md — Alexander Storage Project

> **Purpose**: This document serves as the persistent memory and context for the Alexander Storage project. It tracks architectural decisions, implementation progress, and serves as the single source of truth for this enterprise-grade S3-compatible object storage system.

---

## Table of Contents

1. [Architectural Blueprint](#section-1-architectural-blueprint)
2. [Feature Roadmap](#section-2-feature-roadmap)
3. [Decision Log](#section-3-decision-log)
4. [Current Context](#section-4-current-context)
5. [Technical Debt](#section-5-technical-debt)
6. [API Reference](#section-6-api-reference)
7. [Database Schema](#section-7-database-schema)

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

### Phase 8: Architecture Improvements (Community Requested) ✅ COMPLETED
> **Community Feedback**: "PostgreSQL + Redis is overkill for single-node deployments."

- [x] Embedded database support (SQLite) - `internal/repository/sqlite/`
- [x] Memory-based locking for single-node mode - `internal/lock/memory.go`
- [x] In-memory cache for single-node mode - `internal/cache/memory/cache.go`
- [x] Repository factory for database abstraction - `internal/repository/factory.go`
- [x] Single binary deployment mode

**Implementation Details:**

**SQLite Support:**
- Pure Go SQLite driver (modernc.org/sqlite) - no CGO required
- Full repository implementations matching PostgreSQL interface
- WAL mode enabled for better concurrency
- Embedded migrations via `//go:embed`
- Same schema structure adapted for SQLite syntax

**Memory-Based Locking:**
- `internal/lock/interfaces.go` - Locker abstraction interface
- `internal/lock/memory.go` - In-memory lock with expiration and auto-cleanup
- `internal/lock/noop.go` - No-op lock for testing scenarios
- Automatic mode selection: distributed (Redis) vs single-node (memory)

**In-Memory Cache:**
- `internal/cache/memory/cache.go` - Thread-safe cache with TTL
- Implements same interface as Redis cache
- Background cleanup of expired entries
- Graceful shutdown support

**Configuration:**
```yaml
# Embedded mode
database:
  driver: "sqlite"           # or "postgres"
  path: "./data/alexander.db"
  journal_mode: "WAL"
  busy_timeout: 5000
  cache_size: -2000          # 2MB
  synchronous_mode: "NORMAL"

# Single-node: Redis disabled, uses memory cache/lock
redis:
  enabled: false
```

**Deployment Modes:**
1. **Single-Node/Embedded**: SQLite + memory cache/lock
   - No external dependencies
   - Single binary deployment
   - Ideal for dev/testing/small deployments

2. **Distributed**: PostgreSQL + Redis
   - Horizontal scalability
   - Distributed locking
   - Shared cache across nodes

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

### Decision 9: Embedded SQLite Database Support

**Date**: 2025-12-04 (Planned), 2025-06-25 (Implemented)  
**Status**: ✅ Approved & Implemented  

**Context**: Community feedback requesting "zero-dependency" single-binary deployment.

**Decision**: Add SQLite as alternative metadata backend using modernc.org/sqlite (pure Go).

**Rationale**:
- **True Zero-Dependency**: Single binary, no external services (no CGO required)
- **Homelab Friendly**: `./alexander-server` just works
- **Edge Deployments**: IoT, embedded systems, air-gapped networks
- **Cross-Platform**: Pure Go compiles to any target without C compiler

**Implementation**:
- `internal/repository/sqlite/` - All repository implementations
- `internal/repository/sqlite/migrations/` - Embedded SQL migrations
- `internal/lock/memory.go` - In-memory locking (no Redis needed)
- `internal/cache/memory/cache.go` - In-memory caching (no Redis needed)
- Config: `database.driver: postgres|sqlite`

**SQLite-Specific Adaptations**:
- No `RETURNING ... WHERE xmax = 0` for upsert detection
- No `ANY($1::type[])` - uses manual placeholder generation
- TEXT instead of TIMESTAMPTZ with ISO8601 format
- INTEGER (0/1) instead of BOOLEAN
- Embedded migrations via `//go:embed`

**Performance Tuning**:
```yaml
database:
  driver: sqlite
  path: ./data/alexander.db
  max_open_conns: 1          # SQLite single-writer
  journal_mode: WAL          # Write-Ahead Logging
  busy_timeout: 5000         # 5 seconds
  cache_size: -2000          # 2MB page cache
  synchronous_mode: NORMAL   # Balanced durability/speed
```

---

## Section 4: Current Context

### Active Development Phase
**Phase 9: Advanced Features** (Planning)

### Current Task
Phase 8 completed. Ready for Phase 9: Advanced features like bucket policies, lifecycle rules, etc.

### Last Updated
2025-06-25

### Completed Phases
- ✅ Phase 1: Core Infrastructure
- ✅ Phase 2: IAM & Authentication
- ✅ Phase 3: Bucket Operations
- ✅ Phase 4: Object Operations
- ✅ Phase 5: Versioning
- ✅ Phase 6: Multipart Upload
- ✅ Phase 7: Operations & Observability
- ✅ Phase 8: Architecture Improvements

### Files Modified This Session (Phase 8)
- `internal/lock/interfaces.go` - Lock abstraction interface
- `internal/lock/memory.go` - In-memory lock implementation
- `internal/lock/noop.go` - No-op lock for testing
- `internal/cache/memory/cache.go` - In-memory cache implementation
- `internal/repository/sqlite/db.go` - SQLite connection management
- `internal/repository/sqlite/errors.go` - SQLite error translation
- `internal/repository/sqlite/user_repo.go` - SQLite user repository
- `internal/repository/sqlite/accesskey_repo.go` - SQLite access key repository
- `internal/repository/sqlite/bucket_repo.go` - SQLite bucket repository
- `internal/repository/sqlite/blob_repo.go` - SQLite blob repository
- `internal/repository/sqlite/object_repo.go` - SQLite object repository
- `internal/repository/sqlite/multipart_repo.go` - SQLite multipart repository
- `internal/repository/sqlite/migrations/000001_init.up.sql` - SQLite schema
- `internal/repository/sqlite/migrations/000001_init.down.sql` - SQLite rollback
- `internal/repository/factory.go` - Repository factory
- `internal/config/config.go` - Added SQLite driver support
- `internal/repository/postgres/db.go` - Updated Close() to return error
- `cmd/alexander-server/main.go` - Dual database driver support
- `configs/config.embedded.yaml.example` - Embedded mode config example
- `MEMORY_BANK.md` - Phase 8 documentation

### Pending Tasks (Phase 9)
1. Bucket policies (IAM-like access control)
2. Object lifecycle rules
3. Cross-region replication
4. Server-side encryption
5. Object locking (WORM)
6. Web Dashboard (webui)
7. Python and PHP SDK

### Known Issues
None currently.

### Community Feedback Addressed
- [x] Added "Best for: Archival, Backups, Homelabs" to README
- [x] Added Mermaid architecture diagrams
- [x] Clarified io.TeeReader streaming hash in docs
- [x] ~~Marked Multipart Upload as HIGH PRIORITY~~ → COMPLETED
- [x] Documented Redis as optional for single-node
- [x] ~~Added future SQLite/BadgerDB support to roadmap~~ → COMPLETED (SQLite)
- [x] Added benchmark section placeholder
- [x] **Single binary deployment mode** → COMPLETED

---

## Section 5: Technical Debt

> **Purpose**: Track known technical debt, missing implementations, and areas requiring improvement. Items are prioritized and should be addressed before adding new features.

### 🔴 High Priority (Blocking Features)

#### TD-001: Redis Distributed Lock Not Implemented
**Status**: ✅ Completed (2025-01-06)  
**Files**: `internal/lock/redis.go`  
**Description**: Redis-based distributed lock implemented. Uses adapter pattern to wrap `cache/redis.DistributedLock` as `lock.Locker` interface.

**Implementation**:
- Created `internal/lock/redis.go` with `RedisLocker` struct
- Wraps existing `cache/redis.DistributedLock` functionality
- Implements all 5 interface methods: `Acquire`, `AcquireWithRetry`, `Release`, `Extend`, `IsHeld`
- Compile-time interface check included

---

#### TD-002: Lock Not Integrated Into Services
**Status**: ✅ Completed (2025-01-06)  
**Files**: `cmd/alexander-server/main.go`, `internal/service/*.go`  
**Description**: Locker is now integrated into services for concurrent operation safety.

**Changes**:
- Added `locker lock.Locker` parameter to `ObjectService`, `MultipartService`, `GarbageCollector` constructors
- `GarbageCollector.runWithContext()` acquires distributed lock before processing
- `cmd/alexander-server/main.go` passes locker to all services (removed `_ = locker`)
- Test files updated to use `lock.NewNoOpLocker()`

---

### 🟡 Medium Priority (Quality & Maintainability)

#### TD-003: Redis Cache Interface Mismatch
**Status**: ✅ Already Correct (Verified 2025-01-06)  
**Files**: `internal/cache/redis/cache.go`, `internal/repository/cache.go`  
**Description**: Both Redis and memory caches correctly implement the same `repository.Cache` interface.

**Verification**: Both caches implement `Get`, `Set`, `Delete`, `Exists` methods with identical signatures.

---

#### TD-004: Low Test Coverage
**Status**: ⚠️ Partial  
**Files**: `internal/*/`  
**Description**: Only 3 service test files exist. Missing tests for repositories, handlers, auth, and integration.

**Current Test Files**:
- `internal/service/bucket_service_test.go` ✅
- `internal/service/multipart_service_test.go` ✅
- `internal/service/object_service_test.go` ✅

**Missing Tests**:
- [ ] `internal/repository/postgres/*_test.go`
- [ ] `internal/repository/sqlite/*_test.go`
- [ ] `internal/handler/*_test.go`
- [ ] `internal/auth/*_test.go`
- [ ] `internal/lock/*_test.go`
- [ ] `internal/cache/memory/*_test.go`
- [ ] Integration tests (end-to-end S3 compatibility)

**Target**: Minimum 60% code coverage

---

#### TD-005: Duplicate SQLite Migration Files
**Status**: ✅ Completed (2025-01-06)  
**Files**: `internal/repository/sqlite/migrations/` (kept)

**Description**: Removed duplicate `migrations/sqlite/` directory. Only the embedded version in `internal/repository/sqlite/migrations/` remains.

**Action Taken**: Deleted `migrations/sqlite/000001_init.up.sql` and `migrations/sqlite/000001_init.down.sql`.

---

### 🟢 Low Priority (Future Optimization)

#### TD-006: Multipart Concatenation Bug (CRITICAL - NOW FIXED)
**Status**: ✅ Completed (2025-01-06)  
**Files**: `internal/service/multipart_service.go`  
**Description**: Fixed critical bug where `CompleteMultipartUpload` only stored the first part's data.

**Previous Behavior (BUG)**:
- Only used first part's content hash for final object
- All other parts were ignored, causing data loss

**Fixed Implementation**:
- Added `concatenateParts(ctx, contentHashes, totalSize)` method
- Uses `io.MultiReader` to stream all parts together efficiently
- Computes correct combined SHA-256 hash
- Registers combined blob and returns correct hash
- Memory-efficient: streams data without loading all parts in memory

---

#### TD-007: Admin CLI Completeness
**Status**: ✅ Completed (2025-01-06)  
**Files**: `cmd/alexander-admin/main.go`  
**Description**: Full admin CLI implemented with all management commands.

**Implemented Commands**:
- `user create|list|get|delete` - Full user management with JSON output option
- `accesskey create|list|revoke` - Access key lifecycle management
- `bucket list|delete|set-versioning` - Bucket administration
- `gc run|status` - Manual garbage collection with dry-run support

**Features**:
- Confirmation prompts for destructive operations (`--force` to skip)
- JSON output mode for scripting (`--json`)
- Automatic password generation for user creation
- Support for both PostgreSQL and SQLite backends

---

### 📊 Technical Debt Summary

| ID | Title | Priority | Status | Effort |
|----|-------|----------|--------|--------|
| TD-001 | Redis Distributed Lock | 🔴 High | ✅ Completed | 4h |
| TD-002 | Lock Integration | 🔴 High | ✅ Completed | 8h |
| TD-003 | Redis Cache Interface | 🟡 Medium | ✅ Verified OK | 0h |
| TD-004 | Test Coverage | 🟡 Medium | ⚠️ Partial | 16h+ |
| TD-005 | Duplicate Migrations | 🟡 Medium | ✅ Completed | 0.5h |
| TD-006 | Multipart Concatenation Bug | 🔴 Critical | ✅ Fixed | 4h |
| TD-007 | Admin CLI | 🟢 Low | ✅ Completed | 4h |

**Remaining Effort**: ~16+ hours (TD-004 Test Coverage)

---

### Resolution Log

| Date | ID | Action | Notes |
|------|-----|--------|-------|
| 2025-01-06 | TD-001 | Created `internal/lock/redis.go` | Adapter pattern wrapping `cache/redis.DistributedLock` |
| 2025-01-06 | TD-002 | Integrated locker into services | Updated constructors for ObjectService, MultipartService, GCService |
| 2025-01-06 | TD-003 | Verified interface compatibility | No changes needed - both caches implement same interface |
| 2025-01-06 | TD-005 | Deleted `migrations/sqlite/` | Kept embedded migrations in `internal/repository/sqlite/migrations/` |
| 2025-01-06 | TD-006 | Fixed multipart concatenation | Added `concatenateParts()` method using `io.MultiReader` |
| 2025-01-06 | TD-007 | Implemented full admin CLI | User, accesskey, bucket, gc commands with JSON output |

---

## Section 6: API Reference

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

## Section 7: Database Schema

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
