package updater

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.2.0", "0.1.0", true},
		{"v0.10.0", "0.9.9", true},
		{"1.0.0", "1.0.0", false},
		{"0.1.9", "0.2.0", false},
		{"0.2.0", "dev", true},
		{"dev", "0.1.0", false},
		{"0.3.0-beta.1", "0.2.0", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
