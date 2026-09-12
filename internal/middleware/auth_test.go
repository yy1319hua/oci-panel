package middleware

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestOriginAllowed(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		allowed []string
		want    bool
	}{
		{"empty allowlist denies all", "https://evil.com", nil, false},
		{"exact match", "https://panel.example.com", []string{"https://panel.example.com"}, true},
		{"case-insensitive match", "https://Panel.Example.COM", []string{"https://panel.example.com"}, true},
		{"origin not in list", "https://evil.com", []string{"https://panel.example.com"}, false},
		{"wildcard allows any", "https://anything.com", []string{"*"}, true},
		{"surrounding whitespace trimmed", "https://panel.example.com", []string{"  https://panel.example.com  "}, true},
		{"one of several", "https://b.com", []string{"https://a.com", "https://b.com"}, true},
		{"suffix attack denied", "https://panel.example.com.evil.com", []string{"https://panel.example.com"}, false},
		{"prefix attack denied", "https://evil-panel.example.com", []string{"https://panel.example.com"}, false},
		{"scheme mismatch denied", "http://panel.example.com", []string{"https://panel.example.com"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := originAllowed(tt.origin, tt.allowed); got != tt.want {
				t.Errorf("originAllowed(%q, %v) = %v, want %v", tt.origin, tt.allowed, got, tt.want)
			}
		})
	}
}

func TestGenerateRandomSecret(t *testing.T) {
	s1, err := generateRandomSecret(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s1) != 64 { // 32 字节 -> 64 个十六进制字符
		t.Errorf("hex of 32 bytes should be 64 chars, got %d", len(s1))
	}
	s2, _ := generateRandomSecret(32)
	if s1 == s2 {
		t.Error("two generated secrets must differ (not deterministic)")
	}
}

func TestVerifyPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt setup failed: %v", err)
	}
	tests := []struct {
		name     string
		stored   string
		provided string
		want     bool
	}{
		{"plaintext match", "mypw", "mypw", true},
		{"plaintext mismatch", "mypw", "wrong", false},
		{"empty stored denies", "", "anything", false},
		{"bcrypt match", string(hash), "s3cret", true},
		{"bcrypt mismatch", string(hash), "wrong", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifyPassword(tt.stored, tt.provided); got != tt.want {
				t.Errorf("VerifyPassword(stored=%q, provided=%q) = %v, want %v", tt.stored, tt.provided, got, tt.want)
			}
		})
	}
}

func TestIsBcryptHash(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("x"), bcrypt.MinCost)
	if !IsBcryptHash(string(hash)) {
		t.Error("a real bcrypt hash should be detected")
	}
	if IsBcryptHash("plaintextpassword") {
		t.Error("plaintext should not be detected as bcrypt")
	}
}

func TestSecureCompareString(t *testing.T) {
	if !SecureCompareString("abc", "abc") {
		t.Error("equal strings should compare true")
	}
	if SecureCompareString("abc", "abd") {
		t.Error("different strings should compare false")
	}
	if SecureCompareString("abc", "abcd") {
		t.Error("different-length strings should compare false")
	}
}
