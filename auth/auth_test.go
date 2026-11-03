package auth

import (
	"testing"
	"time"

	"github.com/funxdata/vango/errs"
)

// testSecret 是长度符合 MinSecretLength 要求的测试密钥。
var testSecret = []byte("0123456789abcdef0123456789abcdef")

func TestSignAndParse(t *testing.T) {
	signer, err := NewSigner(testSecret, WithIssuer("vango"))
	if err != nil {
		t.Fatalf("NewSigner returned error: %v", err)
	}

	token, err := signer.Sign(Claims{Subject: "42", Roles: []string{"admin"}}, time.Hour)
	if err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}

	claims, err := signer.Parse(token)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if claims.Subject != "42" {
		t.Fatalf("Subject = %q", claims.Subject)
	}
	if claims.Issuer != "vango" {
		t.Fatalf("Issuer = %q", claims.Issuer)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "admin" {
		t.Fatalf("Roles = %v", claims.Roles)
	}
	if claims.ExpiresAt == 0 {
		t.Fatal("ExpiresAt should be set")
	}
}

func TestParseRejectsTamperedToken(t *testing.T) {
	signer, err := NewSigner(testSecret)
	if err != nil {
		t.Fatalf("NewSigner returned error: %v", err)
	}

	token, err := signer.Sign(Claims{Subject: "42"}, time.Hour)
	if err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}

	tampered := token[:len(token)-2] + "xx"
	if _, err := signer.Parse(tampered); err == nil {
		t.Fatal("expected tampered token to be rejected")
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	now := time.Now()
	signer, err := NewSigner(testSecret, WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewSigner returned error: %v", err)
	}

	token, err := signer.Sign(Claims{Subject: "42"}, time.Minute)
	if err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}

	expired, err := NewSigner(testSecret, WithClock(func() time.Time { return now.Add(2 * time.Minute) }))
	if err != nil {
		t.Fatalf("NewSigner returned error: %v", err)
	}

	if _, err := expired.Parse(token); !isUnauthorized(err) {
		t.Fatalf("expected unauthorized error, got %v", err)
	}
}

func TestNewSignerRequiresStrongSecret(t *testing.T) {
	if _, err := NewSigner(nil); err == nil {
		t.Fatal("expected error for empty secret")
	}
	if _, err := NewSigner([]byte("short-secret")); err == nil {
		t.Fatalf("expected error for secret shorter than %d bytes", MinSecretLength)
	}
}

func TestParseRejectsWrongIssuerAndAudience(t *testing.T) {
	signer, err := NewSigner(testSecret, WithIssuer("vango"), WithAudience("api"))
	if err != nil {
		t.Fatalf("NewSigner returned error: %v", err)
	}
	token, err := signer.Sign(Claims{Subject: "42"}, time.Hour)
	if err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}

	otherIssuer, err := NewSigner(testSecret, WithIssuer("other"), WithAudience("api"))
	if err != nil {
		t.Fatalf("NewSigner returned error: %v", err)
	}
	if _, err := otherIssuer.Parse(token); err == nil {
		t.Fatal("token with a different issuer should be rejected")
	}

	otherAudience, err := NewSigner(testSecret, WithIssuer("vango"), WithAudience("other"))
	if err != nil {
		t.Fatalf("NewSigner returned error: %v", err)
	}
	if _, err := otherAudience.Parse(token); err == nil {
		t.Fatal("token with a different audience should be rejected")
	}
}

func TestParseChecksExpiryOnRawPayload(t *testing.T) {
	now := time.Now()
	signer, err := NewSigner(testSecret, WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewSigner returned error: %v", err)
	}

	// 自定义声明结构体没有 exp 字段，校验仍必须基于原始 payload。
	type customClaims struct {
		Subject string `json:"sub"`
	}
	token, err := signer.Sign(customClaims{Subject: "42"}, time.Minute)
	if err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}

	expired, err := NewSigner(testSecret, WithClock(func() time.Time { return now.Add(2 * time.Minute) }))
	if err != nil {
		t.Fatalf("NewSigner returned error: %v", err)
	}
	var target customClaims
	if err := expired.ParseClaims(token, &target); !isUnauthorized(err) {
		t.Fatalf("expected expired error, got %v", err)
	}
}

func TestPasswordHashAndVerify(t *testing.T) {
	encoded, err := HashPassword("s3cret-password")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if encoded == "s3cret-password" {
		t.Fatal("hash must not equal the plain password")
	}
	if !VerifyPassword(encoded, "s3cret-password") {
		t.Fatal("VerifyPassword should accept the correct password")
	}
	if VerifyPassword(encoded, "wrong-password") {
		t.Fatal("VerifyPassword should reject a wrong password")
	}
	if VerifyPassword("not-a-hash", "s3cret-password") {
		t.Fatal("VerifyPassword should reject malformed hashes")
	}
}

func TestBearerToken(t *testing.T) {
	tests := map[string]string{
		"Bearer abc":  "abc",
		"bearer abc ": "abc",
		"abc":         "",
		"":            "",
		"Basic abc":   "",
	}
	for header, want := range tests {
		if got := BearerToken(header); got != want {
			t.Fatalf("BearerToken(%q) = %q, want %q", header, got, want)
		}
	}
}

func isUnauthorized(err error) bool {
	return errs.From(err).Status == 401
}
