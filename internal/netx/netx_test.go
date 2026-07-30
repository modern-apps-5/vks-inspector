package netx_test

import (
	"net/netip"
	"testing"

	"github.com/modern-apps-5/vks-inspector/internal/netx"
)

func mustPrefix(t *testing.T, s string) netip.Prefix {
	t.Helper()
	p, err := netx.ParsePrefix(s)
	if err != nil {
		t.Fatalf("ParsePrefix(%q): %v", s, err)
	}
	return p
}

func mustRange(t *testing.T, start, end string) netx.Range {
	t.Helper()
	r, err := netx.ParseRange(start, end)
	if err != nil {
		t.Fatalf("ParseRange(%q,%q): %v", start, end, err)
	}
	return r
}

// The cases that produce a confidently wrong preflight verdict. Adjacency is
// the one a naive implementation always gets wrong, and getting it wrong means
// blocking a deployment over an address plan that was fine.
func TestRangeOverlaps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		aStart, aEnd string
		bStart, bEnd string
		want         bool
	}{
		{"identical ranges overlap", "10.0.0.1", "10.0.0.10", "10.0.0.1", "10.0.0.10", true},
		{"partial intersection overlaps", "10.0.0.1", "10.0.0.10", "10.0.0.5", "10.0.0.20", true},
		{"containment overlaps", "10.0.0.0", "10.0.0.255", "10.0.0.10", "10.0.0.20", true},
		{"reverse containment overlaps", "10.0.0.10", "10.0.0.20", "10.0.0.0", "10.0.0.255", true},

		// The off-by-one family.
		{"adjacent ranges do NOT overlap", "10.0.0.0", "10.0.0.255", "10.0.1.0", "10.0.1.255", false},
		{"touching at a single address overlaps", "10.0.0.0", "10.0.0.10", "10.0.0.10", "10.0.0.20", true},
		{"one apart does not overlap", "10.0.0.0", "10.0.0.10", "10.0.0.11", "10.0.0.20", false},

		{"single addresses, same", "10.0.0.5", "10.0.0.5", "10.0.0.5", "10.0.0.5", true},
		{"single addresses, different", "10.0.0.5", "10.0.0.5", "10.0.0.6", "10.0.0.6", false},
		{"disjoint", "10.0.0.0", "10.0.0.10", "192.168.0.0", "192.168.0.10", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := mustRange(t, tt.aStart, tt.aEnd)
			b := mustRange(t, tt.bStart, tt.bEnd)
			if got := a.Overlaps(b); got != tt.want {
				t.Errorf("%s.Overlaps(%s) = %v, want %v", a, b, got, tt.want)
			}
			// Overlap is symmetric. An asymmetric implementation would make
			// findings depend on config field ordering.
			if got := b.Overlaps(a); got != tt.want {
				t.Errorf("%s.Overlaps(%s) = %v, want %v (not symmetric)", b, a, got, tt.want)
			}
		})
	}
}

// Mixed address families must never be reported as colliding.
func TestRangesOfDifferentFamiliesNeverOverlap(t *testing.T) {
	t.Parallel()
	v4 := netx.RangeOfPrefix(mustPrefix(t, "0.0.0.0/0"))
	v6 := netx.RangeOfPrefix(mustPrefix(t, "::/0"))
	if v4.Overlaps(v6) || v6.Overlaps(v4) {
		t.Error("an IPv4 range must not overlap an IPv6 range")
	}
}

func TestRangeOfPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		prefix     string
		start, end string
		count      int
	}{
		{"10.0.0.0/24", "10.0.0.0", "10.0.0.255", 256},
		{"10.0.0.0/32", "10.0.0.0", "10.0.0.0", 1},
		{"10.0.0.0/31", "10.0.0.0", "10.0.0.1", 2},
		{"10.0.0.0/30", "10.0.0.0", "10.0.0.3", 4},
		{"192.168.1.0/28", "192.168.1.0", "192.168.1.15", 16},
		{"172.16.0.0/12", "172.16.0.0", "172.31.255.255", 1 << 20},
		{"0.0.0.0/0", "0.0.0.0", "255.255.255.255", 1 << 32},
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			t.Parallel()
			r := netx.RangeOfPrefix(mustPrefix(t, tt.prefix))
			if r.Start.String() != tt.start || r.End.String() != tt.end {
				t.Errorf("RangeOfPrefix(%s) = %s, want %s-%s", tt.prefix, r, tt.start, tt.end)
			}
			if got := r.Count().Int64(); got != int64(tt.count) {
				t.Errorf("Count() = %d, want %d", got, tt.count)
			}
		})
	}
}

func TestRangeOfIPv6Prefix(t *testing.T) {
	t.Parallel()
	r := netx.RangeOfPrefix(mustPrefix(t, "2001:db8::/32"))
	if r.Start.String() != "2001:db8::" {
		t.Errorf("start = %s", r.Start)
	}
	if r.End.String() != "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff" {
		t.Errorf("end = %s", r.End)
	}
	// An IPv6 /32 does not fit in an int64. Silently overflowing a sizing check
	// is exactly the kind of quiet wrongness this tool exists to avoid.
	if r.Count().IsInt64() {
		t.Error("an IPv6 /32 range should not fit in an int64")
	}
}

// A CIDR with host bits set is almost always an operator error and usually
// signals a deeper misunderstanding of the address plan. Silently masking it
// hides that.
func TestParsePrefixRejectsHostBits(t *testing.T) {
	t.Parallel()

	if _, err := netx.ParsePrefix("10.0.0.5/24"); err == nil {
		t.Error("expected 10.0.0.5/24 to be rejected")
	} else if want := "10.0.0.0/24"; !contains(err.Error(), want) {
		t.Errorf("error should suggest %s, got: %v", want, err)
	}
	if _, err := netx.ParsePrefix("10.0.0.0/24"); err != nil {
		t.Errorf("10.0.0.0/24 should parse: %v", err)
	}
	// A /32 has no host bits to set.
	if _, err := netx.ParsePrefix("10.0.0.5/32"); err != nil {
		t.Errorf("10.0.0.5/32 should parse: %v", err)
	}
}

func TestParseRangeRejectsNonsense(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, start, end string }{
		{"end before start", "10.0.0.20", "10.0.0.10"},
		{"mixed families", "10.0.0.1", "2001:db8::1"},
		{"unparseable start", "not-an-ip", "10.0.0.10"},
		{"unparseable end", "10.0.0.1", "not-an-ip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := netx.ParseRange(tt.start, tt.end); err == nil {
				t.Errorf("expected %s-%s to be rejected", tt.start, tt.end)
			}
		})
	}
	// A single-address range is legitimate, not nonsense.
	if _, err := netx.ParseRange("10.0.0.1", "10.0.0.1"); err != nil {
		t.Errorf("a single-address range should be valid: %v", err)
	}
}

func TestContainsRange(t *testing.T) {
	t.Parallel()

	subnet := netx.RangeOfPrefix(mustPrefix(t, "10.0.0.0/24"))

	tests := []struct {
		name       string
		start, end string
		want       bool
	}{
		{"wholly inside", "10.0.0.10", "10.0.0.20", true},
		{"exactly the subnet", "10.0.0.0", "10.0.0.255", true},
		{"one past the end", "10.0.0.250", "10.0.1.0", false},
		{"one before the start", "9.255.255.255", "10.0.0.10", false},
		{"entirely outside", "10.0.1.0", "10.0.1.10", false},
		{"straddles both ends", "9.0.0.0", "11.0.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := mustRange(t, tt.start, tt.end)
			if got := subnet.ContainsRange(r); got != tt.want {
				t.Errorf("10.0.0.0/24 contains %s = %v, want %v", r, got, tt.want)
			}
		})
	}
}

func TestUsableCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		prefix string
		want   int64
	}{
		// Network and broadcast are not assignable in IPv4 below /31.
		{"10.0.0.0/24", 254},
		{"10.0.0.0/30", 2},
		// RFC 3021 point-to-point: both addresses are usable.
		{"10.0.0.0/31", 2},
		{"10.0.0.0/32", 1},
		// IPv6 has no broadcast address.
		{"2001:db8::/126", 4},
	}
	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			t.Parallel()
			if got := netx.UsableCount(mustPrefix(t, tt.prefix)).Int64(); got != tt.want {
				t.Errorf("UsableCount(%s) = %d, want %d", tt.prefix, got, tt.want)
			}
		})
	}
}

func TestIntersection(t *testing.T) {
	t.Parallel()

	a := mustRange(t, "10.0.0.0", "10.0.0.100")
	b := mustRange(t, "10.0.0.50", "10.0.0.200")
	got, ok := a.Intersection(b)
	if !ok {
		t.Fatal("expected an intersection")
	}
	if got.String() != "10.0.0.50-10.0.0.100" {
		t.Errorf("intersection = %s, want 10.0.0.50-10.0.0.100", got)
	}

	if _, ok := a.Intersection(mustRange(t, "10.1.0.0", "10.1.0.10")); ok {
		t.Error("disjoint ranges should have no intersection")
	}
}

// A network's own subnet and a range carved out of it must not be reported as
// colliding — that containment is required, not a conflict.
func TestSiblingOfParent(t *testing.T) {
	t.Parallel()

	subnet := netx.Named{Source: "networks.management.cidr", Group: "networks.management",
		IsParent: true, Prefix: mustPrefix(t, "10.0.0.0/24")}
	rng := mustRange(t, "10.0.0.30", "10.0.0.34")
	child := netx.Named{Source: "networks.management.ranges[0]", Group: "networks.management", Range: &rng}
	other := netx.Named{Source: "kubernetes.serviceCIDR", Prefix: mustPrefix(t, "10.0.0.0/25")}

	if !subnet.SiblingOfParent(child) || !child.SiblingOfParent(subnet) {
		t.Error("a range and its own parent subnet should be recognised as siblings")
	}
	if subnet.SiblingOfParent(other) {
		t.Error("an unrelated network must not be treated as a sibling")
	}
	// Two ranges within the same network are NOT parent/child and a collision
	// between them is a real finding.
	rng2 := mustRange(t, "10.0.0.32", "10.0.0.40")
	child2 := netx.Named{Source: "networks.management.ranges[1]", Group: "networks.management", Range: &rng2}
	if child.SiblingOfParent(child2) {
		t.Error("two sibling ranges must still be compared against each other")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// "They overlap" is true of every intersecting pair and useless to act on.
// Containment and a partial overlap have different causes and different fixes.
func TestRelate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b string
		want netx.Relation
	}{
		{"identical", "10.0.0.0/24", "10.0.0.0/24", netx.RelIdentical},
		{"a contains b", "10.0.0.0/16", "10.0.1.0/24", netx.RelContains},
		{"b contains a", "10.0.1.0/24", "10.0.0.0/16", netx.RelContainedBy},
		// The reported case: same network address, different prefix length.
		// A naive check calls this a partial overlap; it is containment.
		{"same base, shorter prefix contains longer", "10.96.0.0/23", "10.96.0.0/22", netx.RelContainedBy},
		{"partial", "10.0.0.0-10.0.0.100", "10.0.0.50-10.0.0.200", netx.RelPartial},
		{"disjoint", "10.0.0.0/24", "10.0.1.0/24", netx.RelDisjoint},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := asRange(t, tt.a).Relate(asRange(t, tt.b))
			if got != tt.want {
				t.Errorf("Relate(%s, %s) = %s, want %s", tt.a, tt.b, got, tt.want)
			}
			if got.Describe() == "" {
				t.Error("relation has no description")
			}
		})
	}
}

func asRange(t *testing.T, s string) netx.Range {
	t.Helper()
	if i := indexOf(s, "-"); i >= 0 {
		return mustRange(t, s[:i], s[i+1:])
	}
	return netx.RangeOfPrefix(mustPrefix(t, s))
}
