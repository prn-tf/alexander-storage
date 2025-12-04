-- Alexander Storage Database Schema
-- Migration: 000001_init
-- Description: Initial schema with users, access_keys, buckets, blobs, objects, and multipart tables

-- ============================================
-- EXTENSIONS
-- ============================================
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================
-- CUSTOM TYPES
-- ============================================

-- Bucket versioning status
CREATE TYPE versioning_status AS ENUM ('Disabled', 'Enabled', 'Suspended');

-- Access key status
CREATE TYPE access_key_status AS ENUM ('Active', 'Inactive');

-- Multipart upload status
CREATE TYPE multipart_status AS ENUM ('InProgress', 'Completed', 'Aborted');

-- Storage class
CREATE TYPE storage_class AS ENUM ('STANDARD', 'REDUCED_REDUNDANCY', 'GLACIER', 'DEEP_ARCHIVE');

-- ============================================
-- USERS TABLE
-- ============================================
CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    username        VARCHAR(255) NOT NULL,
    email           VARCHAR(255) NOT NULL,
    password_hash   VARCHAR(255) NOT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    is_admin        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT users_username_unique UNIQUE (username),
    CONSTRAINT users_email_unique UNIQUE (email),
    CONSTRAINT users_username_length CHECK (char_length(username) >= 3 AND char_length(username) <= 255),
    CONSTRAINT users_email_format CHECK (email ~* '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$')
);

-- Indexes for users
CREATE INDEX idx_users_username ON users (username);
CREATE INDEX idx_users_email ON users (email);
CREATE INDEX idx_users_active ON users (is_active) WHERE is_active = TRUE;

-- Trigger to update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================
-- ACCESS KEYS TABLE
-- ============================================
CREATE TABLE access_keys (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT NOT NULL,
    access_key_id       VARCHAR(20) NOT NULL,       -- AWS-style: 20 characters
    encrypted_secret    TEXT NOT NULL,               -- AES-256-GCM encrypted, base64 encoded
    description         VARCHAR(255),
    status              access_key_status NOT NULL DEFAULT 'Active',
    expires_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at        TIMESTAMPTZ,
    
    -- Foreign keys
    CONSTRAINT fk_access_keys_user FOREIGN KEY (user_id) 
        REFERENCES users(id) ON DELETE CASCADE,
    
    -- Constraints
    CONSTRAINT access_keys_access_key_id_unique UNIQUE (access_key_id),
    CONSTRAINT access_keys_access_key_id_length CHECK (char_length(access_key_id) = 20)
);

-- Indexes for access_keys
CREATE INDEX idx_access_keys_user_id ON access_keys (user_id);
CREATE INDEX idx_access_keys_lookup ON access_keys (access_key_id) WHERE status = 'Active';
CREATE INDEX idx_access_keys_expiry ON access_keys (expires_at) WHERE expires_at IS NOT NULL;

-- ============================================
-- BUCKETS TABLE
-- ============================================
CREATE TABLE buckets (
    id              BIGSERIAL PRIMARY KEY,
    owner_id        BIGINT NOT NULL,
    name            VARCHAR(63) NOT NULL,           -- S3 bucket name: 3-63 characters
    region          VARCHAR(63) NOT NULL DEFAULT 'us-east-1',
    versioning      versioning_status NOT NULL DEFAULT 'Disabled',
    object_lock     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Foreign keys
    CONSTRAINT fk_buckets_owner FOREIGN KEY (owner_id) 
        REFERENCES users(id) ON DELETE RESTRICT,
    
    -- Constraints
    CONSTRAINT buckets_name_unique UNIQUE (name),
    CONSTRAINT buckets_name_length CHECK (char_length(name) >= 3 AND char_length(name) <= 63),
    -- S3 bucket naming rules: lowercase, numbers, hyphens, no consecutive periods
    CONSTRAINT buckets_name_format CHECK (name ~* '^[a-z0-9][a-z0-9.-]*[a-z0-9]$')
);

-- Indexes for buckets
CREATE INDEX idx_buckets_owner_id ON buckets (owner_id);
CREATE INDEX idx_buckets_name ON buckets (name);

