package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "" || hash == "correct horse battery staple" {
		t.Fatal("hash should be non-empty and not the plaintext")
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Error("correct password should verify")
	}
	if VerifyPassword("wrong password", hash) {
		t.Error("wrong password must not verify")
	}
	if VerifyPassword("correct horse battery staple", "not-a-valid-hash") {
		t.Error("malformed hash must not verify")
	}
}

func TestNewTokenIsUniqueAndHashes(t *testing.T) {
	tok1, h1, err := NewToken("pat_")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	tok2, _, _ := NewToken("pat_")
	if tok1 == tok2 {
		t.Error("tokens must be unique")
	}
	if tok1[:4] != "pat_" {
		t.Errorf("token should carry its prefix, got %q", tok1)
	}
	// HashToken must match the hash returned by NewToken and be deterministic.
	rehash := HashToken(tok1)
	if len(rehash) != len(h1) {
		t.Fatal("hash length mismatch")
	}
	for i := range h1 {
		if rehash[i] != h1[i] {
			t.Fatal("HashToken must reproduce NewToken's hash")
		}
	}
}
