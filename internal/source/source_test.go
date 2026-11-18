package source

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepend_AbsolutePathFirstLine(t *testing.T) {
	abs, err := filepath.Abs("docs/report.pdf")
	if err != nil {
		t.Fatal(err)
	}
	got := Prepend("# Title\n", "docs/report.pdf")
	want := "<!-- source: " + abs + " -->\n\n# Title\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHeader_CannotBreakOutOfComment(t *testing.T) {
	got := Header("/tmp/a-->b\n.pdf")
	if strings.Count(got, "-->") != 1 || strings.Contains(got, "\n") {
		t.Errorf("unsafe header: %q", got)
	}
}
