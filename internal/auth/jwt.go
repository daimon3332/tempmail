package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Signer struct {
	secret []byte
}

func New(secret string) *Signer { return &Signer{secret: []byte(secret)} }

// Sign produces an HS256 token whose payload is exactly the given claims,
// matching hono's Jwt.sign used upstream (no automatic exp/iat).
func (s *Signer) Sign(claims map[string]any) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims(claims)).SignedString(s.secret)
}

// Verify parses a token. Expiry is honoured only when an exp claim exists,
// matching upstream address tokens which never expire.
func (s *Signer) Verify(token string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	}, jwt.WithoutClaimsValidation())
	if err != nil {
		return nil, err
	}
	if exp, ok := claims["exp"]; ok {
		if f, ok := exp.(float64); ok && int64(f) < time.Now().Unix() {
			return nil, errors.New("token expired")
		}
	}
	return claims, nil
}

func (s *Signer) AddressToken(address string, addressID int64) (string, error) {
	return s.Sign(map[string]any{"address": address, "address_id": addressID})
}

func (s *Signer) UserToken(email string, userID int64) (string, error) {
	now := time.Now().Unix()
	return s.Sign(map[string]any{
		"user_email": email,
		"user_id":    userID,
		"exp":        now + 30*24*3600,
		"iat":        now,
	})
}

func (s *Signer) AccessToken(email string, userID int64, role string) (string, error) {
	now := time.Now().Unix()
	return s.Sign(map[string]any{
		"user_email": email,
		"user_id":    userID,
		"user_role":  role,
		"iat":        now,
		"exp":        now + 3600,
	})
}

func ClaimInt(c jwt.MapClaims, key string) int64 {
	switch v := c[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case string:
		var n int64
		for _, ch := range v {
			if ch < '0' || ch > '9' {
				return 0
			}
			n = n*10 + int64(ch-'0')
		}
		return n
	}
	return 0
}

func ClaimStr(c jwt.MapClaims, key string) string {
	s, _ := c[key].(string)
	return s
}
