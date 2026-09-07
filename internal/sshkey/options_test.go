package sshkey

import "testing"

func TestKnownHostsOptionProtectsConfigurationDelimiters(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"/tmp/hosts", "UserKnownHostsFile=/tmp/hosts"},
		{"/tmp/Application Support/hosts", `UserKnownHostsFile="/tmp/Application Support/hosts"`},
		{`/tmp/a"b/hosts`, `UserKnownHostsFile="/tmp/a\"b/hosts"`},
		{`/tmp/a\b/hosts`, `UserKnownHostsFile="/tmp/a\\b/hosts"`},
		{"/tmp/it's mine/hosts", `UserKnownHostsFile="/tmp/it's mine/hosts"`},
	} {
		if got := KnownHostsOption(tc.path); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.path, got, tc.want)
		}
	}
}
