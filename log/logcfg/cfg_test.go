package logcfg

import "testing"

func TestParse(t *testing.T) {
	text := "; comment\nECUCharacteristics = m.ecu\nSamplesPerSecond = 20\n\nnmot\nrl {EngineLoad}\n"
	f, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if f.ECU != "m.ecu" || f.SamplesPerSecond != 20 || len(f.Vars) != 2 {
		t.Fatalf("%+v", f)
	}
	if f.Vars[1].Name != "rl" || f.Vars[1].Alias != "EngineLoad" {
		t.Fatalf("%+v", f.Vars[1])
	}
	if _, err := Parse("SamplesPerSecond = 0\n"); err == nil {
		t.Fatal("expected range error")
	}
}
