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

func TestTargetForwardArgsUsesLocalForwardOptions(t *testing.T) {
	target := Target{User: "dev", Host: "100.72.16.64", Port: "22"}
	args := target.ForwardArgs([]LocalForward{
		{LocalPort: "3000", RemoteHost: "127.0.0.1", RemotePort: "3000"},
		{LocalPort: "5173", RemoteHost: "127.0.0.1", RemotePort: "5173"},
	})
	want := []string{"-N", "-L", "3000:127.0.0.1:3000", "-L", "5173:127.0.0.1:5173", "-p", "22", "dev@100.72.16.64"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", args, want)
		}
	}
}

func TestLocalForwardRenderSSHSupportsBindHost(t *testing.T) {
	forward := LocalForward{LocalHost: "127.0.0.1", LocalPort: "15432", RemoteHost: "127.0.0.1", RemotePort: "5432"}
	if got := forward.RenderSSH(); got != "127.0.0.1:15432:127.0.0.1:5432" {
		t.Fatalf("RenderSSH() = %q", got)
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

func TestTargetRenderMutagenUsesUserHost(t *testing.T) {
	target := Target{User: "dev", Host: "100.72.16.64"}
	got := target.RenderMutagen("/home/dev/workspace/work/fly-metadata")
	want := "dev@100.72.16.64:/home/dev/workspace/work/fly-metadata"
	if got != want {
		t.Fatalf("RenderMutagen() = %q, want %q", got, want)
	}
}

func TestTargetRenderMutagenUsesSSHURLForPort(t *testing.T) {
	target := Target{User: "dev", Host: "100.72.16.64", Port: "2222"}
	got := target.RenderMutagen("/home/dev/workspace/work/fly-metadata")
	want := "ssh://dev@100.72.16.64:2222/home/dev/workspace/work/fly-metadata"
	if got != want {
		t.Fatalf("RenderMutagen() = %q, want %q", got, want)
	}
}

func TestTargetRenderMutagenOmitsDefaultPort(t *testing.T) {
	target := Target{User: "dev", Host: "100.72.16.64", Port: "22"}
	got := target.RenderMutagen("/home/dev/workspace/work/fly-metadata")
	want := "dev@100.72.16.64:/home/dev/workspace/work/fly-metadata"
	if got != want {
		t.Fatalf("RenderMutagen() = %q, want %q", got, want)
	}
}

func TestTargetRenderGitOmitsDefaultPort(t *testing.T) {
	target := Target{User: "dev", Host: "100.72.16.64", Port: "22"}
	got := target.RenderGit("/home/dev/workspace/work/fly-metadata")
	want := "dev@100.72.16.64:/home/dev/workspace/work/fly-metadata"
	if got != want {
		t.Fatalf("RenderGit() = %q, want %q", got, want)
	}
}
