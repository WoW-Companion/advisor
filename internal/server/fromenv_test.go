package server

import "testing"

func TestFromEnvExplicitWins(t *testing.T) {
	t.Setenv(SavedVariablesEnv, "/env/ForeverScribe.lua")
	for _, c := range []struct {
		name, flag, want, source string
	}{
		{"flag beats env", "/flag/ForeverScribe.lua", "/flag/ForeverScribe.lua", SVFlag},
		{"env used when no flag", "", "/env/ForeverScribe.lua", SVEnv},
	} {
		got := Config{SavedVariables: c.flag}.FromEnv()
		if got.SavedVariables != c.want || got.SVSource != c.source || got.SVNote != "" {
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
}
