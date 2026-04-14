package auth_test

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ruptor-dev/cli/internal/auth"
	"github.com/stretchr/testify/require"
)

// testKey holds the dev RSA keypair used to sign test JWTs. The
// public half is what the binary embeds (keys/public_key.pem); the
// private half lives only in testdata/.
type testKey struct {
	private *rsa.PrivateKey
	public  []byte
}

func loadTestKey(t *testing.T) *testKey {
	t.Helper()

	priv := readPEM(t, filepath.Join("testdata", "jwt_private_key.pem"))
	block, _ := pem.Decode(priv)
	require.NotNil(t, block, "could not decode PEM")

	// openssl writes PKCS#8 for genpkey, PKCS#1 for old `genrsa`. Try both.
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		k2, err2 := x509.ParsePKCS1PrivateKey(block.Bytes)
		require.NoError(t, err2)
		return &testKey{private: k2, public: auth.PublicKeyPEM}
	}
	rsaKey, ok := k.(*rsa.PrivateKey)
	require.True(t, ok, "test key is not RSA")
	return &testKey{private: rsaKey, public: auth.PublicKeyPEM}
}

func readPEM(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}

type testClaims struct {
	TenantID  string         `json:"tenant_id"`
	UserEmail string         `json:"user_email"`
	Plan      string         `json:"plan"`
	TokenID   string         `json:"token_id"`
	TokenType auth.TokenType `json:"token_type"`
	ExpiresAt int64          `json:"expires_at"`
	jwt.RegisteredClaims
}

// signTestJWT produces a signed JWT with the given claims + the dev
// private key. Sets the `exp` registered claim alongside the custom
// expires_at field so the jwt library's validator stays happy.
func signTestJWT(t *testing.T, k *testKey, claims testClaims) string {
	t.Helper()
	if claims.ExpiresAt > 0 {
		claims.RegisteredClaims.ExpiresAt = jwt.NewNumericDate(time.Unix(claims.ExpiresAt, 0))
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := tok.SignedString(k.private)
	require.NoError(t, err)
	return signed
}
