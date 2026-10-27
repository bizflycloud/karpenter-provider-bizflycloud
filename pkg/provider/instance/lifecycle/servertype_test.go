package lifecycle

import "testing"

func TestServerTypeFromFlavor(t *testing.T) {
	cases := map[string]string{
		"p2.16c_32g":  "premium",
		"p4a.24c_64g": "premium",
		"e2.16c_32g":  "enterprise",
		"b1.2c_4g":    "basic",
		"d1.8c_16g":   "dedicated",
		"x1.2c_4g":    "",
		"":            "",
	}
	for flavor, want := range cases {
		if got := serverTypeFromFlavor(flavor); got != want {
			t.Errorf("serverTypeFromFlavor(%q) = %q, want %q", flavor, got, want)
		}
	}
}
