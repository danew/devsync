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

func TestTargetSSHArgsUsesUserHostAndPort(t *testing.T) {
	target := Target{User: "dev", Host: "100.72.16.64", Port: "22"}
	args := target.sshArgs("true")
	want := []string{"-p", "22", "dev@100.72.16.64", "true"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", args, want)
		}
	}
}

func TestTargetSCPArgsUsesCapitalPortFlag(t *testing.T) {
	target := Target{User: "dev", Host: "100.72.16.64", Port: "2222"}
	args := target.SCPArgs("src", "/tmp/dst")
	want := []string{"-r", "-P", "2222", "src", "dev@100.72.16.64:/tmp/dst"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", args, want)
		}
	}
}
