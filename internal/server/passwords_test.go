package server

import "testing"

func TestUserPasswordEncryptionRoundTrip(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	ciphertext, err := a.encryptUserPassword("plain-secret")
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == "plain-secret" || ciphertext == "" {
		t.Fatal("password was not encrypted")
	}
	plain, err := a.decryptUserPassword(ciphertext)
	if err != nil || plain != "plain-secret" {
		t.Fatalf("round trip = %q, %v", plain, err)
	}
}
