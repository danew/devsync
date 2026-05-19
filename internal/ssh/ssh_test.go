package ssh

import "testing"

func TestParseTargetSupportsUserHostAndPort(t *testing.T) {
	target := ParseTarget("dev@100.72.16.64:2222")
	if target.User != "dev" || target.Host != "100.72.16.64" || target.Port != "2222" {
		t.Fatalf("unexpected target: %#v", target)
	}
	if target.String() != "dev@100.72.16.64:2222" {
		t.Fatalf("string = %q", target.String())
	}
}

func TestParseTargetPreservesSSHConfigAlias(t *testing.T) {
	target := ParseTarget("core-dev")
	if target.Alias != "core-dev" || target.String() != "core-dev" {
		t.Fatalf("unexpected alias target: %#v", target)
	}
}
