// Package config provides configuration management for the Alexander storage server.
// Configuration can be loaded from YAML files and environment variables.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config represents the complete application configuration.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Auth      AuthConfig      `mapstructure:"auth"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Metrics   MetricsConfig   `mapstructure:"metrics"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	GC        GCConfig        `mapstructure:"gc"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	MaxBodySize     int64         `mapstructure:"max_body_size"`
}

// DatabaseConfig holds database connection settings.
// Supports both PostgreSQL and SQLite backends.
type DatabaseConfig struct {
	// Driver specifies the database driver: "postgres" or "sqlite".
	// Default is "postgres" for backward compatibility.
	Driver string `mapstructure:"driver"`

	// PostgreSQL settings (used when Driver is "postgres")
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	Database        string        `mapstructure:"database"`
	SSLMode         string        `mapstructure:"ssl_mode"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`

	// SQLite settings (used when Driver is "sqlite")
	Path            string `mapstructure:"path"`             // Path to SQLite database file
	JournalMode     string `mapstructure:"journal_mode"`     // WAL, DELETE, TRUNCATE, etc.
	BusyTimeout     int    `mapstructure:"busy_timeout"`     // Milliseconds to wait for locks
	CacheSize       int    `mapstructure:"cache_size"`       // Page cache size (negative = KB)
	SynchronousMode string `mapstructure:"synchronous_mode"` // NORMAL, FULL, OFF
}

// DSN returns the PostgreSQL connection string.
// Only valid when Driver is "postgres".
func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode,
	)
}

// IsEmbedded returns true if using an embedded database (SQLite).
func (c DatabaseConfig) IsEmbedded() bool {
	return c.Driver == "sqlite"
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Host        string        `mapstructure:"host"`
	Port        int           `mapstructure:"port"`
	Password    string        `mapstructure:"password"`
	DB          int           `mapstructure:"db"`
	PoolSize    int           `mapstructure:"pool_size"`
	DialTimeout time.Duration `mapstructure:"dial_timeout"`
	Enabled     bool          `mapstructure:"enabled"`
}

// Addr returns the Redis address in host:port format.
func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// StorageConfig holds blob storage backend settings.
type StorageConfig struct {
	Backend   string                `mapstructure:"backend"`
	DataDir   string                `mapstructure:"data_dir"`
	TempDir   string                `mapstructure:"temp_dir"`
	S3        S3StorageConfig       `mapstructure:"s3"`
	Multipart MultipartUploadConfig `mapstructure:"multipart"`
}

// S3StorageConfig holds S3 backend settings (for future use).
type S3StorageConfig struct {
	Endpoint        string `mapstructure:"endpoint"`
	Region          string `mapstructure:"region"`
	Bucket          string `mapstructure:"bucket"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	UseSSL          bool   `mapstructure:"use_ssl"`
}

// MultipartUploadConfig holds multipart upload settings.
type MultipartUploadConfig struct {
	MinPartSize      int64         `mapstructure:"min_part_size"`
	MaxPartSize      int64         `mapstructure:"max_part_size"`
	MaxParts         int           `mapstructure:"max_parts"`
	UploadExpiration time.Duration `mapstructure:"upload_expiration"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	// EncryptionKey is the 32-byte key used for AES-256-GCM encryption of secret keys.
	// Must be exactly 32 bytes (256 bits) for AES-256.
	EncryptionKey string `mapstructure:"encryption_key"`

	// SSEMasterKey is the hex-encoded 32-byte master key for SSE-S3 encryption.
	// Used with HKDF to derive per-blob encryption keys.
	SSEMasterKey string `mapstructure:"sse_master_key"`

	// Region is the default region for AWS v4 signature verification.
	Region string `mapstructure:"region"`

	// Service is the service name for AWS v4 signature verification.
	Service string `mapstructure:"service"`

	// PresignedURLExpiration is the default expiration time for presigned URLs.
	PresignedURLExpiration time.Duration `mapstructure:"presigned_url_expiration"`

	// MaxSignatureAge is the maximum age of a signature before it's considered expired.
	MaxSignatureAge time.Duration `mapstructure:"max_signature_age"`
}

