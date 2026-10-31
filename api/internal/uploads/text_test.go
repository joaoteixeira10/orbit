package uploads

import (
	"archive/zip"
	"bytes"
	"testing"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for n, s := range files {
		w, _ := z.Create(n)
		_, _ = w.Write([]byte(s))
	}
	_ = z.Close()
	return b.Bytes()
}

// TestTextOf: Word, PowerPoint (slides in order) and Excel files say what's
// in them; text as it is; anything else nothing.
func TestTextOf(t *testing.T) {
	w := `<w:document xmlns:w="w"><w:body><w:p><w:r><w:t>Hello</w:t></w:r><w:r><w:t xml:space="preserve"> world</w:t></w:r></w:p><w:p><w:r><w:t>Next</w:t></w:r></w:p></w:body></w:document>`
	if got := TextOf("a.docx", "application/zip", zipOf(t, map[string]string{"word/document.xml": w})); got != "Hello world Next" {
		t.Errorf("docx: %q", got)
	}
	slide := func(s string) string {
		return `<p:sld xmlns:p="p" xmlns:a="a"><a:p><a:r><a:t>` + s + `</a:t></a:r></a:p></p:sld>`
	}
	if got := TextOf("deck.pptx", "", zipOf(t, map[string]string{"ppt/slides/slide10.xml": slide("Ten"), "ppt/slides/slide2.xml": slide("Two")})); got != "Two Ten" {
		t.Errorf("pptx: %q", got)
	}
	if got := TextOf("b.xlsx", "", zipOf(t, map[string]string{"xl/sharedStrings.xml": `<sst><si><t>Budget</t></si><si><t>Rent</t></si></sst>`})); got != "Budget Rent" {
		t.Errorf("xlsx: %q", got)
	}
	if got := TextOf("n.md", "text/plain", []byte("# Notes\n\nsome  text")); got != "# Notes some text" {
		t.Errorf("md: %q", got)
	}
	if got := TextOf("x.bin", "application/octet-stream", []byte{0, 1, 2}); got != "" {
		t.Errorf("bin: %q", got)
	}
}
