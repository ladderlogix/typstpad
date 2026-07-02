package api

import (
	"archive/zip"
	"bytes"
	"testing"

	"typstpad/internal/store"
)

func TestRoleAtLeast(t *testing.T) {
	cases := []struct {
		role, min string
		want      bool
	}{
		{"owner", "editor", true},
		{"editor", "editor", true},
		{"suggester", "editor", false},
		{"viewer", "suggester", false},
		{"suggester", "viewer", true},
		{"", "viewer", false},
		{"bogus", "viewer", false},
	}
	for _, c := range cases {
		if got := roleAtLeast(c.role, c.min); got != c.want {
			t.Errorf("roleAtLeast(%q,%q)=%v want %v", c.role, c.min, got, c.want)
		}
	}
}

func TestMentionedMembers(t *testing.T) {
	members := []*store.Member{
		{UserID: "u1", Email: "alice@example.com", Name: "Alice Cooper"},
		{UserID: "u2", Email: "bob@example.com", Name: "Bob"},
		{UserID: "u3", Email: "carol@example.com", Name: "Carol Danvers"},
	}
	// @alice (local part), @bob@example.com (full email), @CarolDanvers (spaceless name)
	hit := mentionedMembers("hey @alice and @bob@example.com plus @CarolDanvers", members)
	if len(hit) != 3 {
		t.Fatalf("expected 3 mentions, got %d: %v", len(hit), keys(hit))
	}
	for _, id := range []string{"u1", "u2", "u3"} {
		if _, ok := hit[id]; !ok {
			t.Errorf("expected %s mentioned", id)
		}
	}
	if len(mentionedMembers("no mentions here", members)) != 0 {
		t.Error("expected no mentions")
	}
	if _, ok := mentionedMembers("hi @dave", members)["u1"]; ok {
		t.Error("unknown handle should not match anyone")
	}
}

func keys(m map[string]*store.Member) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestCleanAndValidPath(t *testing.T) {
	if cleanPath("  foo/../bar.typ ") != "bar.typ" {
		t.Errorf("cleanPath normalize failed: %q", cleanPath("  foo/../bar.typ "))
	}
	if cleanPath("/a/b") != "a/b" {
		t.Errorf("cleanPath leading slash: %q", cleanPath("/a/b"))
	}
	valid := []string{"main.typ", "chapters/intro.typ", "img/logo.png"}
	for _, p := range valid {
		if !validProjectPath(p) {
			t.Errorf("%q should be valid", p)
		}
	}
	invalid := []string{"", ".", "../escape", "a/../../b"}
	for _, p := range invalid {
		if validProjectPath(p) {
			t.Errorf("%q should be invalid", p)
		}
	}
}

func TestCommonZipPrefix(t *testing.T) {
	mk := func(names ...string) []*zip.File {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for _, n := range names {
			w, _ := zw.Create(n)
			_, _ = w.Write([]byte("x"))
		}
		zw.Close()
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatal(err)
		}
		return zr.File
	}
	if p := commonZipPrefix(mk("proj/a.typ", "proj/sub/b.typ")); p != "proj/" {
		t.Errorf("expected shared prefix proj/, got %q", p)
	}
	if p := commonZipPrefix(mk("a.typ", "b.typ")); p != "" {
		t.Errorf("top-level files => no prefix, got %q", p)
	}
	if p := commonZipPrefix(mk("one/a.typ", "two/b.typ")); p != "" {
		t.Errorf("differing top dirs => no prefix, got %q", p)
	}
}
