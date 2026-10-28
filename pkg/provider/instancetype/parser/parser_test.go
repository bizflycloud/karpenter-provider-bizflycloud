package parser

import (
	"testing"

	"github.com/go-logr/logr"
)

// Create only launches the p, b, e and d tiers, so any other flavor must not
// get a category the instance type provider would offer.
func TestCategorizeFlavorExcludesUnknownTiers(t *testing.T) {
	cases := map[string]string{
		"p4a.16c_32g":    "premium",
		"b2.8c_16g":      "basic",
		"e2.4c_6g":       "enterprise",
		"d1.8c_16g":      "dedicated",
		"v2.4c_6g_60gb":  "",
		"v2.4c_8g_100gb": "",
		"p2.2c_4g_vps":   "vps",
	}
	p := NewParser(logr.Discard())
	for flavor, want := range cases {
		if got := p.CategorizeFlavor(flavor); got != want {
			t.Errorf("CategorizeFlavor(%q) = %q, want %q", flavor, got, want)
		}
	}
}
