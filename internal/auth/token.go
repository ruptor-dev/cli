package auth

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenType distinguishes between interactive personal tokens (90-day
// TTL, silent refresh) and long-lived CI tokens (no autorotate).
type TokenType string

const (
	TokenTypePersonal TokenType = "personal"
	TokenTypeCI       TokenType = "ci"
)

// refreshWindow is the CLAUDE.md-mandated silent-refresh trigger: when
// less than this remains until ExpiresAt, the next operation kicks a
// background refresh.
const refreshWindow = 7 * 24 * time.Hour

// Claims is the JWT payload shape the platform issues.
type Claims struct {
	TenantID  string    `json:"tenant_id"`
	UserEmail string    `json:"user_email"`
	Plan      string    `json:"plan"`
	TokenID   string    `json:"token_id"`
	TokenType TokenType `json:"token_type"`
	ExpiresAt int64     `json:"expires_at"`
	jwt.RegisteredClaims
}

// Token is the local representation of a parsed, validated JWT.
type Token struct {
	Raw    string
	Claims Claims
}

// Parse validates the JWT signature and decodes its claims. Returns
// a wrapped ErrTokenExpired if the token has expired according to
// its own ExpiresAt field — callers differentiate so the UI can
// suggest `ruptor auth login`.
func Parse(raw string, publicKeyPEM []byte) (*Token, error) {
	pub, err := parseRSAPublicKey(publicKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("auth: public key: %w", err)
	}

	var claims Claims
	parsed, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method: %v", t.Header["alg"])
		}
		return pub, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("auth: parse: %w", err)
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("auth: invalid token")
	}
	if claims.ExpiresAt > 0 && time.Now().Unix() >= claims.ExpiresAt {
		return nil, ErrTokenExpired
	}
	return &Token{Raw: raw, Claims: claims}, nil
}

// Expired is true when the token's own ExpiresAt is in the past.
func (t *Token) Expired() bool {
	if t == nil || t.Claims.ExpiresAt == 0 {
		return false
	}
	return time.Now().Unix() >= t.Claims.ExpiresAt
}

// NeedsRefresh reports whether the token is still valid but within
// the silent-refresh window. CI tokens are never autorotated.
func (t *Token) NeedsRefresh() bool {
	if t == nil || t.Claims.TokenType == TokenTypeCI {
		return false
	}
	if t.Claims.ExpiresAt == 0 {
		return false
	}
	remaining := time.Until(time.Unix(t.Claims.ExpiresAt, 0))
	return remaining > 0 && remaining < refreshWindow
}

// MaskedSuffix returns the last 4 characters of the raw token — the
// only representation the UI + logs are allowed to show (per
// security.md).
func (t *Token) MaskedSuffix() string {
	if t == nil || len(t.Raw) < 4 {
		return "…"
	}
	return "…" + t.Raw[len(t.Raw)-4:]
}

func parseRSAPublicKey(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("pem: no data")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("pem: not an RSA public key")
	}
	return pub, nil
}
