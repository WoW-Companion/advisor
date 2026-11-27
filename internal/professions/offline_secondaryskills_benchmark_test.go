package professions

import "testing"

// The offline secondary-skill benchmark, second set: the steps the skill-up answer gives for Cooking, First Aid and Fishing at skills the first set (offline_secondary_benchmark_test.go) does not cover,
// the colour and crafts of the recipes listed for Cooking and First Aid at more skills, beside the dataset's profession_routes and recipe thresholds. make offline-benchmark runs it.

var secPathsMore = []struct {
	prof  string
	skill int
}{
	{"Cooking", 25}, {"Cooking", 75}, {"Cooking", 130}, {"Cooking", 200}, {"Cooking", 260}, {"Cooking", 299},
	{"Fishing", 1}, {"Fishing", 50}, {"Fishing", 75}, {"Fishing", 150}, {"Fishing", 225}, {"Fishing", 299},
	{"First Aid", 1}, {"First Aid", 60}, {"First Aid", 100}, {"First Aid", 140}, {"First Aid", 200}, {"First Aid", 250},
}

func TestOfflineBenchmarkSecondaryMoreRouteA(t *testing.T) { secondaryRoutes(t, "d", secPathsMore[:6]) }
func TestOfflineBenchmarkSecondaryMoreRouteB(t *testing.T) {
	secondaryRoutes(t, "e", secPathsMore[6:12])
}
func TestOfflineBenchmarkSecondaryMoreRouteC(t *testing.T) {
	secondaryRoutes(t, "f", secPathsMore[12:])
}

var secColourMore = []struct {
	prof  string
	skill int
}{{"Cooking", 20}, {"Cooking", 60}, {"Cooking", 100}, {"First Aid", 100}, {"First Aid", 140}, {"First Aid", 170}}

func TestOfflineBenchmarkSecondaryMoreColoursA(t *testing.T) {
	skillUpColours(t, "p", secColourMore[:3])
}
func TestOfflineBenchmarkSecondaryMoreColoursB(t *testing.T) {
	skillUpColours(t, "q", secColourMore[3:])
}

// The skill cap a character level allows Cooking, First Aid and Fishing, at levels the rank benchmark does not use: the ranks of the secondary skills are books and quests, so the
// goal stops at the highest rank the level may train, as for any profession.
var secCapMore = []struct {
	prof  string
	level int
}{{"Cooking", 9}, {"Cooking", 35}, {"First Aid", 19}, {"First Aid", 20}, {"Fishing", 34}, {"Fishing", 50}}

func TestOfflineBenchmarkSecondaryMoreCapsA(t *testing.T) { skillCaps(t, "c", secCapMore[:3]) }
func TestOfflineBenchmarkSecondaryMoreCapsB(t *testing.T) { skillCaps(t, "d", secCapMore[3:]) }
