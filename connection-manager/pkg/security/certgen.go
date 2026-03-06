// Package security provides the Auto-Certificate Bootstrapper for dynamic
// server certificate generation based on the host's current network environment.
package security

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

// EnsureServerCert checks whether the server certificate at serverCertPath
// contains SANs for ALL of the host's current IP addresses. If the cert is
// missing, expired, or any IP has changed, it generates a new server
// certificate signed by the CA, writes it to disk, and returns true.
//
// This solves the mTLS SAN mismatch problem (e.g. "x509: certificate is valid
// for 127.0.0.1, not 192.168.129.1") by dynamically discovering IPs at startup.
//
// Returns (regenerated bool, err error).
func EnsureServerCert(
	caCertPath, caKeyPath string,
	serverCertPath, serverKeyPath string,
	logger *logrus.Logger,
) (bool, error) {
	// 1. Discover all host IPs
	hostIPs, err := discoverHostIPs()
	if err != nil {
		return false, fmt.Errorf("failed to discover host IPs: %w", err)
	}

	logger.WithField("ips", ipsToStrings(hostIPs)).Info("Auto-Cert Bootstrapper: discovered host IPs")

	// 2. Check existing certificate
	if certCoversIPs(serverCertPath, hostIPs, logger) {
		logger.Info("Auto-Cert Bootstrapper: existing server certificate covers all host IPs — no regeneration needed")
		return false, nil
	}

	// 3. Load CA cert + key for signing
	caCert, caKey, err := loadCA(caCertPath, caKeyPath)
	if err != nil {
		return false, fmt.Errorf("auto-cert bootstrapper: failed to load CA: %w", err)
	}

	// 4. Generate new server cert + key
	logger.Info("Auto-Cert Bootstrapper: generating new server certificate with current host IPs...")
	if err := generateServerCert(caCert, caKey, serverCertPath, serverKeyPath, hostIPs); err != nil {
		return false, fmt.Errorf("auto-cert bootstrapper: failed to generate server cert: %w", err)
	}

	logger.WithField("ips", ipsToStrings(hostIPs)).Info("Auto-Cert Bootstrapper: server certificate regenerated successfully")
	return true, nil
}

// discoverHostIPs enumerates all active IP addresses (IPv4 + IPv6) on the
// current machine via net.InterfaceAddrs(). It always includes 127.0.0.1 and
// ::1 as baseline loopback addresses.
func discoverHostIPs() ([]net.IP, error) {
	// Start with loopback addresses (always present)
	ipSet := map[string]net.IP{
		"127.0.0.1": net.ParseIP("127.0.0.1"),
		"::1":       net.ParseIP("::1"),
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		default:
			continue
		}

		if ip == nil {
			continue
		}

		// Normalize and deduplicate
		ipStr := ip.String()
		if _, exists := ipSet[ipStr]; !exists {
			ipSet[ipStr] = ip
		}
	}

	ips := make([]net.IP, 0, len(ipSet))
	for _, ip := range ipSet {
		ips = append(ips, ip)
	}
	return ips, nil
}

// certCoversIPs checks if the certificate at certPath exists, is not expired,
// and contains ALL of the given IPs in its SANs.
func certCoversIPs(certPath string, requiredIPs []net.IP, logger *logrus.Logger) bool {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		logger.WithError(err).Info("Auto-Cert Bootstrapper: server certificate not found or unreadable — will generate")
		return false
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		logger.Warn("Auto-Cert Bootstrapper: server certificate is not valid PEM — will regenerate")
		return false
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		logger.WithError(err).Warn("Auto-Cert Bootstrapper: failed to parse server certificate — will regenerate")
		return false
	}

	// Check expiry (regenerate if expiring within 7 days)
	if time.Until(cert.NotAfter) < 7*24*time.Hour {
		logger.WithField("expires", cert.NotAfter).Warn("Auto-Cert Bootstrapper: server certificate expiring soon — will regenerate")
		return false
	}

	// Build a set of IPs that are in the cert's SANs
	certIPSet := make(map[string]bool, len(cert.IPAddresses))
	for _, ip := range cert.IPAddresses {
		certIPSet[ip.String()] = true
	}

	// Check that every required IP is covered
	for _, reqIP := range requiredIPs {
		if !certIPSet[reqIP.String()] {
			logger.WithFields(logrus.Fields{
				"missing_ip": reqIP.String(),
				"cert_ips":   ipsToStrings(cert.IPAddresses),
			}).Info("Auto-Cert Bootstrapper: host IP not in certificate SANs — will regenerate")
			return false
		}
	}

	return true
}

