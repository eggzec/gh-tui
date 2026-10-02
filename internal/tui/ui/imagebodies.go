package ui

import (
	"slices"
	"strings"
)

// maxImageBodies is how many bodies an ImageBodies holds at most, beside
// its document: a thread renders the comments on view again, so those
// forgotten come back as the reader reaches them.
const maxImageBodies = 256

// ImageBodies are the bodies whose images one view draws, such as a pull
// request's and its comments', by their node IDs. GitHub's rendered HTML
// of a body says where it serves the body's images that load from nowhere
// else: an attachment of a private repository, at an address signed for
// a few minutes, and an image of another host, through GitHub's proxy.
// So an image found in none of them loads only if it may be fetched where
// the markdown says. A nil *ImageBodies names no body.
type ImageBodies struct {
	private bool
	// doc is the body the view is about, which is never forgotten, and
	// ids the others, in the order they were added.
	doc    imageBody
	ids    []string
	bodies map[string]string
}

type imageBody struct{ id, text string }

// NewImageBodies returns the bodies of a repository, which private says
// is private, if that is known.
func NewImageBodies(private bool) *ImageBodies {
	return &ImageBodies{private: private, bodies: make(map[string]string)}
}

// SetPrivate says whether the repository of the bodies is private, once
// that is known.
func (b *ImageBodies) SetPrivate(private bool) {
	if b != nil {
		b.private = private
	}
}

// SetDocument sets the body the view is about, such as an issue's.
func (b *ImageBodies) SetDocument(id, text string) {
	if b != nil {
		b.doc = imageBody{id: id, text: text}
	}
}

// Add adds the body of id, such as a comment's, before it is drawn. The
// ID must be one GitHub gave: a comment still being sent has no HTML.
func (b *ImageBodies) Add(id, text string) {
	if b == nil || id == "" {
		return
	}
	if !mayHaveImage(text) {
		// Edited since, it may have lost the images it had.
		if _, ok := b.bodies[id]; ok {
			delete(b.bodies, id)
			b.ids = slices.DeleteFunc(b.ids, func(k string) bool { return k == id })
		}
		return
	}
	if _, ok := b.bodies[id]; !ok {
		if len(b.ids) >= maxImageBodies {
			delete(b.bodies, b.ids[0])
			b.ids = b.ids[1:]
		}
		b.ids = append(b.ids, id)
	}
	b.bodies[id] = text
}

// of returns the ID of a body that holds the image at url, or "" if none
// does: the first, the document first, that embeds it as an image, else
// the first that names it at all, as an image of an img tag whose address
// the markdown spelled otherwise may be. A body that only mentions the
// address, in text or code, holds no image of it in its HTML.
func (b *ImageBodies) of(url string) string {
	if b == nil || url == "" {
		return ""
	}
	texts := func(yield func(id, text string) bool) {
		if b.doc.id != "" && !yield(b.doc.id, b.doc.text) {
			return
		}
		for _, id := range b.ids {
			if !yield(id, b.bodies[id]) {
				return
			}
		}
	}
	for id, text := range texts {
		if embeds(text, url) {
			return id
		}
	}
	for id, text := range texts {
		if strings.Contains(text, url) {
			return id
		}
	}
	return ""
}

// embeds reports whether text holds url as the address of an image, in
// markdown, ![alt](url), or in an img tag, src="url", and not only as
// the start of a longer address.
func embeds(text, url string) bool {
	for at := 0; ; {
		i := strings.Index(text[at:], url)
		if i < 0 {
			return false
		}
		start, end := at+i, at+i+len(url)
		at = start + 1
		if end < len(text) && !strings.ContainsRune(")>\"' \t\r\n", rune(text[end])) {
			continue
		}
		before := strings.TrimRight(text[:start], " \t")
		before = strings.TrimSuffix(strings.TrimSuffix(before, "<"), "\"")
		before = strings.TrimSuffix(before, "'")
		if strings.HasSuffix(before, "](") || len(before) >= 4 && strings.EqualFold(before[len(before)-4:], "src=") {
			return true
		}
	}
}

// mayHaveImage reports whether a body may hold an image, as markdown or
// as an img tag in any case.
func mayHaveImage(text string) bool {
	if strings.Contains(text, "![") {
		return true
	}
	for i := strings.IndexByte(text, '<'); i >= 0 && i+4 <= len(text); {
		if strings.EqualFold(text[i+1:i+4], "img") {
			return true
		}
		next := strings.IndexByte(text[i+1:], '<')
		if next < 0 {
			break
		}
		i += 1 + next
	}
	return false
}
