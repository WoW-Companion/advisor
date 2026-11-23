package clients

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the generated instruction files")

func TestGeneratedFilesInSync(t *testing.T) {
	for _, f := range Files() {
		path := filepath.FromSlash(f.Path)
		if *update {
			if err := os.WriteFile(path, []byte(f.Text), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != f.Text {
			t.Errorf("%s is out of date with body.md (run: go test ./clients -update): %v", f.Path, err)
		}
	}
}

func TestEveryToolNamed(t *testing.T) {
	for _, tool := range []string{"server_info", "list_characters", "character_brief", "whats_next", "upgrade_finder", "upgrade_paths", "stat_check", "update_digest", "profession_plan", "talent_plan", "talent_tree", "get_character_profile", "set_character_profile", "dungeon_plan", "farm_spots", "quest_chain", "quest_plan", "inventory_review"} {
		if !strings.Contains(body, "`"+tool+"`") {
			t.Errorf("body.md never mentions %s", tool)
		}
	}
}