// loadCA reads the CA certificate and private key from disk.
// Returns crypto.Signer which is implemented by both *rsa.PrivateKey and *ecdsa.PrivateKey.
func loadCA(caCertPath, caKeyPath string) (*x509.Certificate, crypto.Signer, error) {
	// Load CA cert
	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read CA cert: %w", err)
	}
	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		return nil, nil, fmt.Errorf("CA cert is not valid PEM")
	}
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA cert: %w", err)
	}

	// Load CA key — supports RSA, ECDSA, and PKCS8 PEM formats
	caKeyPEM, err := os.ReadFile(caKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read CA key: %w", err)
	}
	keyBlock, _ := pem.Decode(caKeyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("CA key is not valid PEM")
	}

	var signer crypto.Signer
	switch keyBlock.Type {
	case "RSA PRIVATE KEY":
		rsaKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parse RSA private key: %w", err)
		}
		signer = rsaKey
	case "EC PRIVATE KEY":
		ecKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parse EC private key: %w", err)
		}
		signer = ecKey
	case "PRIVATE KEY":
		// PKCS#8 can wrap RSA, ECDSA, or Ed25519
		key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parse PKCS8 private key: %w", err)
		}
		var ok bool
		signer, ok = key.(crypto.Signer)
		if !ok {
			return nil, nil, fmt.Errorf("PKCS8 key does not implement crypto.Signer (got %T)", key)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported CA key PEM type: %s", keyBlock.Type)
	}

	return caCert, signer, nil
}

// generateServerCert creates a new ECDSA P-256 server certificate signed by
// the CA, containing all provided IPs as SANs alongside "localhost".
func generateServerCert(
	caCert *x509.Certificate,
	caKey crypto.Signer,
	serverCertPath, serverKeyPath string,
	hostIPs []net.IP,
) error {
	// Generate server keypair (ECDSA P-256 — fast, modern, widely supported)
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate server key: %w", err)
	}

	// Serial number
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "edr-connection-manager",
			Organization: []string{"EDR Platform"},
		},
		NotBefore: now.Add(-5 * time.Minute), // Clock skew tolerance
		NotAfter:  now.AddDate(1, 0, 0),      // 1 year validity

		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},

		// SANs: localhost + all dynamically discovered IPs
		DNSNames:    []string{"localhost", "edr-connection-manager"},
		IPAddresses: hostIPs,

		// Leaf certificate — not a CA
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Sign with CA
	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("sign server cert: %w", err)
	}

	// Write server certificate PEM
	certFile, err := os.Create(serverCertPath)
	if err != nil {
		return fmt.Errorf("create cert file: %w", err)
	}
	defer certFile.Close()
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return fmt.Errorf("write cert PEM: %w", err)
	}

	// Write server private key PEM
	keyFile, err := os.OpenFile(serverKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create key file: %w", err)
	}
	defer keyFile.Close()
	keyBytes, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return fmt.Errorf("marshal server key: %w", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return fmt.Errorf("write key PEM: %w", err)
	}

	return nil
}

// ipsToStrings converts a slice of net.IP to string representations for logging.
func ipsToStrings(ips []net.IP) []string {
	strs := make([]string, len(ips))
	for i, ip := range ips {
		strs[i] = ip.String()
	}
	return strs
}
