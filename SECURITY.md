# Security Policy

## Supported Versions

We release patches for security vulnerabilities for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 0.x.x   | Yes (current)      |

## Reporting a Vulnerability

We take the security of Alexander Storage seriously. If you have discovered a security vulnerability, please report it responsibly.

### How to Report

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please send an email to the maintainers with the following information:

1. **Description**: A clear description of the vulnerability
2. **Impact**: What an attacker could accomplish with this vulnerability
3. **Steps to Reproduce**: Detailed steps to reproduce the issue
4. **Affected Versions**: Which versions are affected
5. **Suggested Fix**: If you have a suggested fix, please include it

### What to Expect

1. **Acknowledgment**: We will acknowledge receipt of your report within 48 hours
2. **Assessment**: We will assess the vulnerability and determine its severity
3. **Updates**: We will keep you informed of our progress
4. **Resolution**: We will work to resolve the issue as quickly as possible
5. **Credit**: With your permission, we will credit you in the security advisory

### Response Timeline

- **Critical vulnerabilities**: We aim to release a fix within 7 days
- **High severity**: We aim to release a fix within 14 days
- **Medium/Low severity**: We aim to release a fix within 30 days

## Security Best Practices

When deploying Alexander Storage, follow these security best practices:

### Encryption Key Management

1. **Generate strong keys**: Use `openssl rand -hex 32` for the encryption key
2. **Secure storage**: Store the encryption key in a secrets manager (e.g., HashiCorp Vault, AWS Secrets Manager)
3. **Key rotation**: Plan for key rotation procedures
4. **Never commit keys**: Ensure encryption keys are never committed to version control

### Network Security

1. **TLS/HTTPS**: Always use TLS in production. Place Alexander behind a reverse proxy (nginx, Caddy) with TLS
2. **Firewall**: Restrict access to the server port (default 9000)
3. **Internal networks**: If possible, run Alexander on internal networks only

### Database Security

1. **Strong passwords**: Use strong, unique passwords for PostgreSQL
2. **Network isolation**: Run PostgreSQL on a private network
3. **SSL connections**: Enable SSL for PostgreSQL connections in production
4. **Regular backups**: Maintain encrypted backups of the database

### Access Control

1. **Principle of least privilege**: Create separate access keys for different applications
2. **Key rotation**: Regularly rotate access keys
3. **Audit logging**: Monitor access key usage
4. **Disable unused keys**: Deactivate access keys that are no longer needed

### Operational Security

1. **Keep updated**: Regularly update to the latest version
2. **Monitor logs**: Set up log aggregation and alerting
3. **Resource limits**: Configure appropriate resource limits
4. **Regular audits**: Periodically audit access and permissions

## Known Security Considerations

### AWS Signature V4

Alexander implements AWS Signature V4 for request authentication. This provides:

- Request integrity verification
- Replay attack prevention (via timestamp validation)
- Credential protection (secret key never transmitted)

### Secret Key Storage

Access key secrets are stored encrypted using AES-256-GCM. The encryption provides:

- Confidentiality (secrets are encrypted at rest)
- Integrity (GCM authentication tag prevents tampering)
- Unique nonces (prevent pattern analysis)

### Presigned URLs

Presigned URLs have configurable expiration times. Consider:

- Using short expiration times for sensitive content
- Maximum expiration is 7 days (per AWS S3 specification)
- URLs include the signature, which cannot be revoked

## Vulnerability Disclosure

We follow a coordinated disclosure process:

1. Reporter submits vulnerability privately
2. We acknowledge and assess the report
3. We develop and test a fix
4. We prepare a security advisory
5. We release the fix and publish the advisory
6. Reporter is credited (with permission)

## Past Security Advisories

No security advisories have been issued yet.

---

Thank you for helping keep Alexander Storage and its users safe!