// GetEncryptionKey returns the encryption key as a byte slice.
// Returns an error if the key is not exactly 32 bytes.
func (c AuthConfig) GetEncryptionKey() ([]byte, error) {
	key := []byte(c.EncryptionKey)
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be exactly 32 bytes, got %d", len(key))
	}
	return key, nil
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	Output     string `mapstructure:"output"`
	TimeFormat string `mapstructure:"time_format"`
}

// MetricsConfig holds Prometheus metrics settings.
type MetricsConfig struct {
	// Enabled determines if metrics collection is active.
	Enabled bool `mapstructure:"enabled"`

	// Port is the port for the metrics HTTP server.
	Port int `mapstructure:"port"`

	// Path is the URL path for the metrics endpoint.
	Path string `mapstructure:"path"`
}

// RateLimitConfig holds rate limiting settings.
type RateLimitConfig struct {
	// Enabled determines if rate limiting is active.
	Enabled bool `mapstructure:"enabled"`

	// RequestsPerSecond is the rate of token refill per client.
	RequestsPerSecond float64 `mapstructure:"requests_per_second"`

	// BurstSize is the maximum number of tokens (burst capacity).
	BurstSize int `mapstructure:"burst_size"`

	// BandwidthEnabled enables bandwidth limiting.
	BandwidthEnabled bool `mapstructure:"bandwidth_enabled"`

	// BytesPerSecond is the bandwidth limit per client (in bytes).
	BytesPerSecond int64 `mapstructure:"bytes_per_second"`
}

// GCConfig holds garbage collection settings.
type GCConfig struct {
	// Enabled determines if automatic garbage collection runs.
	Enabled bool `mapstructure:"enabled"`

	// Interval is how often to run garbage collection.
	Interval time.Duration `mapstructure:"interval"`

	// GracePeriod is how long to wait before deleting orphan blobs.
	GracePeriod time.Duration `mapstructure:"grace_period"`

	// BatchSize is the maximum number of blobs to process per run.
	BatchSize int `mapstructure:"batch_size"`

	// DryRun logs what would be deleted without actually deleting.
	DryRun bool `mapstructure:"dry_run"`
}

