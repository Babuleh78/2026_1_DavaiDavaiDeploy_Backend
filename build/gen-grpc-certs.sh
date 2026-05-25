set -e

export MSYS_NO_PATHCONV=1

CERT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/certs"
mkdir -p "$CERT_DIR"
cd "$CERT_DIR"

openssl genrsa -out ca.key 4096
openssl req -x509 -new -nodes -key ca.key -sha256 -days 3650 \
  -subj "/CN=DDDance Internal CA" -out ca.crt

openssl genrsa -out auth-server.key 4096
openssl req -new -key auth-server.key -subj "/CN=auth" -out auth-server.csr

cat > auth-server.ext <<'EOF'
subjectAltName = DNS:auth, DNS:localhost, IP:127.0.0.1
extendedKeyUsage = serverAuth
EOF
openssl x509 -req -in auth-server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -days 3650 -sha256 -extfile auth-server.ext -out auth-server.crt

chmod 644 ca.crt auth-server.crt auth-server.key
rm -f auth-server.csr auth-server.ext ca.srl

echo "Сертификаты сгенерированы в: $CERT_DIR"
ls -l "$CERT_DIR"
