package oidc

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
)

// jwkSet is the JSON Web Key Set response format (RFC 7517).
type jwkSet struct {
	Keys []jwk `json:"keys"`
}

// jwk is a single JSON Web Key entry for an EC public key.
type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
}

// HandleJWKS serves the JSON Web Key Set at GET /jwks.
//
// Relying parties fetch this to obtain the public keys needed to verify
// ID Token signatures. The response is cacheable.
func (p *Provider) HandleJWKS(w http.ResponseWriter, r *http.Request) {
	pub := p.keyManager.PublicKey()

	set := jwkSet{
		Keys: []jwk{
			ecPublicKeyToJWK(pub, p.keyManager.KeyID()),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(set)
}

// ecPublicKeyToJWK converts an ECDSA public key to JWK format.
//
// EC P-256 coordinates are 32 bytes. We zero-pad to ensure consistent
// encoding regardless of the big.Int byte representation.
func ecPublicKeyToJWK(pub *ecdsa.PublicKey, kid string) jwk {
	// P-256 coordinates are 32 bytes (256 bits / 8).
	const coordLen = 32

	return jwk{
		Kty: "EC",
		Crv: "P-256",
		X:   base64.RawURLEncoding.EncodeToString(padToN(pub.X.Bytes(), coordLen)),
		Y:   base64.RawURLEncoding.EncodeToString(padToN(pub.Y.Bytes(), coordLen)),
		Kid: kid,
		Use: "sig",
		Alg: "ES256",
	}
}

// padToN left-pads b with zeroes to length n. If b is already >= n bytes,
// it is returned as-is.
func padToN(b []byte, n int) []byte {
	if len(b) >= n {
		return b
	}
	padded := make([]byte, n)
	copy(padded[n-len(b):], b)
	return padded
}
