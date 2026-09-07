package sshkey

import "strings"

// KnownHostsOption quotes for OpenSSH's configuration parser, not a shell.
// Even one argv value is parsed as a whitespace-separated list of filenames.
func KnownHostsOption(path string) string {
	if strings.ContainsAny(path, " \t\r\n\"\\'") {
		path = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(path) + `"`
	}
	return "UserKnownHostsFile=" + path
}
