package icloud

import (
	"bytes"
	"context"
	"crypto/rand"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The server side of SRP-6a must derive the same session proof as the client.
func TestSRPMatchesServerProof(t *testing.T) {
	for i := 0; i < 20; i++ {
		salt := make([]byte, 16)
		rand.Read(salt)
		if i == 0 {
			salt[0] = 0 // leading zero must be stripped like python srp
		}
		key, err := passwordKey("hunter2", "s2k_fo", salt, 1000)
		if err != nil {
			t.Fatal(err)
		}
		c, err := newSRPClient()
		if err != nil {
			t.Fatal(err)
		}
		s := new(big.Int).SetBytes(salt).Bytes()
		x := new(big.Int).SetBytes(h256(s, new(big.Int).SetBytes(h256([]byte(":"), key)).Bytes()))
		v := new(big.Int).Exp(srpG, x, srpN)
		bb := make([]byte, 32)
		rand.Read(bb)
		b := new(big.Int).SetBytes(bb)
		k := new(big.Int).SetBytes(h256(srpN.Bytes(), pad(srpG.Bytes())))
		B := new(big.Int).Add(new(big.Int).Mul(k, v), new(big.Int).Exp(srpG, b, srpN))
		B.Mod(B, srpN)

		m1, m2, err := c.process("user@example.com", key, salt, B.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		u := new(big.Int).SetBytes(h256(pad(c.A.Bytes()), pad(B.Bytes())))
		S := new(big.Int).Exp(new(big.Int).Mul(c.A, new(big.Int).Exp(v, u, srpN)), b, srpN)
		K := h256(S.Bytes())
		hn, hg := h256(srpN.Bytes()), h256(pad(srpG.Bytes()))
		for j := range hn {
			hn[j] ^= hg[j]
		}
		want := h256(hn, h256([]byte("user@example.com")), s, c.A.Bytes(), B.Bytes(), K)
		if !bytes.Equal(m1, want) || !bytes.Equal(m2, h256(c.A.Bytes(), want, K)) {
			t.Fatalf("round %d: client proof does not match server", i)
		}
	}
}

func TestParseHMEHeader(t *testing.T) {
	h := ParseHMEHeader("p=Hide My Email <Example_Alias.1a@icloud.com>; d=www.example.com;\r\n f=relay@example.org; r=to; s=Sender@Shop.com")
	if h.Alias != "example_alias.1a@icloud.com" || h.ForwardTo != "relay@example.org" || h.Sender != "sender@shop.com" {
		t.Fatalf("unexpected header: %+v", h)
	}
	if h := ParseHMEHeader("p=;f=nobody"); h.Alias != "" || h.ForwardTo != "" {
		t.Fatalf("invalid values accepted: %+v", h)
	}
}

func TestParseCookies(t *testing.T) {
	for _, in := range []string{
		`{"X-APPLE-WEBAUTH-TOKEN":"\"v=2:t=x\"","X-APPLE-WEBAUTH-USER":"\"v=1\""}`,
		`[{"name":"X-APPLE-WEBAUTH-TOKEN","value":"\"v=2:t=x\""}]`,
		`Cookie: X-APPLE-WEBAUTH-TOKEN="v=2:t=x"; other=1`,
	} {
		c, err := ParseCookies(in)
		if err != nil || c["X-APPLE-WEBAUTH-TOKEN"] != `"v=2:t=x"` {
			t.Fatalf("parse %q: %v %v", in, c, err)
		}
	}
	if _, err := ParseCookies(`{"a":"b"}`); err == nil {
		t.Fatal("cookies without session token must be rejected")
	}
}

func TestClientCallsAndCookieRefresh(t *testing.T) {
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/hme/list":
			gotCookie = r.Header.Get("Cookie")
			w.Header().Add("Set-Cookie", `X-APPLE-WEBAUTH-TOKEN="v=2:t=new"; Domain=.icloud.com; Path=/; Secure`)
			w.Header().Add("Set-Cookie", `GONE=; Expires=Thu, 01-Jan-1970 00:00:01 GMT`)
			w.Write([]byte(`{"success":true,"result":{"selectedForwardTo":"relay@example.org","forwardToEmails":["a@icloud.com","relay@example.org"],
				"hmeEmails":[{"hme":"One.Two@icloud.com","anonymousId":"x1","label":"L","isActive":true,"createTimestamp":1700000000000}]}}`))
		case "/v1/hme/generate":
			w.Write([]byte(`{"success":false,"error":{"errorCode":"-41015","errorMessage":"You have reached the limit of addresses you can create right now."}}`))
		case "/v1/hme/deactivate":
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()
	c, err := NewClient(map[string]string{"X-APPLE-WEBAUTH-TOKEN": `"v=2:t=old"`, "GONE": "1"}, "icloud.com", "")
	if err != nil {
		t.Fatal(err)
	}
	c.ServiceURL, c.validated = srv.URL, true
	ctx := context.Background()
	l, err := c.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotCookie, `X-APPLE-WEBAUTH-TOKEN="v=2:t=old"`) {
		t.Fatalf("cookie not sent verbatim: %q", gotCookie)
	}
	if len(l.Aliases) != 1 || l.Aliases[0].Email != "one.two@icloud.com" || !l.Aliases[0].Active || l.ForwardTo != "relay@example.org" || len(l.ForwardOptions) != 2 {
		t.Fatalf("unexpected list: %+v", l)
	}
	if ck := c.Cookies(); ck["X-APPLE-WEBAUTH-TOKEN"] != `"v=2:t=new"` || ck["GONE"] != "" {
		t.Fatalf("cookies not refreshed: %v", ck)
	}
	if _, err := c.Create(ctx, "x", ""); !IsRateLimited(err) {
		t.Fatalf("want rate limit, got %v", err)
	}
	if err := c.Deactivate(ctx, "x1"); err != ErrUnauthorized {
		t.Fatalf("want unauthorized, got %v", err)
	}
}
