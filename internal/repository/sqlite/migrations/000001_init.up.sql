-- Alexander Storage Database Schema for SQLite
-- Migration: 000001_init
-- Description: Initial schema with users, access_keys, buckets, blobs, objects, and multipart tables
-- 
-- SQLite Compatibility Notes:
-- - No custom ENUM types (use TEXT with CHECK constraints)
-- - No UUID type (use TEXT with proper formatting)
-- - No TIMESTAMPTZ (use TEXT with ISO8601 format)
-- - No text_pattern_ops (use standard LIKE)
-- - No partial unique indexes (use workarounds)
-- - No xmax system column for upsert detection

-- ============================================
-- USERS TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    username        TEXT NOT NULL,
    email           TEXT NOT NULL,
    password_hash   TEXT NOT NULL,
    is_active       INTEGER NOT NULL DEFAULT 1,
    is_admin        INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    
    CONSTRAINT users_username_unique UNIQUE (username),
    CONSTRAINT users_email_unique UNIQUE (email),
    CONSTRAINT users_username_length CHECK (length(username) >= 3 AND length(username) <= 255)
);

-- Indexes for users
CREATE INDEX IF NOT EXISTS idx_users_username ON users (username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);
CREATE INDEX IF NOT EXISTS idx_users_active ON users (is_active) WHERE is_active = 1;

-- Trigger to update updated_at
CREATE TRIGGER IF NOT EXISTS users_updated_at
    AFTER UPDATE ON users
    FOR EACH ROW
    BEGIN
        UPDATE users SET updated_at = datetime('now') WHERE id = NEW.id;
    END;

-- ============================================
-- ACCESS KEYS TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS access_keys (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL,
    access_key_id       TEXT NOT NULL,       -- AWS-style: 20 characters
    encrypted_secret    TEXT NOT NULL,       -- AES-256-GCM encrypted, base64 encoded
    description         TEXT,
    status              TEXT NOT NULL DEFAULT 'Active' CHECK (status IN ('Active', 'Inactive')),
    expires_at          TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    last_used_at        TEXT,
    
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT access_keys_access_key_id_unique UNIQUE (access_key_id),
    CONSTRAINT access_keys_access_key_id_length CHECK (length(access_key_id) = 20)
);

-- Indexes for access_keys
CREATE INDEX IF NOT EXISTS idx_access_keys_user_id ON access_keys (user_id);
CREATE INDEX IF NOT EXISTS idx_access_keys_lookup ON access_keys (access_key_id);
CREATE INDEX IF NOT EXISTS idx_access_keys_expiry ON access_keys (expires_at);

-- ============================================
-- BUCKETS TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS buckets (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id        INTEGER NOT NULL,
    name            TEXT NOT NULL,                    -- S3 bucket name: 3-63 characters
    region          TEXT NOT NULL DEFAULT 'us-east-1',
    versioning      TEXT NOT NULL DEFAULT 'Disabled' CHECK (versioning IN ('Disabled', 'Enabled', 'Suspended')),
    object_lock     INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT buckets_name_unique UNIQUE (name),
    CONSTRAINT buckets_name_length CHECK (length(name) >= 3 AND length(name) <= 63)
);

-- Indexes for buckets
CREATE INDEX IF NOT EXISTS idx_buckets_owner_id ON buckets (owner_id);
CREATE INDEX IF NOT EXISTS idx_buckets_name ON buckets (name);

-- ============================================
-- BLOBS TABLE (Content-Addressable Storage)
-- ============================================
CREATE TABLE IF NOT EXISTS blobs (
    content_hash    TEXT PRIMARY KEY,           -- SHA-256 hex: 64 characters
    size            INTEGER NOT NULL,
    storage_path    TEXT NOT NULL,              -- Path in storage backend
    ref_count       INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    last_accessed   TEXT NOT NULL DEFAULT (datetime('now')),
    
    CONSTRAINT blobs_ref_count_non_negative CHECK (ref_count >= 0),
    CONSTRAINT blobs_size_non_negative CHECK (size >= 0),
    CONSTRAINT blobs_content_hash_length CHECK (length(content_hash) = 64)
);

-- Indexes for blobs
CREATE INDEX IF NOT EXISTS idx_blobs_orphan ON blobs (ref_count, created_at) WHERE ref_count = 0;
CREATE INDEX IF NOT EXISTS idx_blobs_last_accessed ON blobs (last_accessed);

