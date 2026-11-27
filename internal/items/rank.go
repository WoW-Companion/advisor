package items

import "strings"

// PvP rank gear is bought with honor by one faction only, and the items table carries no race or faction column, so the
// rank's own name in the item name is the only signal. These are the prefixes that belong to one faction's rank ladder
// alone. A prefix both factions use (Sergeant's) or one ordinary items also start with (Knight's, Champion's, Commander's,
// General's, Marshal's, Scout's, Grunt's, Private's) is left out: an item that wrongly looks faction-locked would vanish
// from the advice, so an unsure name stays offered to both.
var rankPrefixes = []struct{ prefix, faction string }{
	{"Master Sergeant's ", "Alliance"}, {"Sergeant Major's ", "Alliance"}, {"Knight-Lieutenant's ", "Alliance"},
	{"Knight-Captain's ", "Alliance"}, {"Knight-Champion's ", "Alliance"}, {"Lieutenant Commander's ", "Alliance"},
	{"Field Marshal's ", "Alliance"}, {"Grand Marshal's ", "Alliance"},
	{"Senior Sergeant's ", "Horde"}, {"First Sergeant's ", "Horde"}, {"Stone Guard's ", "Horde"}, {"Blood Guard's ", "Horde"},
	{"Legionnaire's ", "Horde"}, {"Centurion's ", "Horde"}, {"Lieutenant General's ", "Horde"}, {"High Warlord's ", "Horde"},
}

// RankFaction is the faction ("Alliance" or "Horde") whose PvP rank ladder an item name belongs to, or "" for an item that
// is no rank gear or whose faction the name does not settle.
func RankFaction(name string) string {
	name = strings.TrimSpace(name)
	for _, p := range rankPrefixes {
		if strings.HasPrefix(name, p.prefix) {
			return p.faction
		}
	}
	return ""
}

// WrongFaction reports whether an item is rank gear of the other faction than the character's. An unknown faction is
// never a reason to leave an item out.
func WrongFaction(name, faction string) bool {
	f := RankFaction(name)
	return f != "" && faction != "" && !strings.EqualFold(f, faction)
}
