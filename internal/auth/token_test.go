package auth_test

import (
	"testing"
	"time"

	"github.com/ruptor-dev/cli/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_ValidToken(t *testing.T) {
	k := loadTestKey(t)
	raw := signTestJWT(t, k, testClaims{
		TenantID:  "ten_abc",
		UserEmail: "user@example.com",
		Plan:      "starter",
		TokenID:   "tok_1",
		TokenType: auth.TokenTypePersonal,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour).Unix(),
	})

	tok, err := auth.Parse(raw, k.public)
	require.NoError(t, err)
	assert.Equal(t, "ten_abc", tok.Claims.TenantID)
	assert.Equal(t, "user@example.com", tok.Claims.UserEmail)
	assert.Equal(t, auth.TokenTypePersonal, tok.Claims.TokenType)
	assert.False(t, tok.Expired())
}

func TestParse_ExpiredToken(t *testing.T) {
	k := loadTestKey(t)
	raw := signTestJWT(t, k, testClaims{
		TenantID:  "ten_abc",
		UserEmail: "u@e.com",
		ExpiresAt: time.Now().Add(-time.Hour).Unix(),
	})

	_, err := auth.Parse(raw, k.public)
	require.ErrorIs(t, err, auth.ErrTokenExpired)
}

func TestParse_InvalidSignature(t *testing.T) {
	k := loadTestKey(t)
	raw := signTestJWT(t, k, testClaims{ExpiresAt: time.Now().Add(time.Hour).Unix()})

	// Flip one character in the signature segment so verification fails.
	flipped := raw[:len(raw)-2] + "AA"
	_, err := auth.Parse(flipped, k.public)
	require.Error(t, err)
	assert.NotErrorIs(t, err, auth.ErrTokenExpired)
}

func TestParse_MalformedPublicKey(t *testing.T) {
	k := loadTestKey(t)
	raw := signTestJWT(t, k, testClaims{ExpiresAt: time.Now().Add(time.Hour).Unix()})

	_, err := auth.Parse(raw, []byte("not a PEM"))
	require.Error(t, err)
}

func TestToken_NeedsRefresh(t *testing.T) {
	k := loadTestKey(t)

	tests := []struct {
		name    string
		expires time.Duration
		typ     auth.TokenType
		want    bool
	}{
		{"inside refresh window", 3 * 24 * time.Hour, auth.TokenTypePersonal, true},
		{"outside refresh window", 30 * 24 * time.Hour, auth.TokenTypePersonal, false},
		{"ci tokens never refresh", 3 * 24 * time.Hour, auth.TokenTypeCI, false},
		{"expired does not refresh", -time.Hour, auth.TokenTypePersonal, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := time.Now().Add(tt.expires).Unix()
			raw := signTestJWT(t, k, testClaims{
				TokenType: tt.typ,
				ExpiresAt: exp,
			})

			tok, err := auth.Parse(raw, k.public)
			if tt.expires < 0 {
				require.ErrorIs(t, err, auth.ErrTokenExpired)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, tok.NeedsRefresh())
		})
	}
}

func TestToken_MaskedSuffix(t *testing.T) {
	k := loadTestKey(t)
	raw := signTestJWT(t, k, testClaims{ExpiresAt: time.Now().Add(time.Hour).Unix()})
	tok, err := auth.Parse(raw, k.public)
	require.NoError(t, err)

	masked := tok.MaskedSuffix()
	// "…" is a 3-byte rune; expect ellipsis prefix + last 4 chars.
	assert.Equal(t, "…"+raw[len(raw)-4:], masked)
}
