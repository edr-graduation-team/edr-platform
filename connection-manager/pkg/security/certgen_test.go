package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCertCoversIPs_RejectsFutureNotBeforeBeyondTolerance(t *testing.T) {
	t.Parallel()

	certPath := filepath.Join(t.TempDir(), "server.crt")
	requiredIP := net.ParseIP("127.0.0.1")
	require.NotNil(t, requiredIP)

	err := writeSelfSignedServerCert(
		certPath,
		time.Now().Add(2*time.Hour), // far in future
		time.Now().AddDate(1, 0, 0),
		[]net.IP{requiredIP},
	)
	require.NoError(t, err)

	ok := certCoversIPs(certPath, []net.IP{requiredIP}, logrus.New())
	assert.False(t, ok)
}

func TestCertCoversIPs_AllowsSmallFutureNotBeforeWithinTolerance(t *testing.T) {
	t.Parallel()

	certPath := filepath.Join(t.TempDir(), "server.crt")
	requiredIP := net.ParseIP("127.0.0.1")
	require.NotNil(t, requiredIP)

	err := writeSelfSignedServerCert(
		certPath,
		time.Now().Add(5*time.Minute), // within tolerance
		time.Now().AddDate(1, 0, 0),
		[]net.IP{requiredIP},
	)
	require.NoError(t, err)

	ok := certCoversIPs(certPath, []net.IP{requiredIP}, logrus.New())
	assert.True(t, ok)
}

func TestGetCertClockSkewTolerance_Default(t *testing.T) {
	t.Setenv(certClockSkewToleranceEnv, "")
	assert.Equal(t, defaultCertClockSkewTolerance, getCertClockSkewTolerance(logrus.New()))
}

func TestGetCertClockSkewTolerance_InvalidFallsBackToDefault(t *testing.T) {
	t.Setenv(certClockSkewToleranceEnv, "invalid-duration")
	assert.Equal(t, defaultCertClockSkewTolerance, getCertClockSkewTolerance(logrus.New()))
}

func TestGetCertClockSkewTolerance_ClampsToMax(t *testing.T) {
	t.Setenv(certClockSkewToleranceEnv, "2h")
	assert.Equal(t, maxCertClockSkewTolerance, getCertClockSkewTolerance(logrus.New()))
}

func writeSelfSignedServerCert(certPath string, notBefore, notAfter time.Time, ips []net.IP) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "edr-connection-manager",
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		DNSNames:    []string{"localhost", "edr-connection-manager"},
		IPAddresses: ips,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return os.WriteFile(certPath, certPEM, 0600)
}
