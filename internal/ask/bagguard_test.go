package ask

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/inventory"
)

var guardProtected = []inventory.Protected{
	{ItemID: 50, Item: "Kravel's Parts", Quests: []string{"Parts for Kravel"}},
	{ItemID: 51, Item: "Glowing Shard", Quests: []string{"The Glowing Shard"}},
}

func TestGuardBagAnswerDropsProtectedFromToss(t *testing.T) {
	in := "**Sell**\n- Linen Cloth x20\n\n**Toss**\n- Kravel's Parts\n- Old Rag\n\n**Keep**\n- Glowing Shard"
	out, fixes := GuardBagAnswer(in, guardProtected)
	if len(fixes) != 1 {
		t.Fatalf("fixes = %v", fixes)
	}
	toss := out[strings.Index(out, "**Toss**"):strings.Index(out, "**Keep**")]
	if strings.Contains(toss, "Kravel") || !strings.Contains(toss, "Old Rag") {
		t.Errorf("toss section = %q", toss)
	}
	if !strings.Contains(out, "Kept: Kravel's Parts (needed for Parts for Kravel)") || !strings.Contains(out, "- Glowing Shard") {
		t.Errorf("answer = %q", out)
	}
}

func TestGuardBagAnswerInlineList(t *testing.T) {
	out, _ := GuardBagAnswer("Toss: Old Rag, Glowing Shard, Broken Cup", guardProtected)
	if !strings.HasPrefix(out, "Toss: Old Rag, Broken Cup") || !strings.Contains(out, "Kept: Glowing Shard") {
		t.Errorf("answer = %q", out)
	}
}

func TestGuardBagAnswerLeavesCleanAnswerAlone(t *testing.T) {
	in := "Keep:\n- Kravel's Parts\n\nSell:\n- Linen Cloth"
	if out, fixes := GuardBagAnswer(in, guardProtected); out != in || fixes != nil {
		t.Errorf("changed: %q %v", out, fixes)
	}
}