-- ============================================
-- BLOBS TABLE (Content-Addressable Storage)
-- ============================================
CREATE TABLE blobs (
    content_hash    CHAR(64) PRIMARY KEY,           -- SHA-256 hex: 64 characters
    size            BIGINT NOT NULL,
    storage_path    VARCHAR(512) NOT NULL,          -- Path in storage backend
    ref_count       INTEGER NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT blobs_ref_count_non_negative CHECK (ref_count >= 0),
    CONSTRAINT blobs_size_non_negative CHECK (size >= 0),
    CONSTRAINT blobs_content_hash_format CHECK (content_hash ~* '^[a-f0-9]{64}$')
);

-- Indexes for blobs
-- Index for garbage collection: find orphan blobs efficiently
CREATE INDEX idx_blobs_orphan ON blobs (ref_count, created_at) WHERE ref_count = 0;
-- Index for access time tracking
CREATE INDEX idx_blobs_last_accessed ON blobs (last_accessed);

-- ============================================
-- OBJECTS TABLE (with versioning support)
-- ============================================
CREATE TABLE objects (
    id                  BIGSERIAL PRIMARY KEY,
    bucket_id           BIGINT NOT NULL,
    key                 VARCHAR(1024) NOT NULL,         -- Object key (path)
    version_id          UUID NOT NULL DEFAULT uuid_generate_v4(),
    is_latest           BOOLEAN NOT NULL DEFAULT TRUE,
    is_delete_marker    BOOLEAN NOT NULL DEFAULT FALSE,
    content_hash        CHAR(64),                       -- NULL for delete markers
    size                BIGINT NOT NULL DEFAULT 0,
    content_type        VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
    etag                VARCHAR(64),                    -- Usually MD5 or composite hash
    storage_class       storage_class NOT NULL DEFAULT 'STANDARD',
    metadata            JSONB DEFAULT '{}',             -- User metadata (x-amz-meta-*)
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ,
    
    -- Foreign keys
    CONSTRAINT fk_objects_bucket FOREIGN KEY (bucket_id) 
        REFERENCES buckets(id) ON DELETE CASCADE,
    CONSTRAINT fk_objects_blob FOREIGN KEY (content_hash) 
        REFERENCES blobs(content_hash) ON DELETE RESTRICT,
    
    -- Constraints
    -- Delete markers must not have content
    CONSTRAINT objects_delete_marker_no_content 
        CHECK (is_delete_marker = FALSE OR content_hash IS NULL),
    -- Non-delete markers must have content (unless size is 0)
    CONSTRAINT objects_content_required 
        CHECK (is_delete_marker = TRUE OR content_hash IS NOT NULL OR size = 0),
    CONSTRAINT objects_size_non_negative CHECK (size >= 0)
);

-- *** CRITICAL: Partial unique index for versioning ***
-- Ensures only ONE object per bucket+key can be marked as "latest"
-- This is the key pattern for efficient single-table versioning
CREATE UNIQUE INDEX idx_objects_latest 
    ON objects (bucket_id, key) 
    WHERE is_latest = TRUE;

-- Core index for object lookups
CREATE INDEX idx_objects_bucket_key ON objects (bucket_id, key);

-- Index for version listing (newest first)
CREATE INDEX idx_objects_versions ON objects (bucket_id, key, created_at DESC);

-- Index for listing objects in a bucket (only latest versions, non-deleted)
CREATE INDEX idx_objects_bucket_list 
    ON objects (bucket_id, key, created_at DESC) 
    WHERE is_latest = TRUE AND is_delete_marker = FALSE;

-- Index for prefix queries (ListObjectsV2)
CREATE INDEX idx_objects_prefix 
    ON objects (bucket_id, key text_pattern_ops) 
    WHERE is_latest = TRUE AND is_delete_marker = FALSE;

-- Index by content_hash for deduplication queries
CREATE INDEX idx_objects_content_hash ON objects (content_hash) WHERE content_hash IS NOT NULL;

-- ============================================
-- MULTIPART UPLOADS TABLE
-- ============================================
CREATE TABLE multipart_uploads (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    bucket_id       BIGINT NOT NULL,
    key             VARCHAR(1024) NOT NULL,
    initiator_id    BIGINT NOT NULL,
    status          multipart_status NOT NULL DEFAULT 'InProgress',
    storage_class   storage_class NOT NULL DEFAULT 'STANDARD',
    metadata        JSONB DEFAULT '{}',
    initiated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '7 days'),
    completed_at    TIMESTAMPTZ,
    
    -- Foreign keys
    CONSTRAINT fk_multipart_uploads_bucket FOREIGN KEY (bucket_id) 
        REFERENCES buckets(id) ON DELETE CASCADE,
    CONSTRAINT fk_multipart_uploads_initiator FOREIGN KEY (initiator_id) 
        REFERENCES users(id) ON DELETE CASCADE
);

