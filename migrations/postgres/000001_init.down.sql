-- Alexander Storage Database Schema
-- Migration: 000001_init (DOWN)
-- Description: Rollback initial schema

-- Drop functions
DROP FUNCTION IF EXISTS upsert_blob(CHAR(64), BIGINT, VARCHAR(512));
DROP FUNCTION IF EXISTS decrement_blob_ref(CHAR(64));
DROP FUNCTION IF EXISTS increment_blob_ref(CHAR(64));
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables (order matters due to foreign keys)
DROP TABLE IF EXISTS upload_parts;
DROP TABLE IF EXISTS multipart_uploads;
DROP TABLE IF EXISTS objects;
DROP TABLE IF EXISTS blobs;
DROP TABLE IF EXISTS buckets;
DROP TABLE IF EXISTS access_keys;
DROP TABLE IF EXISTS users;

-- Drop custom types
DROP TYPE IF EXISTS storage_class;
DROP TYPE IF EXISTS multipart_status;
DROP TYPE IF EXISTS access_key_status;
DROP TYPE IF EXISTS versioning_status;

-- Note: Extensions are not dropped as they may be used by other schemas
-- DROP EXTENSION IF EXISTS "pgcrypto";
-- DROP EXTENSION IF EXISTS "uuid-ossp";
