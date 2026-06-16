package protocol

import "testing"

func TestValidateMessage(t *testing.T) {
	m := New("seed.hello", "sess_1", "sbx_1", 1, nil)
	if err := Validate(m); err != nil {
		t.Fatal(err)
	}
	m.Seq = 0
	if err := Validate(m); err == nil {
		t.Fatal("expected invalid seq")
	}
}
