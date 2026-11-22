// Package route plans an efficient order for a character's quest stops: pick-ups, objectives and
// turn-ins, with map coordinates. The order comes from a deterministic optimiser (Held-Karp up to
// ExactMax stops, then nearest-neighbour improved by order-respecting 2-opt and or-opt), never from
// a language model. The rules are in docs/plan-format.md.
package route

import (
	"math"
)

// ExactMax is the most stops solved exactly; larger routes use the heuristic.
const ExactMax = 12

// Node is one place to visit. Pred lists the indexes of nodes that must come first.
type Node struct {
	X, Y float64
	Pred []int
}

// Problem is an open path from Start (nil: free start) through every node, respecting Pred.
type Problem struct {
	Nodes []Node
	Start *[2]float64
	// Scale multiplies a distance (map percent to yards, say); 0 means 1. It does not change the order
	// on a single map, only the reported length.
	Scale float64
}

func (p *Problem) dist(a, b int) float64 {
	return math.Hypot(p.Nodes[a].X-p.Nodes[b].X, p.Nodes[a].Y-p.Nodes[b].Y)
}

func (p *Problem) fromStart(b int) float64 {
	if p.Start == nil {
		return 0
	}
	return math.Hypot(p.Start[0]-p.Nodes[b].X, p.Start[1]-p.Nodes[b].Y)
}

// Length is the path length of order, in map percent (before Scale).
func (p *Problem) Length(order []int) float64 {
	t := 0.0
	for i, n := range order {
		if i == 0 {
			t += p.fromStart(n)
		} else {
			t += p.dist(order[i-1], n)
		}
	}
	return t
}

// Feasible reports whether order visits every node once with each node after its predecessors.
func (p *Problem) Feasible(order []int) bool {
	if len(order) != len(p.Nodes) {
		return false
	}
	pos := make([]int, len(order))
	for i := range pos {
		pos[i] = -1
	}
	for i, n := range order {
		if n < 0 || n >= len(pos) || pos[n] >= 0 {
			return false
		}
		pos[n] = i
	}
	for n, nd := range p.Nodes {
		for _, q := range nd.Pred {
			if pos[q] > pos[n] {
				return false
			}
		}
	}
	return true
}

// Solve returns the best order found. seeds are extra feasible orders to start the heuristic from
// (for example the naive quest-by-quest order); the result is never longer than any of them.
func Solve(p *Problem, seeds ...[]int) []int {
	n := len(p.Nodes)
	if n == 0 {
		return nil
	}
	if n <= ExactMax {
		return heldKarp(p)
	}
	cands := [][]int{nearestNeighbour(p)}
	for _, s := range seeds {
		if p.Feasible(s) {
			cands = append(cands, append([]int(nil), s...))
		}
	}
	var best []int
	bestLen := math.Inf(1)
	for _, c := range cands {
		improve(p, c)
		if l := p.Length(c); l < bestLen-1e-12 {
			best, bestLen = c, l
		}
	}
	return best
}

func predMasks(p *Problem) []uint32 {
	m := make([]uint32, len(p.Nodes))
	for i, nd := range p.Nodes {
		for _, q := range nd.Pred {
			m[i] |= 1 << uint(q)
		}
	}
	return m
}

func heldKarp(p *Problem) []int {
	n := len(p.Nodes)
	pm := predMasks(p)
	full := 1<<uint(n) - 1
	inf := math.Inf(1)
	dp := make([]float64, (full+1)*n)
	from := make([]int8, (full+1)*n)
	for i := range dp {
		dp[i] = inf
	}
	for i := 0; i < n; i++ {
		if pm[i] == 0 {
			dp[(1<<uint(i))*n+i] = p.fromStart(i)
			from[(1<<uint(i))*n+i] = -1
		}
	}
	for mask := 1; mask <= full; mask++ {
		for last := 0; last < n; last++ {
			cur := dp[mask*n+last]
			if mask&(1<<uint(last)) == 0 || cur == inf {
				continue
			}
			for nx := 0; nx < n; nx++ {
				if mask&(1<<uint(nx)) != 0 || uint32(mask)&pm[nx] != pm[nx] {
					continue
				}
				nm := mask | 1<<uint(nx)
				if v := cur + p.dist(last, nx); v < dp[nm*n+nx] {
					dp[nm*n+nx] = v
					from[nm*n+nx] = int8(last)
				}
			}
		}
	}
	bestLast, bestV := -1, inf
	for l := 0; l < n; l++ {
		if v := dp[full*n+l]; v < bestV {
			bestLast, bestV = l, v
		}
	}
	if bestLast < 0 {
		return nil // a precedence cycle: no feasible order
	}
	order := make([]int, n)
	mask, last := full, bestLast
	for i := n - 1; i >= 0; i-- {
		order[i] = last
		prev := int(from[mask*n+last])
		mask &^= 1 << uint(last)
		last = prev
	}
	return order
}

func nearestNeighbour(p *Problem) []int {
	n := len(p.Nodes)
	done := make([]bool, n)
	order := make([]int, 0, n)
	for len(order) < n {
		best, bestD := -1, math.Inf(1)
		for i := 0; i < n; i++ {
			if done[i] {
				continue
			}
			ok := true
			for _, q := range p.Nodes[i].Pred {
				if !done[q] {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			var d float64
			if len(order) == 0 {
				d = p.fromStart(i)
			} else {
				d = p.dist(order[len(order)-1], i)
			}
			if d < bestD-1e-12 {
				best, bestD = i, d
			}
		}
		if best < 0 {
			return nil
		}
		done[best] = true
		order = append(order, best)
	}
	return order
}

// improve applies 2-opt (reverse a segment) and or-opt (move a run of 1 to 3 stops) while either
// shortens the path and keeps every precedence. order is changed in place.
func improve(p *Problem, order []int) {
	n := len(order)
	cur := p.Length(order)
	for pass := 0; pass < 200; pass++ {
		changed := false
		for i := 0; i < n-1; i++ {
			for j := i + 1; j < n; j++ {
				cand := append([]int(nil), order...)
				for a, b := i, j; a < b; a, b = a+1, b-1 {
					cand[a], cand[b] = cand[b], cand[a]
				}
				if l := p.Length(cand); l < cur-1e-9 && p.Feasible(cand) {
					copy(order, cand)
					cur, changed = l, true
				}
			}
		}
		for seg := 1; seg <= 3; seg++ {
			for i := 0; i+seg <= n; i++ {
				run := append([]int(nil), order[i:i+seg]...)
				rest := append(append([]int(nil), order[:i]...), order[i+seg:]...)
				for k := 0; k <= len(rest); k++ {
					if k == i {
						continue
					}
					cand := append(append(append([]int(nil), rest[:k]...), run...), rest[k:]...)
					if l := p.Length(cand); l < cur-1e-9 && p.Feasible(cand) {
						copy(order, cand)
						cur, changed = l, true
						run = append([]int(nil), order[i:i+seg]...)
						rest = append(append([]int(nil), order[:i]...), order[i+seg:]...)
					}
				}
			}
		}
		if !changed {
			return
		}
	}
}
