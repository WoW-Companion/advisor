package main

import (
	"io"
	"strings"
	"testing"
)

func TestProfessionPlanRejectsBadArguments(t *testing.T) {
	for _, tc := range []struct{ args, want string }{
		{"", "needs --profession"},
		{"--profession Cooking --faction orc", "horde or alliance"},
		{"--profession Cooking", "needs --dataset"},
		{"--profession Cooking --dataset /no/such/dir", "not a directory"},
	} {
		t.Setenv("WOWC_DATASET", "")
		err := run(t.Context(), append([]string{"profession-plan"}, strings.Fields(tc.args)...), io.Discard)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: error %v, want %q", tc.args, err, tc.want)
		}
	}
}
