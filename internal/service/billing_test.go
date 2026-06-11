package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"event_id":"evt_1","action":"renew"}`)
	secret := "whsec_test"

	if !VerifySignature(secret, body, sign(secret, body)) {
		t.Error("valid signature rejected")
	}
	if VerifySignature(secret, body, sign("wrong-secret", body)) {
		t.Error("signature with wrong secret accepted")
	}
	if VerifySignature(secret, []byte(`tampered`), sign(secret, body)) {
		t.Error("signature over different body accepted")
	}
	if VerifySignature(secret, body, "") {
		t.Error("empty signature accepted")
	}
	if VerifySignature("", body, sign("", body)) {
		t.Error("empty secret must always reject")
	}
	if VerifySignature(secret, body, "zzzz-not-hex") {
		t.Error("garbage signature accepted")
	}
}
