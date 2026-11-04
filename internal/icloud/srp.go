package icloud

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
)

// RFC 5054 2048-bit group, as used by Apple's GrandSlam SRP-6a (SHA-256,
// rfc5054 padding, no username in x) — mirrors python `srp` semantics.
var (
	srpN, _ = new(big.Int).SetString(strings.Join(strings.Fields(`
AC6BDB41324A9A9BF166DE5E1389582FAF72B6651987EE07FC3192943DB56050A37329CBB4A099ED8193E0757767A13DD52312AB4B03310D
CD7F48A9DA04FD50E8083969EDB767B0CF6095179A163AB3661A05FBD5FAAAE82918A9962F0B93B855F97993EC975EEAA80D740ADBF4FF74
7359D041D5C33EA71D281E446B14773BCA97B43A23FB801676BD207A436C6481F1D2B9078717461A5B9D32E688F87748544523B524B0D57D
5EA77A2775D2ECFA032CFBDBF52FB3786160279004E57AE6AF874E7303CE53299CCC041C7BC308D82A5698F3A8D0C38271AE35F8E9DBFBB6
94B5C803D89F7AE435DE236D525F54759B65E372FCD68EF20FA7111F9E4AFF73`), ""), 16)
	srpG    = big.NewInt(2)
	srpSize = len(srpN.Bytes())
)

func h256(parts ...[]byte) []byte {
	h := sha256.New()
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}

func pad(b []byte) []byte {
	if len(b) >= srpSize {
		return b
	}
	return append(make([]byte, srpSize-len(b)), b...)
}

type srpClient struct {
	a, A *big.Int
}

func newSRPClient() (*srpClient, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	a := new(big.Int).SetBytes(buf)
	return &srpClient{a: a, A: new(big.Int).Exp(srpG, a, srpN)}, nil
}

// passwordKey derives the SRP password per Apple's s2k / s2k_fo protocols.
func passwordKey(password, protocol string, salt []byte, iterations int) ([]byte, error) {
	sum := sha256.Sum256([]byte(password))
	p := string(sum[:])
	if protocol == "s2k_fo" {
		p = hex.EncodeToString(sum[:])
	}
	return pbkdf2.Key(sha256.New, p, salt, iterations, 32)
}

// process returns M1 and M2 for the server challenge.
func (c *srpClient) process(username string, key, salt, bBytes []byte) (m1, m2 []byte, err error) {
	B := new(big.Int).SetBytes(bBytes)
	if new(big.Int).Mod(B, srpN).Sign() == 0 {
		return nil, nil, errors.New("invalid SRP challenge")
	}
	s := new(big.Int).SetBytes(salt).Bytes()
	u := new(big.Int).SetBytes(h256(pad(c.A.Bytes()), pad(B.Bytes())))
	if u.Sign() == 0 {
		return nil, nil, errors.New("invalid SRP challenge")
	}
	k := new(big.Int).SetBytes(h256(srpN.Bytes(), pad(srpG.Bytes())))
	x := new(big.Int).SetBytes(h256(s, new(big.Int).SetBytes(h256([]byte(":"), key)).Bytes()))

	base := new(big.Int).Sub(B, new(big.Int).Mul(k, new(big.Int).Exp(srpG, x, srpN)))
	base.Mod(base, srpN)
	exp := new(big.Int).Add(c.a, new(big.Int).Mul(u, x))
	S := new(big.Int).Exp(base, exp, srpN)
	K := h256(S.Bytes())

	hn, hg := h256(srpN.Bytes()), h256(pad(srpG.Bytes()))
	for i := range hn {
		hn[i] ^= hg[i]
	}
	m1 = h256(hn, h256([]byte(username)), s, c.A.Bytes(), B.Bytes(), K)
	m2 = h256(c.A.Bytes(), m1, K)
	return m1, m2, nil
}
