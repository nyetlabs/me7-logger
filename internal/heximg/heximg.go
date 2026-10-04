// Package heximg reads a hex listing into an image.
// A "size 0xNN" line sets the length. Other lines are an optional hex
// offset, a colon, and hex bytes. '#' starts a comment line.
package heximg

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func Read(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var img []byte
	sized := false
	at := 0
	lineNo := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "size "); ok {
			n, err := strconv.ParseUint(strings.TrimSpace(rest), 0, 32)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
			}
			img = make([]byte, n)
			sized = true
			at = 0
			continue
		}
		if i := strings.IndexByte(line, ':'); i >= 0 {
			n, err := strconv.ParseUint(strings.TrimSpace(line[:i]), 16, 32)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
			}
			at = int(n)
			line = strings.TrimSpace(line[i+1:])
		}
		for _, fd := range strings.Fields(line) {
			b, err := hex.DecodeString(fd)
			if err != nil || len(b) != 1 {
				return nil, fmt.Errorf("%s:%d: %q", path, lineNo, fd)
			}
			if at >= len(img) {
				if sized {
					return nil, fmt.Errorf("%s:%d: offset %X past size", path, lineNo, at)
				}
				n := make([]byte, at+1)
				copy(n, img)
				img = n
			}
			img[at] = b[0]
			at++
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return img, nil
}
