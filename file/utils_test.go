package file

import "testing"

func TestCleanAttachmentFileNameRemovesPathComponents(t *testing.T) {
	for _, input := range []string{
		"../../private.txt",
		"/../../private.txt",
		`..\\..\\private.txt`,
	} {
		if got := CleanAttachmentFileName(input); got != "private.txt" {
			t.Errorf("CleanAttachmentFileName(%q) = %q, want private.txt", input, got)
		}
	}
}
