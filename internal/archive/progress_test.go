package archive

import (
	"strings"
	"testing"
)

// frame mimics a 7-Zip in-place redraw: print text, then backspace over it,
// overwrite with spaces, and backspace again before the next frame.
func frame(s string) string {
	n := len([]rune(s))
	return s + strings.Repeat("\b", n) + strings.Repeat(" ", n) + strings.Repeat("\b", n)
}

func TestParseProgressLine(t *testing.T) {
	cases := []struct {
		line  string
		ok    bool
		phase Phase
		pct   float64
		cur   string
	}{
		{"  0M Scan ", true, PhaseScan, 0, ""},
		{"  0%", true, PhaseCompress, 0, ""},
		{" 42%", true, PhaseCompress, 42, ""},
		{"100% 22 + src/sub/nested.txt", true, PhaseCompress, 100, "src/sub/nested.txt"},
		{" 37% 5 + 中文/文件.txt", true, PhaseCompress, 37, "中文/文件.txt"},
		{"100% 22 Header creation", true, PhaseHeader, 100, ""},
		{"", false, "", 0, ""},
		{"   ", false, "", 0, ""},
		{"junk without percent", false, "", 0, ""},
	}
	for _, c := range cases {
		got, ok := parseProgressLine(c.line)
		if ok != c.ok {
			t.Errorf("parseProgressLine(%q) ok=%v, want %v", c.line, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if got.Phase != c.phase || got.Percent != c.pct || got.Current != c.cur {
			t.Errorf("parseProgressLine(%q) = %+v, want phase=%s pct=%v cur=%q",
				c.line, got, c.phase, c.pct, c.cur)
		}
	}
}

func TestProgressParserReplaysRedraw(t *testing.T) {
	stream := frame("  0M Scan ") +
		frame("  0%") +
		frame("100% 22 + src/sub/nested.txt") +
		frame("100% 22 Header creation")

	var got []Progress
	p := newProgressParser(func(line string) {
		if pr, ok := parseProgressLine(line); ok {
			got = append(got, pr)
		}
	})
	if err := p.consume(strings.NewReader(stream)); err != nil {
		t.Fatalf("consume: %v", err)
	}

	want := []Progress{
		{Phase: PhaseScan, Percent: 0},
		{Phase: PhaseCompress, Percent: 0},
		{Phase: PhaseCompress, Percent: 100, Current: "src/sub/nested.txt"},
		{Phase: PhaseHeader, Percent: 100},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d frames %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i].Phase != want[i].Phase || got[i].Percent != want[i].Percent || got[i].Current != want[i].Current {
			t.Errorf("frame %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestProgressParserDeduplicates(t *testing.T) {
	stream := frame(" 10%") + frame(" 10%") + frame(" 20%")
	var count int
	p := newProgressParser(func(string) { count++ })
	if err := p.consume(strings.NewReader(stream)); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if count != 2 {
		t.Errorf("emitted %d distinct frames, want 2", count)
	}
}
