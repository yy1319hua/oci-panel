package controllers

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestMFAChallengeIsBoundAndSingleUse(t *testing.T) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "OCI Panel", AccountName: "admin"})
	if err != nil {
		t.Fatal(err)
	}

	sc := &SysController{mfaChallenges: make(map[string]mfaChallenge)}
	ticket, err := sc.issueMFAChallenge("admin")
	if err != nil {
		t.Fatal(err)
	}

	code, err := totp.GenerateCode(key.Secret(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// challenge 内部绑定 account，错误验证码不应被接受
	if _, ok := sc.consumeMFAChallenge(ticket, "000000", key.Secret()); ok {
		t.Fatal("MFA challenge was accepted with an invalid code")
	}
	account, ok := sc.consumeMFAChallenge(ticket, code, key.Secret())
	if !ok || account != "admin" {
		t.Fatal("valid MFA challenge was rejected or returned wrong account")
	}
	if _, ok := sc.consumeMFAChallenge(ticket, code, key.Secret()); ok {
		t.Fatal("MFA challenge was accepted more than once")
	}
}

func TestMFAChallengeLocksAfterFailedAttempts(t *testing.T) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "OCI Panel", AccountName: "admin"})
	if err != nil {
		t.Fatal(err)
	}

	sc := &SysController{mfaChallenges: make(map[string]mfaChallenge)}
	ticket, err := sc.issueMFAChallenge("admin")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < mfaChallengeTries; i++ {
		if _, ok := sc.consumeMFAChallenge(ticket, "000000", key.Secret()); ok {
			t.Fatal("invalid MFA code was accepted")
		}
	}
	code, err := totp.GenerateCode(key.Secret(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sc.consumeMFAChallenge(ticket, code, key.Secret()); ok {
		t.Fatal("MFA challenge remained usable after maximum failed attempts")
	}
}

func TestLoginFailuresAreRateLimitedAndClearedOnSuccess(t *testing.T) {
	sc := &SysController{loginFailures: make(map[string]loginFailure)}
	for i := 0; i < loginFailureLimit; i++ {
		sc.recordLoginFailure("admin")
	}
	if !sc.loginRateLimited("admin") {
		t.Fatal("login failures were not rate limited")
	}
	sc.clearLoginFailures("admin")
	if sc.loginRateLimited("admin") {
		t.Fatal("successful login did not clear the failure counter")
	}
}