-- ============================================
-- OBJECTS TABLE (with versioning support)
-- ============================================
CREATE TABLE IF NOT EXISTS objects (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    bucket_id           INTEGER NOT NULL,
    key                 TEXT NOT NULL,                      -- Object key (path)
    version_id          TEXT NOT NULL,                      -- UUID as text
    is_latest           INTEGER NOT NULL DEFAULT 1,
    is_delete_marker    INTEGER NOT NULL DEFAULT 0,
    content_hash        TEXT,                               -- NULL for delete markers
    size                INTEGER NOT NULL DEFAULT 0,
    content_type        TEXT NOT NULL DEFAULT 'application/octet-stream',
    etag                TEXT,                               -- Usually MD5 or composite hash
    storage_class       TEXT NOT NULL DEFAULT 'STANDARD' CHECK (storage_class IN ('STANDARD', 'REDUCED_REDUNDANCY', 'GLACIER', 'DEEP_ARCHIVE')),
    metadata            TEXT DEFAULT '{}',                  -- JSON for user metadata
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at          TEXT,
    
    FOREIGN KEY (bucket_id) REFERENCES buckets(id) ON DELETE CASCADE,
    FOREIGN KEY (content_hash) REFERENCES blobs(content_hash) ON DELETE RESTRICT,
    
    -- Delete markers must not have content
    CONSTRAINT objects_delete_marker_no_content 
        CHECK (is_delete_marker = 0 OR content_hash IS NULL),
    -- Non-delete markers must have content (unless size is 0)
    CONSTRAINT objects_content_required 
        CHECK (is_delete_marker = 1 OR content_hash IS NOT NULL OR size = 0),
    CONSTRAINT objects_size_non_negative CHECK (size >= 0)
);

-- Indexes for objects
-- Note: SQLite supports partial indexes with WHERE clause
CREATE UNIQUE INDEX IF NOT EXISTS idx_objects_latest 
    ON objects (bucket_id, key) 
    WHERE is_latest = 1;

CREATE INDEX IF NOT EXISTS idx_objects_bucket_key ON objects (bucket_id, key);
CREATE INDEX IF NOT EXISTS idx_objects_versions ON objects (bucket_id, key, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_objects_bucket_list 
    ON objects (bucket_id, key, created_at DESC) 
    WHERE is_latest = 1 AND is_delete_marker = 0;
CREATE INDEX IF NOT EXISTS idx_objects_content_hash ON objects (content_hash) WHERE content_hash IS NOT NULL;

-- ============================================
-- MULTIPART UPLOADS TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS multipart_uploads (
    id              TEXT PRIMARY KEY,                   -- UUID as text
    bucket_id       INTEGER NOT NULL,
    key             TEXT NOT NULL,
    initiator_id    INTEGER NOT NULL,
    status          TEXT NOT NULL DEFAULT 'InProgress' CHECK (status IN ('InProgress', 'Completed', 'Aborted')),
    storage_class   TEXT NOT NULL DEFAULT 'STANDARD' CHECK (storage_class IN ('STANDARD', 'REDUCED_REDUNDANCY', 'GLACIER', 'DEEP_ARCHIVE')),
    metadata        TEXT DEFAULT '{}',                  -- JSON for metadata
    initiated_at    TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at      TEXT NOT NULL DEFAULT (datetime('now', '+7 days')),
    completed_at    TEXT,
    
    FOREIGN KEY (bucket_id) REFERENCES buckets(id) ON DELETE CASCADE,
    FOREIGN KEY (initiator_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Indexes for multipart_uploads
CREATE INDEX IF NOT EXISTS idx_multipart_uploads_bucket ON multipart_uploads (bucket_id, key);
CREATE INDEX IF NOT EXISTS idx_multipart_uploads_expires ON multipart_uploads (expires_at);
CREATE INDEX IF NOT EXISTS idx_multipart_uploads_status ON multipart_uploads (status);

-- ============================================
-- UPLOAD PARTS TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS upload_parts (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    upload_id       TEXT NOT NULL,
    part_number     INTEGER NOT NULL,
    content_hash    TEXT NOT NULL,                      -- Each part references a blob
    size            INTEGER NOT NULL,
    etag            TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    
    FOREIGN KEY (upload_id) REFERENCES multipart_uploads(id) ON DELETE CASCADE,
    FOREIGN KEY (content_hash) REFERENCES blobs(content_hash) ON DELETE RESTRICT,
    
    CONSTRAINT upload_parts_unique UNIQUE (upload_id, part_number),
    CONSTRAINT upload_parts_part_number_range CHECK (part_number >= 1 AND part_number <= 10000),
    CONSTRAINT upload_parts_size_positive CHECK (size > 0)
);

-- Indexes for upload_parts
CREATE INDEX IF NOT EXISTS idx_upload_parts_upload_id ON upload_parts (upload_id, part_number);
