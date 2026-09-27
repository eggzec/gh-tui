package markdown

// Block is a fenced code block of a source, as its language shows it.
type Block struct {
	// Lang is the first word of the block's info string, in lower case.
	Lang string
	// Full is the markdown of the block shown in full.
	Full string
	// Collapsed is a line of markdown that shows in place of the block
	// until the reader opens it, or "" when the block always shows in
	// full.
	Collapsed string
	// URL is a page that shows the block, for the open action, or "".
	URL string
}

// fenced returns how the fenced code block src, from its opening fence to
// its closing one, whose info string starts with lang, shows. It is where
// a language gets a way of its own to show, such as a diagram; for now
// every block, mermaid too, shows in full as code, highlighted if its
// language is one that is and b allows.
func fenced(lang, src string, b *budget) Block {
	return Block{Lang: lang, Full: withLang(lang, src, b)}
}

// showBlock is fenced, and a test's own way to show blocks while it runs.
var showBlock = fenced

// Blocks returns the fenced code blocks of src in order. The index of a
// collapsible one is what [Renderer.Render] takes to show it in full.
func Blocks(src string) []Block {
	var out []Block
	scan(src, "", func(string) {}, func(_ int, b Block) { out = append(out, b) })
	return out
}
