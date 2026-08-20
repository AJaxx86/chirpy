package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	const password = "correct horse battery staple"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if hash == password {
		t.Error("HashPassword() returned the plaintext password")
	}

	matches, err := CheckPasswordHash(password, hash)
	if err != nil {
		t.Fatalf("CheckPasswordHash() error = %v", err)
	}
	if !matches {
		t.Error("CheckPasswordHash() = false, want true for the original password")
	}

	matches, err = CheckPasswordHash("incorrect password", hash)
	if err != nil {
		t.Fatalf("CheckPasswordHash() error = %v", err)
	}
	if matches {
		t.Error("CheckPasswordHash() = true, want false for an incorrect password")
	}
}
