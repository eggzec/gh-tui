package core

// CodeownersFile is a repository's CODEOWNERS file as it was at a commit,
// unparsed.
type CodeownersFile struct {
	// Path is where the file was found, relative to the root of the
	// repository. It is empty if the commit has no CODEOWNERS file.
	Path string
	// Text is the file's content.
	Text string
}
