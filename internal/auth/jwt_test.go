package auth

import "testing"

func TestAddressTokenRoundTrip(t *testing.T) {
	s := New("secret")
	tok, err := s.AddressToken("a@b.org", 42)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Verify(tok)
	if err != nil || ClaimStr(c, "address") != "a@b.org" || ClaimInt(c, "address_id") != 42 {
		t.Fatalf("%v %v", err, c)
	}
	if _, err := New("other").Verify(tok); err == nil {
		t.Fatal("wrong secret accepted")
	}
}

// Token produced by hono Jwt.sign({address:"x@y.z",address_id:1}, "secret", "HS256").
func TestUpstreamToken(t *testing.T) {
	const tok = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZGRyZXNzIjoieEB5LnoiLCJhZGRyZXNzX2lkIjoxfQ.SajSbx0N6S0pi2N3DdjMtNzby-yYjjWqb4oKVg0rZMw"
	c, err := New("secret").Verify(tok)
	if err != nil || ClaimStr(c, "address") != "x@y.z" || ClaimInt(c, "address_id") != 1 {
		t.Fatalf("%v %v", err, c)
	}
}

func TestExpiredToken(t *testing.T) {
	s := New("k")
	tok, _ := s.Sign(map[string]any{"user_id": 1, "exp": 1})
	if _, err := s.Verify(tok); err == nil {
		t.Fatal("expired token accepted")
	}
}
