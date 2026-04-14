package auth

import _ "embed"

// PublicKeyPEM is the RSA public key the CLI uses to verify JWTs the
// platform issues. It is embedded at build time — rotation ships via
// a binary release per security.md.
//
//go:embed keys/public_key.pem
var PublicKeyPEM []byte
