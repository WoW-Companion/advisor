package ui

import (
	"os/exec"
	"strings"
	"testing"
)

// The AI settings (mode, key field, models, Test messages) run as real code under Node.
func TestAISettingsLogicUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	if out, err := exec.Command(node, "aiset_logic_test.js").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// The page loads the module, has the question and the key field as a password box, and the shell can open
// the key and install pages the section links to.
func TestAISettingsWiring(t *testing.T) {
	html, js, rs := read(t, "index.html"), read(t, "app.js"), read(t, "../src-tauri/src/main.rs")
	for _, want := range []string{`src="aiset.js"`, `How should the companion answer?`, `id="ai-key" type="password"`, `id="ai-test"`, `id="ai-advanced"`} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	if !strings.Contains(js, "WowcAiSettings.create") {
		t.Error("app.js does not start the AI settings")
	}
	aiset := read(t, "aiset.js")
	for _, u := range []string{"https://platform.claude.com/settings/keys", "https://platform.openai.com/api-keys", "https://openrouter.ai/keys", "https://ollama.com/download", "https://lmstudio.ai/download"} {
		if !strings.Contains(aiset, u) || !strings.Contains(rs, `"`+u+`"`) {
			t.Errorf("%s must be in aiset.js and the shell's page list", u)
		}
	}
}
