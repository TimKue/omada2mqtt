// Package dnssync holds the pure logic for naming Omada clients from an
// external source of truth (OPNsense DHCP leases): normalising MACs, building
// the desired MAC->name map, and computing the set of renames. It has no I/O,
// so it is straightforward to unit-test.
package dnssync

import (
	"strings"

	"github.com/TimKue/omada2mqtt/pkg/omada"
	"github.com/TimKue/omada2mqtt/pkg/opnsense"
)

// Change is one planned client rename.
type Change struct {
	SiteID string
	Mac    string // Omada MAC format (uppercase, dashed), used for the write
	Old    string
	New    string
}

// NormMAC lowercases a MAC and strips ":"/"-" so the OPNsense and Omada formats
// compare equal (e.g. "d4:d6:df:0a:63:29" == "D4-D6-DF-0A-63-29").
func NormMAC(mac string) string {
	return strings.ToLower(strings.NewReplacer(":", "", "-", "").Replace(mac))
}

// DesiredNames builds a normalised-MAC -> hostname map from OPNsense leases.
// Leases without a real hostname are skipped: an empty string, or the Dnsmasq
// "*" placeholder, both mean "no name" and must never overwrite an Omada name.
func DesiredNames(leases []opnsense.Lease) map[string]string {
	want := make(map[string]string, len(leases))
	for _, l := range leases {
		if h := strings.TrimSpace(l.Hostname); h != "" && h != "*" {
			want[NormMAC(l.HWAddr)] = h
		}
	}
	return want
}

// Plan returns the renames needed for one site's clients: every client whose
// MAC has a desired name that differs from its current Omada name. Clients with
// no desired name are left untouched.
func Plan(siteID string, clients []omada.ClientInfo, want map[string]string) []Change {
	var plan []Change
	for _, cl := range clients {
		desired, ok := want[NormMAC(cl.Mac)]
		if !ok || desired == cl.Name {
			continue
		}
		plan = append(plan, Change{SiteID: siteID, Mac: cl.Mac, Old: cl.Name, New: desired})
	}
	return plan
}