// Load reads configuration from the specified file and environment variables.
// Environment variables take precedence over file values.
// Environment variables are prefixed with ALEXANDER_ and use _ as separator.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Environment variable configuration
	v.SetEnvPrefix("ALEXANDER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Config file configuration
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
		v.AddConfigPath("/etc/alexander")
	}

	// Read config file (optional - environment variables can be used instead)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is acceptable - use defaults and env vars
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values.
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 9000)
	v.SetDefault("server.read_timeout", 30*time.Second)
	v.SetDefault("server.write_timeout", 60*time.Second)
	v.SetDefault("server.idle_timeout", 120*time.Second)
	v.SetDefault("server.shutdown_timeout", 30*time.Second)
	v.SetDefault("server.max_body_size", 5*1024*1024*1024) // 5GB

	// Database defaults
	v.SetDefault("database.driver", "postgres")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "alexander")
	v.SetDefault("database.password", "")
	v.SetDefault("database.database", "alexander")
	v.SetDefault("database.ssl_mode", "prefer")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", 5*time.Minute)
	v.SetDefault("database.conn_max_idle_time", 5*time.Minute)
	// SQLite defaults
	v.SetDefault("database.path", "./data/alexander.db")
	v.SetDefault("database.journal_mode", "WAL")
	v.SetDefault("database.busy_timeout", 5000)
	v.SetDefault("database.cache_size", -2000)
	v.SetDefault("database.synchronous_mode", "NORMAL")

	// Redis defaults
	v.SetDefault("redis.host", "localhost")
	v.SetDefault("redis.port", 6379)
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.pool_size", 10)
	v.SetDefault("redis.dial_timeout", 5*time.Second)
	v.SetDefault("redis.enabled", true)

	// Storage defaults
	v.SetDefault("storage.backend", "filesystem")
	v.SetDefault("storage.data_dir", "./data/blobs")
	v.SetDefault("storage.temp_dir", "./data/temp")
	v.SetDefault("storage.multipart.min_part_size", 5*1024*1024)      // 5MB
	v.SetDefault("storage.multipart.max_part_size", 5*1024*1024*1024) // 5GB
	v.SetDefault("storage.multipart.max_parts", 10000)
	v.SetDefault("storage.multipart.upload_expiration", 7*24*time.Hour) // 7 days

	// Auth defaults
	v.SetDefault("auth.encryption_key", "") // Must be provided
	v.SetDefault("auth.region", "us-east-1")
	v.SetDefault("auth.service", "s3")
	v.SetDefault("auth.presigned_url_expiration", 15*time.Minute)
	v.SetDefault("auth.max_signature_age", 15*time.Minute)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")
	v.SetDefault("logging.time_format", time.RFC3339)

	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.port", 9091)
	v.SetDefault("metrics.path", "/metrics")

	// Rate limiting defaults
	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.requests_per_second", 100)
	v.SetDefault("rate_limit.burst_size", 200)
	v.SetDefault("rate_limit.bandwidth_enabled", false)
	v.SetDefault("rate_limit.bytes_per_second", 100*1024*1024) // 100 MB/s

	// Garbage collection defaults
	v.SetDefault("gc.enabled", true)
	v.SetDefault("gc.interval", 1*time.Hour)
	v.SetDefault("gc.grace_period", 24*time.Hour)
	v.SetDefault("gc.batch_size", 1000)
	v.SetDefault("gc.dry_run", false)
}

// Validate checks the configuration for required values and valid ranges.
func (c *Config) Validate() error {
	// Validate server configuration
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}

	// Validate database configuration
	validDrivers := map[string]bool{"postgres": true, "sqlite": true}
	if !validDrivers[c.Database.Driver] {
		return fmt.Errorf("database.driver must be 'postgres' or 'sqlite'")
	}

	if c.Database.Driver == "postgres" {
		if c.Database.Host == "" {
			return fmt.Errorf("database.host is required for postgres driver")
		}
		if c.Database.User == "" {
			return fmt.Errorf("database.user is required for postgres driver")
		}
		if c.Database.Database == "" {
			return fmt.Errorf("database.database is required for postgres driver")
		}
	} else if c.Database.Driver == "sqlite" {
		if c.Database.Path == "" {
			return fmt.Errorf("database.path is required for sqlite driver")
		}
	}

	// Validate storage configuration
	if c.Storage.Backend == "" {
		return fmt.Errorf("storage.backend is required")
	}
	if c.Storage.Backend == "filesystem" && c.Storage.DataDir == "" {
		return fmt.Errorf("storage.data_dir is required for filesystem backend")
	}

	// Validate auth configuration
	if c.Auth.EncryptionKey != "" {
		if len(c.Auth.EncryptionKey) != 32 {
			return fmt.Errorf("auth.encryption_key must be exactly 32 characters")
		}
	}

	// Validate logging configuration
	validLevels := map[string]bool{
		"trace": true, "debug": true, "info": true,
		"warn": true, "error": true, "fatal": true, "panic": true,
	}
	if !validLevels[strings.ToLower(c.Logging.Level)] {
		return fmt.Errorf("logging.level must be one of: trace, debug, info, warn, error, fatal, panic")
	}

	return nil
}

// MustLoad loads configuration or panics on error.
// Useful for main function initialization.
func MustLoad(configPath string) *Config {
	cfg, err := Load(configPath)
	if err != nil {
		panic(fmt.Sprintf("failed to load configuration: %v", err))
	}
	return cfg
}
