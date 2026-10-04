// Package connect chooses the ME7Logger connect line from an image's 5-baud
// address table. Needle names, key bytes, and addresses come from config/names.yaml.
// Until the slow-init needle hits, generate leaves Connect unset.
package connect

// Entry is one 30-byte row of the 5-baud address table.
type Entry struct {
	Address    byte
	Sync       byte
	Key1       byte
	Key2       byte
	KW1281     bool
	Complement byte
}

const entryLen = 30

// Policy is the connect rule from config/names.yaml.
type Policy struct {
	Key1     byte
	Key2     byte
	Prefer   byte
	Fallback byte
	Fast     []byte
}

// ParseTable reads rows starting at off. A row is kept when its sync byte is
// 0x55. Parsing stops at the first row that is not, or after 8 rows.
func ParseTable(data []byte, off int) []Entry {
	var out []Entry
	for n := 0; n < 8; n++ {
		i := off + n*entryLen
		if i+entryLen > len(data) {
			break
		}
		if data[i+1] != 0x55 {
			break
		}
		out = append(out, Entry{
			Address:    data[i],
			Sync:       data[i+1],
			Key1:       data[i+2],
			Key2:       data[i+3],
			KW1281:     data[i+5] == 1,
			Complement: data[i+6],
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func kwp(e Entry, p Policy) bool {
	return e.Key1 == p.Key1 && e.Key2 == p.Key2 && !e.KW1281
}

// Choose returns SLOW-<prefer> when that address is the KWP2000 row, otherwise
// SLOW-<fallback>. Address 0 is never chosen.
func Choose(entries []Entry, p Policy) (string, bool) {
	var prefer, fallback bool
	for _, e := range entries {
		if !kwp(e, p) || e.Address == 0 {
			continue
		}
		switch e.Address {
		case p.Prefer:
			prefer = true
		case p.Fallback:
			fallback = true
		}
	}
	switch {
	case prefer:
		return "SLOW-0x" + hex(p.Prefer), true
	case fallback:
		return "SLOW-0x" + hex(p.Fallback), true
	default:
		return "", false
	}
}

// FastTarget accepts b when it is one of the configured physical targets.
func FastTarget(b byte, allowed []byte) (byte, bool) {
	for _, a := range allowed {
		if b == a {
			return b, true
		}
	}
	return 0, false
}

func hex(b byte) string {
	const h = "0123456789ABCDEF"
	return string([]byte{h[b>>4], h[b&0x0f]})
}
