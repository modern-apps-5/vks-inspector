package prompt

import "testing"

// The normalisers decide how much typing friction an operator meets. Accepting
// what someone naturally types, and rejecting only what is genuinely ambiguous,
// is the line these cases pin.
func TestNormCIDR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{name: "a plain network", in: "10.0.0.0/8", want: "10.0.0.0/8"},
		{name: "whitespace is trimmed", in: "  172.16.0.0/12  ", want: "172.16.0.0/12"},

		// A bare address means "this one host". Making someone type /32 to say
		// that is friction with nothing to show for it — and for a field like
		// "networks this must not collide with", protecting a single address is
		// an ordinary intent.
		{name: "a bare IPv4 address becomes a /32", in: "192.168.200.5", want: "192.168.200.5/32"},
		{name: "a bare IPv6 address becomes a /128", in: "2001:db8::1", want: "2001:db8::1/128"},
		{name: "an explicit /32 is unchanged", in: "192.168.200.5/32", want: "192.168.200.5/32"},

		// Genuinely ambiguous: the whole /24, or that one host? Guessing at an
		// address plan is exactly what this tool exists not to do. The error
		// must name the masked form so the fix is one edit away.
		{name: "host bits set is rejected", in: "192.168.200.5/24", wantErr: "192.168.200.0/24"},
		{name: "host bits set, larger prefix", in: "1.1.1.1/24", wantErr: "1.1.1.0/24"},

		{name: "nonsense is rejected", in: "not-an-address", wantErr: "not a valid CIDR"},
		{name: "empty is rejected", in: "", wantErr: "not a valid CIDR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normCIDR(tt.in)
			switch {
			case tt.wantErr != "":
				if err == nil {
					t.Fatalf("normCIDR(%q) = %q, want an error containing %q", tt.in, got, tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not mention %q", err, tt.wantErr)
				}
			case err != nil:
				t.Fatalf("normCIDR(%q): %v", tt.in, err)
			case got != tt.want:
				t.Errorf("normCIDR(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Networks the deployment will sit on cannot be a single host, so the strict
// form refuses a bare address rather than silently turning it into a /32.
func TestNormStrictCIDRRefusesBareAddress(t *testing.T) {
	t.Parallel()

	if _, err := normStrictCIDR("192.168.200.5"); err == nil {
		t.Error("a bare address should not be accepted where a network is required")
	}
	if got, err := normStrictCIDR("192.168.200.0/24"); err != nil || got != "192.168.200.0/24" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestNormHost(t *testing.T) {
	t.Parallel()

	for _, ok := range []string{"vcenter.corp.local", "10.0.0.1", "vc01"} {
		if _, err := normHost(ok); err != nil {
			t.Errorf("normHost(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{"", "  ", "https://vc.corp.local", "vc corp local"} {
		if _, err := normHost(bad); err == nil {
			t.Errorf("normHost(%q) should have been rejected", bad)
		}
	}
}

func TestNormPositiveInt(t *testing.T) {
	t.Parallel()

	if got, err := normPositiveInt(" 1700 "); err != nil || got != "1700" {
		t.Errorf("got %q, %v", got, err)
	}
	for _, bad := range []string{"0", "-1", "abc", "1.5", ""} {
		if _, err := normPositiveInt(bad); err == nil {
			t.Errorf("normPositiveInt(%q) should have been rejected", bad)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
