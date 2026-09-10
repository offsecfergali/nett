// Package module defines the nett module catalog: a registry of every module
// the framework knows about, each tagged with its implementation status and the
// capabilities it provides. The CLI uses this to power `nett modules` and
// `nett capabilities`.
//
// Honesty is enforced structurally: `nett capabilities` lists capabilities
// only from modules whose Status is StatusImplemented, so the tool never
// advertises a capability that does not yet exist. Planned modules are still
// listed by `nett modules` (with their milestone and a "planned" marker) so a
// user can see the roadmap, but they are never counted as working capabilities.
package module

// Status describes whether a module's code actually exists and is tested.
type Status string

const (
	// StatusImplemented means the module is fully implemented and tested.
	StatusImplemented Status = "implemented"
	// StatusPlanned means the module is designed but not yet implemented.
	StatusPlanned Status = "planned"
)

// Capability is a named group of concrete features, e.g. group "DNS" with items
// {"A","AAAA","CNAME",...}. A capability is only surfaced by `nett
// capabilities` when the module providing it is StatusImplemented.
type Capability struct {
	Group string
	Items []string
}

// Descriptor is the catalog entry for a single module.
type Descriptor struct {
	// Name is the stable identifier used on the CLI (e.g. "dns").
	Name string
	// Short is a one-line description.
	Short string
	// Milestone is the milestone that delivers the module (e.g. "M4").
	Milestone string
	// Status is implemented or planned.
	Status Status
	// Active is true when the module makes network requests and therefore MUST
	// consult the scope engine before contacting any target.
	Active bool
	// Capabilities are the concrete features the module provides.
	Capabilities []Capability
}

// Implemented reports whether the descriptor is fully implemented.
func (d *Descriptor) Implemented() bool { return d.Status == StatusImplemented }
