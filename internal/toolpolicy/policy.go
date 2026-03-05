package toolpolicy

import "slices"

type Policy struct {
	Allow []string
	Deny  []string
}

func (p Policy) Allowed(name string) bool {
	if slices.Contains(p.Deny, name) {
		return false
	}
	if len(p.Allow) == 0 {
		return true
	}
	return slices.Contains(p.Allow, name)
}
