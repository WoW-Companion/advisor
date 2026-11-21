package main

import "testing"

func TestCoInitStatus(t *testing.T) {
	cases := []struct {
		name   string
		hr     int32
		uninit bool
		note   bool
		fail   bool
	}{
		{"S_OK", sOK, true, false, false},
		{"S_FALSE", sFalse, true, false, false},
		{"RPC_E_CHANGED_MODE", rpcEChangedMode, false, true, false},
		{"E_OUTOFMEMORY", -2147024882, false, false, true},
		{"E_INVALIDARG", -2147024809, false, false, true},
	}
	for _, c := range cases {
		uninit, note, err := coInitStatus(c.hr)
		if uninit != c.uninit || (note != "") != c.note || (err != nil) != c.fail {
			t.Errorf("%s: got uninit=%v note=%q err=%v", c.name, uninit, note, err)
		}
	}
}
