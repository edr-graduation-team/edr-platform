# Certificate Directory

This directory should contain the TLS certificates for the gRPC server.

## Required Files

| File | Description |
|------|-------------|
| `ca.crt` | Root CA certificate (PEM format) |
| `server.crt` | Server certificate signed by CA (PEM format) |
| `server.key` | Server private key (PEM format) |
| `jwt_private.pem` | RSA private key for JWT signing (2048-bit) |
| `jwt_public.pem` | RSA public key for JWT verification |

## Generating Test Certificates

For development/testing, you can generate certificates using the provided script:

```bash
# Run from project root
./scripts/generate_certs.sh
```

Or manually with OpenSSL:

```bash
# 1. Generate Root CA
openssl genrsa -out ca.key 4096
openssl req -x509 -new -nodes -key ca.key -sha256 -days 1024 -out ca.crt \
    -subj "/C=US/ST=State/L=City/O=Antigravity EDR/CN=Antigravity Root CA"

# 2. Generate Server Certificate
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr \
    -subj "/C=US/ST=State/L=City/O=Antigravity EDR/CN=edr-server"
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out server.crt -days 365 -sha256

# 3. Generate JWT Keys (RS256)
openssl genrsa -out jwt_private.pem 2048
openssl rsa -in jwt_private.pem -pubout -out jwt_public.pem
```

## Security Notes

> [!WARNING]
> **Never commit private keys to version control!**

- The `.gitignore` file excludes `*.pem` and `*.key` files
- Use environment variables or secret management in production
- Rotate certificates regularly (recommended: 90 days)
- Store CA private key in HSM or secure storage

## Production Recommendations

1. Use a proper PKI or HSM for CA key storage
2. Use HashiCorp Vault or similar for secret management
3. Automate certificate rotation
4. Monitor certificate expiry dates
5. Use short-lived certificates where possible
