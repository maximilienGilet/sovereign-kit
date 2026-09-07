package sshkey

import (
	"encoding/base64"
	"fmt"
	"strings"
)

var supportedTypes = map[string]struct{}{
	"ssh-ed25519":                        {},
	"ssh-rsa":                            {},
	"ssh-dss":                            {},
	"ecdsa-sha2-nistp256":                {},
	"ecdsa-sha2-nistp384":                {},
	"ecdsa-sha2-nistp521":                {},
	"sk-ssh-ed25519@openssh.com":         {},
	"sk-ecdsa-sha2-nistp256@openssh.com": {},
}

func Normalize(publicKey string) (string, error) {
	fields := strings.Fields(publicKey)
	if len(fields) < 2 {
		return "", fmt.Errorf("SSH public key must contain an OpenSSH key type and blob")
	}
	if _, ok := supportedTypes[fields[0]]; !ok {
		return "", fmt.Errorf("unsupported OpenSSH public key type %q", fields[0])
	}
	decoded, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(fields[1])
	}
	if err != nil || len(decoded) == 0 {
		return "", fmt.Errorf("SSH public key blob is not valid base64")
	}
	return fields[0] + " " + fields[1], nil
}
