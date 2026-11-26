package questie

import (
	"cmp"
	"math"
	"sort"
	"strings"
)

// Node kinds a name alone settles; the others are left for the caller to settle from what the node yields.
const (
	KindOre  = "ore"
	KindFish = "fishing"
)

// Yield is one item an object contains, from the item row's objectDrops (item field 3).
type Yield struct {
	Item  int
	Name  string
	Level int // the item's required level (item scalar 10); 0 when unknown
}

// Node is an object that yields items: a herb node, an ore vein, a fishing school, or a chest.
type Node struct {
	ID     int
	Name   string
	Kind   string // KindOre or KindFish when the name says so, else empty
	Spawns []Spawn
	Yields []Yield
}

const (
	maxYields = 24
)

// nameKind classifies by the object's name: veins and deposits are ore; schools, wreckage and
// other floating things are fishing pools.
func nameKind(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "vein"), strings.Contains(n, "deposit"):
		return KindOre
	case strings.Contains(n, "school"), strings.Contains(n, "wreckage"), strings.Contains(n, "floating debris"), strings.Contains(n, "oil spill"):
		return KindFish
	}
	return ""
}

// linkNodes builds Nodes from the object rows and the items whose objectDrops name them. A missing
// object section, or one no item points to, leaves NodesReason and no nodes.
func (ix *Index) linkNodes(lay Layout, objects, itemRows map[int]rawRow) {
	ix.Nodes = map[int]Node{}
	if len(objects) == 0 {
		ix.NodesReason = "no object rows in the database"
		return
	}
	yields := map[int][]Yield{}
	for id, row := range itemRows {
		objs := ints(row[lay.at("item", "objectDrops")])
		if len(objs) == 0 {
			continue
		}
		y := Yield{Item: id, Name: text(row.scalars()[lay.sc("item", "name")]), Level: num(row.scalars()[lay.sc("item", "requiredLevel")])}
		if y.Name == "" {
			continue
		}
		for _, o := range objs {
			if _, ok := objects[o]; ok {
				yields[o] = append(yields[o], y)
			}
		}
	}
	for id, ys := range yields {
		e := buildEntity(objects[id], lay, "object")
		if e.Name == "" || len(e.Spawns) == 0 {
			continue
		}
		sort.Slice(ys, func(i, j int) bool { return ys[i].Item < ys[j].Item })
		if len(ys) > maxYields {
			ys = ys[:maxYields]
		}
		ix.Nodes[id] = Node{ID: id, Name: e.Name, Kind: nameKind(e.Name), Spawns: e.Spawns, Yields: ys}
	}
	if len(ix.Nodes) == 0 {
		ix.NodesReason = "no object in the database yields an item"
	}
}

// Spot is one place to look, in map percent.
type Spot struct{ X, Y float64 }

// NodeZone is a zone's spawns of a set of nodes.
type NodeZone struct {
	Zone    string
	Spawns  int
	Here    bool // the character is in this zone
	Spots   []Spot
	Nodes   []string // the node names that spawn here, most spawns first
	nodeCnt map[string]int
}

// MaxSpots is how many sample coordinates a zone keeps.
const MaxSpots = 2

// NodeZones groups the spawns of the nodes by zone: the character's own zone first (its spots
// nearest the character first), then the zones with the most spawns. A zone no area table names is
// left out.
func (ix *Index) NodeZones(nodes []Node, at Where) []NodeZone {
	byZone := map[string]*NodeZone{}
	type pt struct {
		Spot
		d float64
	}
	pts := map[string][]pt{}
	for _, n := range nodes {
		for _, s := range n.Spawns {
			a, ok := ix.Areas[s.Area]
			zone := cmp.Or(a.Zone, a.Sub)
			if !ok || zone == "" {
				continue
			}
			z := byZone[zone]
			if z == nil {
				z = &NodeZone{Zone: zone, nodeCnt: map[string]int{}}
				byZone[zone] = z
			}
			z.Spawns++
			z.nodeCnt[n.Name]++
			d := -1.0
			if ix.onMap(s.Area, at) {
				z.Here = true
				d = math.Hypot(s.X-at.X, s.Y-at.Y)
			}
			pts[zone] = append(pts[zone], pt{Spot{s.X, s.Y}, d})
		}
	}
	var out []NodeZone
	for name, z := range byZone {
		p := pts[name]
		sort.SliceStable(p, func(i, j int) bool { return p[i].d >= 0 && (p[j].d < 0 || p[i].d < p[j].d) })
		for _, e := range p[:min(MaxSpots, len(p))] {
			z.Spots = append(z.Spots, e.Spot)
		}
		for n := range z.nodeCnt {
			z.Nodes = append(z.Nodes, n)
		}
		sort.Slice(z.Nodes, func(i, j int) bool {
			a, b := z.Nodes[i], z.Nodes[j]
			return z.nodeCnt[a] > z.nodeCnt[b] || z.nodeCnt[a] == z.nodeCnt[b] && a < b
		})
		out = append(out, *z)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.Here != b.Here:
			return a.Here
		case a.Spawns != b.Spawns:
			return a.Spawns > b.Spawns
		}
		return a.Zone < b.Zone
	})
	return out
}