-- Indexes for multipart_uploads
CREATE INDEX idx_multipart_uploads_bucket ON multipart_uploads (bucket_id, key);
CREATE INDEX idx_multipart_uploads_expires ON multipart_uploads (expires_at) 
    WHERE status = 'InProgress';
CREATE INDEX idx_multipart_uploads_status ON multipart_uploads (status) 
    WHERE status = 'InProgress';

-- ============================================
-- UPLOAD PARTS TABLE
-- ============================================
CREATE TABLE upload_parts (
    id              BIGSERIAL PRIMARY KEY,
    upload_id       UUID NOT NULL,
    part_number     INTEGER NOT NULL,
    content_hash    CHAR(64) NOT NULL,              -- Each part references a blob
    size            BIGINT NOT NULL,
    etag            VARCHAR(64) NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Foreign keys
    CONSTRAINT fk_upload_parts_upload FOREIGN KEY (upload_id) 
        REFERENCES multipart_uploads(id) ON DELETE CASCADE,
    CONSTRAINT fk_upload_parts_blob FOREIGN KEY (content_hash) 
        REFERENCES blobs(content_hash) ON DELETE RESTRICT,
    
    -- Constraints
    CONSTRAINT upload_parts_unique UNIQUE (upload_id, part_number),
    CONSTRAINT upload_parts_part_number_range CHECK (part_number >= 1 AND part_number <= 10000),
    CONSTRAINT upload_parts_size_positive CHECK (size > 0)
);

-- Indexes for upload_parts
CREATE INDEX idx_upload_parts_upload_id ON upload_parts (upload_id, part_number);

-- ============================================
-- HELPER FUNCTIONS
-- ============================================

-- Function to atomically increment blob reference count
CREATE OR REPLACE FUNCTION increment_blob_ref(p_content_hash CHAR(64))
RETURNS void AS $$
BEGIN
    UPDATE blobs 
    SET ref_count = ref_count + 1,
        last_accessed = NOW()
    WHERE content_hash = p_content_hash;
END;
$$ LANGUAGE plpgsql;

-- Function to atomically decrement blob reference count
CREATE OR REPLACE FUNCTION decrement_blob_ref(p_content_hash CHAR(64))
RETURNS INTEGER AS $$
DECLARE
    new_count INTEGER;
BEGIN
    UPDATE blobs 
    SET ref_count = ref_count - 1
    WHERE content_hash = p_content_hash
    RETURNING ref_count INTO new_count;
    
    RETURN COALESCE(new_count, -1);
END;
$$ LANGUAGE plpgsql;

-- Function to upsert blob with atomic reference counting
CREATE OR REPLACE FUNCTION upsert_blob(
    p_content_hash CHAR(64),
    p_size BIGINT,
    p_storage_path VARCHAR(512)
)
RETURNS TABLE(is_new BOOLEAN, current_ref_count INTEGER) AS $$
DECLARE
    v_is_new BOOLEAN;
    v_ref_count INTEGER;
BEGIN
    -- Try to insert new blob
    INSERT INTO blobs (content_hash, size, storage_path, ref_count)
    VALUES (p_content_hash, p_size, p_storage_path, 1)
    ON CONFLICT (content_hash) DO UPDATE 
    SET ref_count = blobs.ref_count + 1,
        last_accessed = NOW()
    RETURNING 
        (xmax = 0) AS is_new,  -- xmax = 0 means INSERT, not UPDATE
        ref_count
    INTO v_is_new, v_ref_count;
    
    RETURN QUERY SELECT v_is_new, v_ref_count;
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- INITIAL DATA (Optional: Create admin user)
-- ============================================
-- Uncomment to create initial admin user (password should be hashed in application)
-- INSERT INTO users (username, email, password_hash, is_admin)
-- VALUES ('admin', 'admin@localhost', 'CHANGE_ME', TRUE);
