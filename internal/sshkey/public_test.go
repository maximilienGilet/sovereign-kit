package sshkey

import (
	"strings"
	"testing"
)

func TestNormalizeAcceptsSupportedOpenSSHKeyTypes(t *testing.T) {
	for _, publicKey := range []string{
		"ssh-ed25519 QUFBQQ== comment",
		"ssh-rsa QUFBQQ==",
		"ecdsa-sha2-nistp256 QUFBQQ==",
		"sk-ssh-ed25519@openssh.com QUFBQQ==",
		"sk-ecdsa-sha2-nistp256@openssh.com QUFBQQ==",
	} {
		normalized, err := Normalize(publicKey)
		if err != nil {
			t.Fatalf("key=%q error=%v", publicKey, err)
		}
		parts := strings.Fields(publicKey)
		want := parts[0] + " " + parts[1]
		if normalized != want {
			t.Fatalf("key=%q normalized=%q want=%q", publicKey, normalized, want)
		}
	}
}

func TestNormalizeRejectsPrivateUnknownAndInvalidKeys(t *testing.T) {
	for _, value := range []string{
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		"unknown-key QUFBQQ==",
		"ssh-ed25519 not-base64!",
		"ssh-ed25519",
	} {
		if normalized, err := Normalize(value); err == nil {
			t.Fatalf("value=%q normalized=%q", value, normalized)
		}
	}
}

func TestNormalizeIgnoresComment(t *testing.T) {
	left, err := Normalize("ssh-ed25519 QUFBQQ== old-comment")
	if err != nil {
		t.Fatal(err)
	}
	right, err := Normalize("ssh-ed25519 QUFBQQ== new-comment")
	if err != nil {
		t.Fatal(err)
	}
	if left != right {
		t.Fatalf("left=%q right=%q", left, right)
	}
}
