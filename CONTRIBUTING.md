# Contributing to Alexander Storage

Thank you for your interest in contributing to Alexander Storage! This document provides guidelines and information for contributors.

---

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Coding Standards](#coding-standards)
- [Testing](#testing)
- [Submitting Changes](#submitting-changes)
- [Issue Guidelines](#issue-guidelines)

---

## Code of Conduct

This project adheres to a [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

---

## Getting Started

### Prerequisites

Before you begin, ensure you have the following installed:

- **Go 1.21+**: [Download Go](https://golang.org/dl/)
- **PostgreSQL 14+**: [Download PostgreSQL](https://www.postgresql.org/download/)
- **Redis 7+**: [Download Redis](https://redis.io/download/) (optional)
- **Docker** and **Docker Compose**: For running the full stack locally
- **Make**: For build automation
- **golangci-lint**: For code linting

### Fork and Clone

1. Fork the repository on GitHub
2. Clone your fork locally:

```bash
git clone https://github.com/YOUR_USERNAME/alexander-storage.git
cd alexander-storage
```

3. Add the upstream repository:

```bash
git remote add upstream https://github.com/prn-tf/alexander-storage.git
```

4. Keep your fork synchronized:

```bash
git fetch upstream
git checkout main
git merge upstream/main
```

---

## Development Setup

### 1. Install Dependencies

```bash
go mod download
```

### 2. Start Infrastructure

Use Docker Compose to start PostgreSQL and Redis:

```bash
docker-compose -f configs/docker-compose.yaml up -d postgres redis
```

### 3. Configure Environment

Copy the example configuration:

```bash
cp configs/config.yaml.example configs/config.yaml
```

Set required environment variables:

```bash
export ALEXANDER_AUTH_ENCRYPTION_KEY=$(openssl rand -hex 32)
export ALEXANDER_DATABASE_PASSWORD=yourpassword
```

### 4. Run Migrations

```bash
go run cmd/alexander-migrate/main.go up
```

### 5. Build and Run

```bash
make build
./bin/alexander-server
```

---

## Making Changes

### Branch Naming

Use descriptive branch names:

- `feature/add-bucket-versioning`
- `fix/signature-verification-bug`
- `docs/update-readme`
- `refactor/auth-middleware`

### Commit Messages

Follow the conventional commit format:

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, missing semicolons, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Examples:**

```
feat(auth): add presigned URL generation

Implement presigned URL generation for GET, PUT, and DELETE operations.
URLs are valid for a configurable duration (default 15 minutes).

Closes #42
```

```
fix(storage): handle concurrent blob deletion

Add distributed locking to prevent race conditions when
decrementing blob reference counts.
```

---

## Coding Standards

### Go Style Guide

We follow the standard Go style guidelines:

- Use `gofmt` for formatting
- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

### Project-Specific Guidelines

1. **Error Handling**
   - Always wrap errors with context: `fmt.Errorf("failed to do X: %w", err)`
   - Define domain-specific errors in `internal/domain/errors.go`
   - Use sentinel errors for expected conditions

2. **Logging**
   - Use structured logging with zerolog
   - Include relevant context fields
   - Use appropriate log levels:
     - `Debug`: Detailed debugging information
     - `Info`: General operational information
     - `Warn`: Warning conditions
     - `Error`: Error conditions

3. **Comments**
   - All exported types, functions, and methods must have doc comments
   - Use complete sentences starting with the item name
   - Include usage examples for complex APIs

4. **Package Structure**
   - Keep packages focused and cohesive
   - Avoid circular dependencies
   - Use internal packages for private code

### Code Organization

```
internal/
  domain/       # Core business entities (no external dependencies)
  repository/   # Data access interfaces and implementations
  service/      # Business logic layer
  auth/         # Authentication and authorization
  storage/      # Blob storage abstraction
  config/       # Configuration management
  pkg/          # Shared utilities (crypto, etc.)
```

### Linting

Run the linter before submitting:

```bash
make lint
```

We use `golangci-lint` with the configuration in `.golangci.yml`.

---

## Testing

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run specific package tests
go test ./internal/auth/...

# Run with verbose output
go test -v ./...
```

### Writing Tests

1. **Unit Tests**
   - Place tests in the same package as the code
   - Name test files `*_test.go`
   - Use table-driven tests when appropriate

2. **Integration Tests**
   - Use build tags for integration tests: `//go:build integration`
   - Require external dependencies (database, Redis)
   - Clean up test data after each test

3. **Test Coverage**
   - Aim for at least 70% coverage for new code
   - Critical paths (auth, storage) should have higher coverage

**Example Test:**

```go
func TestEncryptor_EncryptDecrypt(t *testing.T) {
    tests := []struct {
        name      string
        plaintext string
    }{
        {"empty string", ""},
        {"short string", "hello"},
        {"long string", strings.Repeat("x", 1000)},
    }

    key := make([]byte, 32)
    rand.Read(key)
    enc, _ := crypto.NewEncryptor(key)

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            encrypted, err := enc.EncryptString(tt.plaintext)
            require.NoError(t, err)

            decrypted, err := enc.DecryptString(encrypted)
            require.NoError(t, err)
            assert.Equal(t, tt.plaintext, decrypted)
        })
    }
}
```

---

## Submitting Changes

### Pull Request Process

1. **Update your fork** with the latest upstream changes
2. **Create a feature branch** from `main`
3. **Make your changes** following the coding standards
4. **Write or update tests** for your changes
5. **Run the full test suite** and linter
6. **Push to your fork** and create a pull request

### Pull Request Checklist

Before submitting, ensure:

- [ ] Code follows the project's coding standards
- [ ] All tests pass (`make test`)
- [ ] Linter passes (`make lint`)
- [ ] New code has adequate test coverage
- [ ] Documentation is updated if needed
- [ ] Commit messages follow the conventional format
- [ ] CHANGELOG.md is updated for notable changes

### Review Process

1. A maintainer will review your PR within a few days
2. Address any feedback or requested changes
3. Once approved, a maintainer will merge your PR

---

## Issue Guidelines

### Reporting Bugs

When reporting bugs, include:

1. **Summary**: A clear and concise description
2. **Steps to Reproduce**: Detailed steps to reproduce the issue
3. **Expected Behavior**: What you expected to happen
4. **Actual Behavior**: What actually happened
5. **Environment**: OS, Go version, PostgreSQL version, etc.
6. **Logs**: Relevant log output (redact sensitive information)

### Requesting Features

When requesting features, include:

1. **Problem Statement**: What problem does this solve?
2. **Proposed Solution**: Your suggested approach
3. **Alternatives Considered**: Other solutions you've thought about
4. **Use Cases**: How would this feature be used?

### Issue Labels

- `bug`: Something isn't working
- `enhancement`: New feature or request
- `documentation`: Documentation improvements
- `good first issue`: Good for newcomers
- `help wanted`: Extra attention is needed
- `question`: Further information is requested

---

## Recognition

Contributors are recognized in:

- The project's README.md
- Release notes for significant contributions
- The GitHub contributors page

---

## Questions?

If you have questions, feel free to:

- Open a [Discussion](https://github.com/prn-tf/alexander-storage/discussions)
- Ask in an issue with the `question` label

Thank you for contributing to Alexander Storage!
