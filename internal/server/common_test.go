package server

import "testing"

func TestFindAllowedDomain(t *testing.T) {
	allow := []string{"a.org", "b.xyz"}
	cases := []struct {
		in   string
		sub  bool
		want string
	}{
		{"a.org", false, "a.org"}, {"A.ORG", false, "a.org"}, {"x.a.org", false, ""}, {"x.a.org", true, "x.a.org"},
		{"c.com", true, ""}, {"-bad.a.org", true, ""}, {"", true, ""},
	}
	for _, c := range cases {
		if got := findAllowedDomain(c.in, allow, c.sub); got != c.want {
			t.Errorf("%q sub=%v: got %q want %q", c.in, c.sub, got, c.want)
		}
	}
}

func TestSHA256Hex(t *testing.T) {
	if sha256Hex("abc") != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatal("sha256 mismatch")
	}
}
