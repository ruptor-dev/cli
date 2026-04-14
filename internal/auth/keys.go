package auth

import _ "embed"

// PublicKeyPEM is the RSA public key the CLI uses to verify JWTs the
// platform issues. It is embedded at build time — rotation ships via
// a binary release per security.md.
//
// TODO(v1-launch): replace the current dev keypair (generated locally,
// committed at internal/auth/keys/public_key.pem and
// internal/auth/testdata/jwt_private_key.pem) with the production key
// when the platform ships. The private key half never lands in this
// repo; it lives in platform-side secrets.
//
//go:embed keys/public_key.pem
var PublicKeyPEM []byte
