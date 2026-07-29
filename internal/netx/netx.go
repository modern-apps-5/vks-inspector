// Package netx is the address arithmetic shared by every CIDR and pool-sizing
// check.
//
// It exists so the overlap and containment logic is written and tested once.
// The off-by-one cases here — adjacent-but-disjoint ranges, /31 and /32,
// inclusive end addresses — are exactly the ones that produce a confidently
// wrong preflight verdict, and they deserve a package with its own tests rather
// than being reimplemented per check.
//
// Everything is built on net/netip: values are comparable, there is no hidden
// allocation, and an IPv4 address parsed from a string is never silently a
// 4-in-6 address.
package netx

import (
	"fmt"
	"math/big"
	"net/netip"
	"strings"
)

// Named pairs a network with where in the config it came from, so a finding can
// say "kubernetes.podCIDRs[0] overlaps networks.workload[1]" rather than just
// printing two CIDRs and leaving the operator to find them.
type Named struct {
	// Source is the config path, e.g. "networks.management.cidr".
	Source string
	// Label is the human name, e.g. "management".
	Label string
	// Prefix is set when the entry came from a CIDR.
	Prefix netip.Prefix
	// Range is set when the entry came from a start/end pair.
	Range *Range
	// Routable records whether the config declared this range as needing to be
	// reachable from outside.
	Routable bool

	// Group identifies the network this entry belongs to, e.g.
	// "networks.management". IsParent marks the group's own subnet as opposed
	// to a range carved out of it.
	//
	// These exist so overlap detection does not report a network's own static
	// range as colliding with its own subnet. That containment is required,
	// not a conflict — range.containment asserts it — and reporting it as a
	// blocker would make the tool cry wolf on every correctly-written config.
	Group    string
	IsParent bool
}

// SiblingOfParent reports whether one of the pair is the other's own subnet.
func (n Named) SiblingOfParent(o Named) bool {
	return n.Group != "" && n.Group == o.Group && (n.IsParent || o.IsParent)
}

// String renders the network for a report.
func (n Named) String() string {
	switch {
	case n.Range != nil:
		return n.Range.String()
	case n.Prefix.IsValid():
		return n.Prefix.String()
	default:
		return "<invalid>"
	}
}

// Describe renders source and value together.
func (n Named) Describe() string { return n.Source + " (" + n.String() + ")" }

// Overlaps reports whether two named networks intersect, in any combination of
// prefix and range.
func (n Named) Overlaps(o Named) bool {
	a, aok := n.AsRange()
	b, bok := o.AsRange()
	if !aok || !bok {
		return false
	}
	return a.Overlaps(b)
}

// AsRange normalises a Named to an address range.
func (n Named) AsRange() (Range, bool) {
	if n.Range != nil {
		return *n.Range, true
	}
	if n.Prefix.IsValid() {
		return RangeOfPrefix(n.Prefix), true
	}
	return Range{}, false
}

// Range is an inclusive address range. Inclusive because that is how vSphere,
// NSX and ALB all express IP pools, and converting at the edges rather than in
// the middle keeps the off-by-one in one place.
type Range struct {
	Start netip.Addr
	End   netip.Addr
}

// ParseRange parses an inclusive "start-end" pair.
func ParseRange(start, end string) (Range, error) {
	s, err := netip.ParseAddr(strings.TrimSpace(start))
	if err != nil {
		return Range{}, fmt.Errorf("range start %q: %w", start, err)
	}
	e, err := netip.ParseAddr(strings.TrimSpace(end))
	if err != nil {
		return Range{}, fmt.Errorf("range end %q: %w", end, err)
	}
	r := Range{Start: s, End: e}
	if err := r.Validate(); err != nil {
		return Range{}, err
	}
	return r, nil
}

// Validate rejects ranges that cannot mean anything.
func (r Range) Validate() error {
	if !r.Start.IsValid() || !r.End.IsValid() {
		return fmt.Errorf("range has an invalid address")
	}
	if r.Start.Is4() != r.End.Is4() {
		return fmt.Errorf("range %s mixes IPv4 and IPv6", r)
	}
	if r.End.Less(r.Start) {
		return fmt.Errorf("range %s ends before it starts", r)
	}
	return nil
}

// RangeOfPrefix converts a prefix to its inclusive address range.
//
// The whole prefix is used, including the network and broadcast addresses. For
// overlap detection that is correct: two prefixes that share only a broadcast
// address still collide. Checks that care about *usable* addresses ask for
// UsableCount instead.
func RangeOfPrefix(p netip.Prefix) Range {
	p = p.Masked()
	return Range{Start: p.Addr(), End: lastAddr(p)}
}

