// Package source records where converted Markdown came from
package source

import (
	"path/filepath"
	"strings"
)

// Header returns the comment line that identifies the original file by its
// absolute path, e.g. "<!-- source: /docs/report.pdf -->".
func Header(inputPath string) string {
	path := inputPath
	if abs, err := filepath.Abs(inputPath); err == nil {
		path = abs
	}
	// A comment cannot contain its own terminator or a line break
	path = strings.NewReplacer("-->", "--%3E", "\n", "%0A", "\r", "%0D").Replace(path)
	return "<!-- source: " + path + " -->"
}

// Prepend places the source header at the very start of the Markdown
func Prepend(markdown, inputPath string) string {
	return Header(inputPath) + "\n\n" + markdown
}
