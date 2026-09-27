package mermaid

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// known is the state of mermaid-live-editor's own serde test, as pako and
// js-base64 encode it; it holds its defaultState.
const known = "pako:eNpVjLFuwkAQRH9ltVUi4R9wgQR2QoMEBVUcipW99p3gbk_rs1Bk-985A5GS6UbvzYxYS8OYY3uVW21II5zKbw8pm6owavvoqD9Dlq2nHUdw4vlngu3bTqA3EoL13fvT3y4SFON-0Riisf4yP1Hx2B88T1BWewpRwvkvOd1kgo_KHk26_0-Mclp9Vi3lLWU1KRSkDwVX2KltMI868Aodq6Ol4jgnFMh_ibhfqjJ0BtPFtU9tCA1FLi11Si9lvgMWKFYF"

// decode returns the JSON of a "pako:" payload, as mermaid.live reads it.
func decode(t *testing.T, p string) []byte {
	t.Helper()
	data, ok := strings.CutPrefix(p, "pako:")
	if !ok {
		t.Fatalf("%q doesn't start with pako:", p)
	}
	b, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("zlib: %v", err)
	}
	js, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("zlib: %v", err)
	}
	return js
}

// The state is byte for byte what mermaid.live writes of its defaultState.
func TestEncodeMatchesMermaidLive(t *testing.T) {
	want := decode(t, known)
	var s state
	if err := json.Unmarshal(want, &s); err != nil {
		t.Fatal(err)
	}
	p, err := Encode(s.Code)
	if err != nil {
		t.Fatal(err)
	}
	if got := decode(t, p); !bytes.Equal(got, want) {
		t.Errorf("state\n got %s\nwant %s", got, want)
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	for _, code := range []string{
		"",
		"graph TD\n  A-->B & C\n  B-->|<b>x</b>|C",
		"sequenceDiagram\n  Alice->>Bob: héllo \"there\" \\ 🙂\n",
		strings.Repeat("flowchart LR\n  a --> b\n", 200),
	} {
		p, err := Encode(code)
		if err != nil {
			t.Fatal(err)
		}
		var s state
		if err := json.Unmarshal(decode(t, p), &s); err != nil {
			t.Fatal(err)
		}
		if s.Code != code {
			t.Errorf("round trip gave %q, want %q", s.Code, code)
		}
		if !s.UpdateDiagram || s.Mermaid != "{}" {
			t.Errorf("state %+v isn't mermaid.live's default", s)
		}
	}
}

// The link of a known diagram. Go's deflate differs from pako's, which
// mermaid.live doesn't mind, so this only pins what Go makes.
func TestViewURL(t *testing.T) {
	const want = "https://mermaid.live/view#pako:eNo0yjEKAjEQRuGrhL-evUAKQdkjWInNYMbZgMmEMalC7i6Clo_3TTwsCSLUuR3hut9rCOdtO11AUM8JsfsQQhEv_E3MBULjejMr_-s29EB88usthNESd9kzq_OPrM8A4Mwi5A"
	if got := ViewURL("graph TD\n  A-->B"); got != want {
		t.Errorf("ViewURL() = %q, want %q", got, want)
	}
}

// A diagram whose link would be too long gets none, and neither does code
// too long to be worth compressing.
func TestViewURLCap(t *testing.T) {
	// Names that compress badly.
	rnd := rand.New(rand.NewPCG(1, 2))
	var b strings.Builder
	b.WriteString("graph TD\n")
	for range 1000 {
		fmt.Fprintf(&b, "  n%x --> n%x\n", rnd.Uint64(), rnd.Uint64())
	}
	if u := ViewURL(b.String()); u != "" {
		t.Errorf("ViewURL of %d bytes = %d bytes, want none", b.Len(), len(u))
	}
	if u := ViewURL(strings.Repeat("a", maxCode+1)); u != "" {
		t.Errorf("ViewURL of code past maxCode = %d bytes, want none", len(u))
	}
	if u := ViewURL(strings.Repeat("graph TD\n  A-->B\n", 400)); u == "" || len(u) > MaxURL {
		t.Errorf("ViewURL of a repetitive diagram = %d bytes, want a link", len(u))
	}
}

// Every link ViewURL makes, up to the longest, is one a terminal link
// takes.
func TestViewURLIsLinkable(t *testing.T) {
	rnd := rand.New(rand.NewPCG(5, 6))
	var b strings.Builder
	b.WriteString("graph TD\n")
	linked := 0
	for range 200 {
		fmt.Fprintf(&b, "  n%x --> n%x\n", rnd.Uint32(), rnd.Uint32())
		u := ViewURL(b.String())
		if u == "" {
			break
		}
		if termtext.Link(u, "x") == "x" {
			t.Fatalf("a link of %d bytes isn't linkable", len(u))
		}
		linked++
	}
	if linked == 0 || linked == 200 {
		t.Errorf("%d diagrams linked, want the limit reached", linked)
	}
}

func TestKind(t *testing.T) {
	tests := []struct{ code, want string }{
		{"graph TD\n  A-->B", "flowchart"},
		{"flowchart LR", "flowchart"},
		{"graph;", "flowchart"},
		{"sequenceDiagram\n  A->>B: hi", "sequence diagram"},
		{"classDiagram-v2", "class diagram"},
		{"stateDiagram-v2\n  [*] --> A", "state diagram"},
		{"erDiagram", "ER diagram"},
		{"gantt\n  title x", "Gantt chart"},
		{"pie title Pets", "pie chart"},
		{"journey", "user journey"},
		{"gitGraph", "git graph"},
		{"mindmap", "mindmap"},
		{"timeline", "timeline"},
		{"C4Context", "C4 diagram"},
		{"xychart-beta", "XY chart"},
		{"\n\n  %% a comment\n%%{init: {'theme':'dark'}}%%\nsequenceDiagram", "sequence diagram"},
		{"---\ntitle: flowchart\n---\npie", "pie chart"},
		{"", "diagram"},
		{"   \n", "diagram"},
		{"hello world", "diagram"},
		{"Graph TD", "diagram"},
		{"sequenceDiagramX", "diagram"},
		{"\x1b[31mgraph", "diagram"},
		{"\u202egraph", "diagram"},
		{"---\nunclosed", "diagram"},
	}
	for _, tt := range tests {
		if got := Kind(tt.code); got != tt.want {
			t.Errorf("Kind(%q) = %q, want %q", tt.code, got, tt.want)
		}
	}
}
