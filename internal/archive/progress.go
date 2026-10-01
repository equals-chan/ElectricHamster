package archive

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// 7-Zip terminal progress is redrawn in place. Depending on the platform and
// whether stdout is a tty, the redraw is done with carriage returns and/or
// backspaces: it prints a frame, then "\b"*n + " "*n + "\b"*n to erase it
// before printing the next frame. progressParser replays that editing into a
// logical line, then emits each distinct visible frame.
type progressParser struct {
	buf     []rune
	cursor  int
	lastStr string
	onLine  func(string)
}

func newProgressParser(onLine func(string)) *progressParser {
	return &progressParser{onLine: onLine}
}

func (p *progressParser) consume(r io.Reader) error {
	br := bufio.NewReaderSize(r, 4096)
	for {
		ru, _, err := br.ReadRune()
		if err == io.EOF {
			p.emit()
			return nil
		}
		if err != nil {
			return err
		}
		p.feed(ru)
	}
}

func (p *progressParser) feed(ru rune) {
	switch ru {
	case '\b':
		p.emit()
		if p.cursor > 0 {
			p.cursor--
		}
	case '\r', '\n':
		p.emit()
		p.cursor = 0
	default:
		if p.cursor < len(p.buf) {
			p.buf[p.cursor] = ru
		} else {
			p.buf = append(p.buf, ru)
		}
		p.cursor++
	}
}

func (p *progressParser) emit() {
	s := strings.TrimRight(string(p.buf), " ")
	if strings.TrimSpace(s) == "" || s == p.lastStr {
		return
	}
	p.lastStr = s
	if p.onLine != nil {
		p.onLine(s)
	}
}

var (
	rePercent = regexp.MustCompile(`^\s*(\d{1,3})%`)
	reScan    = regexp.MustCompile(`\bScan\b`)
	reHeader  = regexp.MustCompile(`Header creation`)
	reItem    = regexp.MustCompile(`^\s*\d{1,3}%\s+\d+\s+([+\-U])\s+(.*)$`)
)

// parseProgressLine interprets one visible 7-Zip progress frame. It returns
// false for lines that carry no useful information.
func parseProgressLine(line string) (Progress, bool) {
	t := strings.TrimSpace(line)
	if t == "" {
		return Progress{}, false
	}
	p := Progress{}
	hasPercent := false
	if m := rePercent.FindStringSubmatch(t); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			p.Percent = float64(n)
			hasPercent = true
		}
	}
	switch {
	case reHeader.MatchString(t):
		p.Phase = PhaseHeader
	case reScan.MatchString(t):
		p.Phase = PhaseScan
	default:
		p.Phase = PhaseCompress
	}
	if m := reItem.FindStringSubmatch(line); m != nil {
		p.Current = strings.TrimSpace(m[2])
	}
	if !hasPercent && p.Current == "" && p.Phase == PhaseCompress {
		return Progress{}, false
	}
	return p, true
}
