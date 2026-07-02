package compile

import "testing"

func TestParseDiagnostics(t *testing.T) {
	stderr := "" +
		"/work/job1/main.typ:3:5: error: unknown variable: foo\n" +
		"/work/job1/chapters/intro.typ:10:2: warning: this is deprecated\n" +
		"error: file not found\n" +
		"\n" +
		"some unrelated noise line\n"
	got := parseDiagnostics(stderr, "/work/job1")

	if len(got) != 3 {
		t.Fatalf("expected 3 diagnostics, got %d: %+v", len(got), got)
	}
	if got[0].File != "main.typ" || got[0].Line != 3 || got[0].Col != 5 ||
		got[0].Severity != "error" || got[0].Message != "unknown variable: foo" {
		t.Errorf("first diag wrong: %+v", got[0])
	}
	if got[1].File != "chapters/intro.typ" || got[1].Severity != "warning" {
		t.Errorf("second diag wrong: %+v", got[1])
	}
	if got[2].Severity != "error" || got[2].Message != "file not found" || got[2].File != "" {
		t.Errorf("bare diag wrong: %+v", got[2])
	}
}
