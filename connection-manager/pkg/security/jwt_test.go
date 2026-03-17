// Package security provides unit tests for security components.
package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTManager_GenerateAndValidate(t *testing.T) {
	// Create temporary key files
	tmpDir := t.TempDir()
	privateKeyPath := filepath.Join(tmpDir, "private.pem")
	publicKeyPath := filepath.Join(tmpDir, "public.pem")

	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Write private key
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	err = os.WriteFile(privateKeyPath, privateKeyPEM, 0600)
	require.NoError(t, err)

	// Write public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})
	err = os.WriteFile(publicKeyPath, publicKeyPEM, 0644)
	require.NoError(t, err)

	// Create JWT manager
	jwtManager, err := NewJWTManager(
		privateKeyPath,
		publicKeyPath,
		"test-issuer",
		"test-audience",
		15*time.Minute,
		24*time.Hour,
	)
	require.NoError(t, err)

	// Test token generation
	t.Run("GenerateTokenPair", func(t *testing.T) {
		tokenPair, err := jwtManager.GenerateTokenPair("agent-123", "testuser", []string{"data_collector"})
		require.NoError(t, err)
		assert.NotEmpty(t, tokenPair.AccessToken)
		assert.NotEmpty(t, tokenPair.RefreshToken)
		assert.True(t, tokenPair.AccessExp.After(time.Now()))
		assert.True(t, tokenPair.RefreshExp.After(time.Now()))
	})

	// Test token validation
	t.Run("ValidateToken", func(t *testing.T) {
		tokenPair, err := jwtManager.GenerateTokenPair("agent-456", "admin-user", []string{"admin"})
		require.NoError(t, err)

		claims, err := jwtManager.ValidateToken(tokenPair.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, "agent-456", claims.AgentID)
		assert.Equal(t, "access", claims.Type)
		assert.Contains(t, claims.Roles, "admin")
	})

	// Test invalid token
	t.Run("ValidateInvalidToken", func(t *testing.T) {
		_, err := jwtManager.ValidateToken("invalid-token")
		assert.Error(t, err)
	})

	// Test refresh token
	t.Run("RefreshAccessToken", func(t *testing.T) {
		tokenPair, err := jwtManager.GenerateTokenPair("agent-789", "testuser", []string{"data_collector"})
		require.NoError(t, err)

		newAccessToken, newExp, err := jwtManager.RefreshAccessToken(tokenPair.RefreshToken)
		require.NoError(t, err)
		assert.NotEmpty(t, newAccessToken)
		assert.NotEqual(t, tokenPair.AccessToken, newAccessToken)
		assert.True(t, newExp.After(time.Now()))

		// Validate new access token
		claims, err := jwtManager.ValidateToken(newAccessToken)
		require.NoError(t, err)
		assert.Equal(t, "agent-789", claims.AgentID)
	})

	// Test access token cannot be used as refresh
	t.Run("RefreshWithAccessTokenFails", func(t *testing.T) {
		tokenPair, err := jwtManager.GenerateTokenPair("agent-aaa", "", []string{})
		require.NoError(t, err)

		_, _, err = jwtManager.RefreshAccessToken(tokenPair.AccessToken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a refresh token")
	})

	// Test GetTokenID
	t.Run("GetTokenID", func(t *testing.T) {
		tokenPair, err := jwtManager.GenerateTokenPair("agent-bbb", "", []string{})
		require.NoError(t, err)

		jti, err := jwtManager.GetTokenID(tokenPair.AccessToken)
		require.NoError(t, err)
		assert.NotEmpty(t, jti)
	})
}
