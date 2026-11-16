package parse

import (
	"encoding/binary"
	"testing"
)

// record writes one capture record as the shim does.
func record(match, pattern, capture, start, end uint32) []byte {
	b := make([]byte, captureRecord)
	for i, v := range []uint32{match, pattern, capture, start, end, 0, 0} {
		binary.LittleEndian.PutUint32(b[4*i:], v)
	}
	return b
}

func TestDecodeRefusesARecordOutsideItsBounds(t *testing.T) {
	q := &query{names: []string{"x"}, predicates: [][]predicate{nil}}
	content := []byte("abc")
	cases := map[string][]byte{
		"an unknown capture":      record(0, 0, 5, 0, 1),
		"an unknown pattern":      record(0, 3, 0, 0, 1),
		"an end past the content": record(0, 0, 0, 0, 9),
		"a start after the end":   record(0, 0, 0, 2, 1),
	}
	for name, raw := range cases {
		if _, err := decode(q, raw, content); err == nil {
			t.Errorf("decode(%s) error = nil, want an error", name)
		}
	}
	if got, err := decode(q, record(0, 0, 0, 0, 3), content); err != nil || len(got.Matches) != 1 {
		t.Errorf("decode(a sound record) = %+v, %v; want one match", got, err)
	}
}