// Contains reports whether the range includes an address.
func (r Range) Contains(a netip.Addr) bool {
	if !a.IsValid() || a.Is4() != r.Start.Is4() {
		return false
	}
	return !a.Less(r.Start) && !r.End.Less(a)
}

// Overlaps reports whether two ranges share any address.
//
// Adjacency is not overlap: 10.0.0.0-10.0.0.255 and 10.0.1.0-10.0.1.255 do not
// overlap. This is the case that a naive implementation gets wrong.
func (r Range) Overlaps(o Range) bool {
	if r.Start.Is4() != o.Start.Is4() {
		return false // different families cannot collide
	}
	return !r.End.Less(o.Start) && !o.End.Less(r.Start)
}

// ContainsRange reports whether r fully contains o.
func (r Range) ContainsRange(o Range) bool {
	return r.Contains(o.Start) && r.Contains(o.End)
}

// Count returns the number of addresses in the range, inclusive.
//
// big.Int because an IPv6 range does not fit in a uint64 and silently
// overflowing a sizing check is exactly the kind of quiet wrongness this tool
// exists to avoid.
func (r Range) Count() *big.Int {
	start := new(big.Int).SetBytes(addrBytes(r.Start))
	end := new(big.Int).SetBytes(addrBytes(r.End))
	return new(big.Int).Add(new(big.Int).Sub(end, start), big.NewInt(1))
}

// CountInt returns Count as an int, saturating at maxInt. Sizing checks compare
// against small expected counts, so saturation is safe there — but callers
// wanting exactness must use Count.
func (r Range) CountInt() int {
	c := r.Count()
	if !c.IsInt64() {
		return int(^uint(0) >> 1)
	}
	n := c.Int64()
	if n > int64(int(^uint(0)>>1)) {
		return int(^uint(0) >> 1)
	}
	return int(n)
}

// IsContiguousWith reports whether o starts immediately after r ends.
func (r Range) IsContiguousWith(o Range) bool {
	next := r.End.Next()
	return next.IsValid() && next == o.Start
}

// String renders "start-end".
func (r Range) String() string { return r.Start.String() + "-" + r.End.String() }

// Intersection returns the overlapping portion of two ranges, if any.
func (r Range) Intersection(o Range) (Range, bool) {
	if !r.Overlaps(o) {
		return Range{}, false
	}
	start := r.Start
	if start.Less(o.Start) {
		start = o.Start
	}
	end := r.End
	if o.End.Less(end) {
		end = o.End
	}
	return Range{Start: start, End: end}, true
}

// ParsePrefix parses a CIDR and rejects one whose host bits are set.
//
// "10.0.0.5/24" is almost always a mistake in a config that means to declare a
// network, and silently masking it to 10.0.0.0/24 hides an operator error that
// often indicates a deeper misunderstanding of the address plan.
func ParsePrefix(s string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(strings.TrimSpace(s))
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%q is not a valid CIDR: %w", s, err)
	}
	if p.Masked() != p {
		return netip.Prefix{}, fmt.Errorf(
			"%q has host bits set; did you mean %s?", s, p.Masked())
	}
	return p, nil
}

// UsableCount returns the number of assignable addresses in a prefix.
//
// For IPv4 prefixes shorter than /31 this excludes the network and broadcast
// addresses. /31 (RFC 3021 point-to-point) and /32 are returned whole. IPv6 has
// no broadcast address and is returned whole.
func UsableCount(p netip.Prefix) *big.Int {
	total := RangeOfPrefix(p).Count()
	if p.Addr().Is4() && p.Bits() < 31 {
		return new(big.Int).Sub(total, big.NewInt(2))
	}
	return total
}

func lastAddr(p netip.Prefix) netip.Addr {
	b := addrBytes(p.Addr())
	bits := p.Bits()
	for i := range b {
		// Number of prefix bits that fall inside this byte.
		lead := bits - i*8
		switch {
		case lead >= 8:
			// fully inside the prefix, unchanged
		case lead <= 0:
			b[i] = 0xff
		default:
			b[i] |= byte(0xff >> lead)
		}
	}
	a, _ := netip.AddrFromSlice(b)
	if p.Addr().Is4() {
		return a.Unmap()
	}
	return a
}

func addrBytes(a netip.Addr) []byte {
	if a.Is4() {
		v := a.As4()
		return v[:]
	}
	v := a.As16()
	return v[:]
}
