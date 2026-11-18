package layoutdiff

// DynamicMask returns the A uids that change between two runs of the SAME
// engine (ads, carousels, timestamps): the regions to ignore when comparing
// engines. Elements are masked with their whole subtree, so a moving carousel
// does not report every child.
func DynamicMask(x, x2 *Tree) map[int]bool {
	m := MatchTrees(x, x2)
	res := Analyze(x, x2, m, map[int]bool{}, map[int]bool{})
	mask := map[int]bool{}
	for _, c := range res.Causes {
		if c.Side == "A" {
			for _, s := range Subtree(c.Node) {
				mask[s.UID] = true
			}
			continue
		}
		if c.Node.Parent != nil {
			if pa := m.RevPair[c.Node.Parent.UID]; pa != nil {
				mask[pa.UID] = true
			}
		}
	}
	return mask
}
