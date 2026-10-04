package ident

import (
	"strings"
	"testing"
)

func TestFind(t *testing.T) {
	img := make([]byte, 0x17020)
	copy(img[0:12], []byte("8D0907551M  "))
	copy(img[12:32], []byte("2.7l V6/5VT         "))
	copy(img[32:36], []byte("0002"))
	copy(img[0x17000:], []byte("0261207143 1037360857"))
	id := Find(img)
	if strings.TrimSpace(id.PartNumber) != "8D0907551M" || id.SWVersion != "0002" {
		t.Fatalf("%+v", id)
	}
	if id.HWNumber != "0261207143" || id.SWNumber != "1037360857" {
		t.Fatalf("%+v", id)
	}
	if !strings.Contains(id.EngineID, "2.7l V6/5VT") {
		t.Fatalf("engine %q", id.EngineID)
	}
}
