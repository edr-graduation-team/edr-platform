#!/bin/bash
# Generate development certificates for testing
# DO NOT USE IN PRODUCTION

set -e

CERTS_DIR="$(dirname "$0")/../certs"
mkdir -p "$CERTS_DIR"
cd "$CERTS_DIR"

echo "Generating development certificates..."

# 1. Generate Root CA
echo "Generating Root CA..."
openssl genrsa -out ca.key 4096
openssl req -x509 -new -nodes -key ca.key -sha256 -days 1024 -out ca.crt \
    -subj "/C=US/ST=State/L=City/O=Antigravity EDR/OU=Development/CN=Antigravity Dev CA"

# 2. Generate Server Certificate
echo "Generating Server Certificate..."
openssl genrsa -out server.key 2048

# Create server certificate config
cat > server.cnf << EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
req_extensions = v3_req

[dn]
C = US
ST = State
L = City
O = Antigravity EDR
OU = Server
CN = edr-server

[v3_req]
basicConstraints = CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = edr-server
DNS.3 = *.edr.local
IP.1 = 127.0.0.1
IP.2 = ::1
EOF

openssl req -new -key server.key -out server.csr -config server.cnf
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out server.crt -days 365 -sha256 -extensions v3_req -extfile server.cnf

# 3. Generate Test Agent Certificate
echo "Generating Test Agent Certificate..."
openssl genrsa -out agent.key 2048

cat > agent.cnf << EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
req_extensions = v3_req

[dn]
C = US
ST = State
L = City
O = Antigravity EDR
OU = Agent
CN = test-agent-001

[v3_req]
basicConstraints = CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = agent-test-agent-001.edr.local
URI.1 = urn:edr:agent:test-agent-001
EOF

openssl req -new -key agent.key -out agent.csr -config agent.cnf
openssl x509 -req -in agent.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out agent.crt -days 365 -sha256 -extensions v3_req -extfile agent.cnf

# 4. Generate JWT Keys
echo "Generating JWT Keys..."
openssl genrsa -out jwt_private.pem 2048
openssl rsa -in jwt_private.pem -pubout -out jwt_public.pem

# Clean up temporary files
rm -f *.csr *.cnf *.srl

echo ""
echo "Certificate generation complete!"
echo ""
echo "Generated files in $CERTS_DIR:"
ls -la "$CERTS_DIR"
echo ""
echo "WARNING: These are development certificates only!"
echo "DO NOT USE IN PRODUCTION!"
