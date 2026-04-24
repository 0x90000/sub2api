package service

import "testing"

func TestSanitizeWindsurfText_RewritesInternalPaths(t *testing.T) {
	input := `read /tmp/windsurf-workspace/src/main.go and /opt/windsurf/bin/tool`
	got := sanitizeWindsurfText(input)
	if got == input {
		t.Fatalf("sanitizeWindsurfText() did not change %q", input)
	}
	if got != `read ./src/main.go and [internal]` {
		t.Fatalf("sanitizeWindsurfText() = %q", got)
	}
}

func TestWindsurfPathSanitizeStream_HoldsSplitWorkspaceLiteral(t *testing.T) {
	stream := newWindsurfPathSanitizeStream()

	part1 := stream.Feed("open /tmp/wind")
	part2 := stream.Feed("surf-workspace/app.js")
	part3 := stream.Flush()

	got := part1 + part2 + part3
	if got != "open ./app.js" {
		t.Fatalf("sanitized stream = %q, want %q", got, "open ./app.js")
	}
	if containsWindsurfInternalPath(got) {
		t.Fatalf("sanitized stream still leaked internal path: %q", got)
	}
}

func TestSanitizeWindsurfText_RewritesReferenceWorkspacePaths(t *testing.T) {
	input := `write /home/user/projects/workspace-abcd1234/src/app.ts`
	got := sanitizeWindsurfText(input)
	if got != `write ./src/app.ts` {
		t.Fatalf("sanitizeWindsurfText() = %q, want %q", got, `write ./src/app.ts`)
	}
}

func TestResolveWindsurfCascadeWorkspacePath_UsesStableAccountWorkspaceUnderConfiguredRoot(t *testing.T) {
	got := resolveWindsurfCascadeWorkspacePath(&Account{ID: 42}, "secret-token", "/tmp/windsurf-workspace")
	want := "/tmp/windsurf-workspace/workspace-account-42"
	if got != want {
		t.Fatalf("resolveWindsurfCascadeWorkspacePath() = %q, want %q", got, want)
	}
}

func TestSanitizeWindsurfText_RewritesAccountWorkspacePaths(t *testing.T) {
	input := `read /tmp/windsurf-workspace/workspace-account-42/src/main.go`
	got := sanitizeWindsurfText(input)
	if got != `read ./src/main.go` {
		t.Fatalf("sanitizeWindsurfText() = %q, want %q", got, `read ./src/main.go`)
	}
}
