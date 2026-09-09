package knxclient

import (
	"math"
	"testing"

	"github.com/vapourismo/knx-go/knx/dpt"
)

func TestDecodeValue(t *testing.T) {
	tests := []struct {
		name string
		dpt  string
		data []byte
		want interface{}
	}{
		{name: "switch on", dpt: DPT1_001, data: dpt.DPT_1001(true).Pack(), want: true},
		{name: "switch off", dpt: DPT1_001, data: dpt.DPT_1001(false).Pack(), want: false},
		{name: "scaling", dpt: DPT5_001, data: dpt.DPT_5001(42).Pack(), want: 42},
		{name: "ratio", dpt: DPT5_005, data: dpt.DPT_5005(128).Pack(), want: 128},
		{name: "temperature", dpt: DPT9_001, data: dpt.DPT_9001(23.5).Pack(), want: 23.5},
		{name: "humidity", dpt: DPT9_007, data: dpt.DPT_9007(61.2).Pack(), want: 61.2},
		{name: "scene", dpt: DPT17_001, data: dpt.DPT_17001(3).Pack(), want: 3},
		{name: "hvac mode", dpt: DPT20_105, data: dpt.DPT_20105(3).Pack(), want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeValue(tt.dpt, tt.data)
			if err != nil {
				t.Fatalf("DecodeValue: %v", err)
			}
			if gotInt, ok := got.(int); ok {
				if math.Abs(float64(gotInt-tt.want.(int))) > 1 {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
				return
			}
			if gotFloat, ok := got.(float64); ok {
				if math.Abs(gotFloat-tt.want.(float64)) > 0.1 {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeValueRejectsUnsupportedDPT(t *testing.T) {
	if _, err := DecodeValue("DPT-99.001", []byte{0}); err == nil {
		t.Fatal("expected unsupported DPT error")
	}
}

func TestDecodeCandidates(t *testing.T) {
	candidates := DecodeCandidates(dpt.DPT_9001(23.5).Pack())
	found := false
	for _, candidate := range candidates {
		if candidate.DPT == DPT9_001 {
			found = true
			if value, ok := candidate.Value.(float64); !ok || math.Abs(value-23.5) > 0.1 {
				t.Fatalf("temperature candidate = %#v", candidate.Value)
			}
		}
	}
	if !found {
		t.Fatal("DPT-9.001 candidate not found")
	}
}
