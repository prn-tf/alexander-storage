-- Alexander Storage Database Schema for SQLite
-- Migration: 000001_init (DOWN)
-- Description: Drops all tables

DROP TRIGGER IF EXISTS users_updated_at;

DROP TABLE IF EXISTS upload_parts;
DROP TABLE IF EXISTS multipart_uploads;
DROP TABLE IF EXISTS objects;
DROP TABLE IF EXISTS blobs;
DROP TABLE IF EXISTS buckets;
DROP TABLE IF EXISTS access_keys;
DROP TABLE IF EXISTS users;
