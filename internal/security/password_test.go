package security

import (
	"strings"
	"testing"
)

func TestPasswordHashUsesArgon2idAndRandomSalt(t *testing.T) {
	password := "regional-operator-password"
	first, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash first password: %v", err)
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash second password: %v", err)
	}
	if !strings.HasPrefix(first, "argon2id$v=19$") {
		t.Fatalf("unexpected hash format: %s", first)
	}
	if first == second {
		t.Fatal("password hashes should use independent random salts")
	}
	if !VerifyPassword(first, password) || !VerifyPassword(second, password) {
		t.Fatal("correct password did not verify")
	}
	if VerifyPassword(first, "different-password") {
		t.Fatal("wrong password verified")
	}
}

func TestPasswordHashRejectsInvalidInputsAndFormats(t *testing.T) {
	for _, password := range []string{"short", strings.Repeat("x", 257)} {
		if _, err := HashPassword(password); err == nil {
			t.Errorf("password length %d should fail", len(password))
		}
	}
	invalid := []string{
		"",
		"sha256$120000$salt$digest",
		"argon2id$v=18$m=65536,t=3,p=2$salt$digest",
		"argon2id$v=19$m=1,t=3,p=2$bad$bad",
		"argon2id$v=19$m=65536,t=1,p=2$bad$bad",
		"argon2id$v=19$m=65536,t=3,p=1$bad$bad",
	}
	for _, encoded := range invalid {
		if VerifyPassword(encoded, "regional-operator-password") {
			t.Errorf("invalid hash verified: %q", encoded)
		}
	}
}
