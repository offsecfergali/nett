package module

import (
	"fmt"
	"sort"
	"sync"
)

// Registry is a concurrency-safe catalog of module descriptors. It preserves
// registration order for stable output and offers filtered views for the CLI.
type Registry struct {
	mu    sync.RWMutex
	mods  map[string]*Descriptor
	order []string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{mods: make(map[string]*Descriptor)}
}

// Register adds a descriptor. It returns an error for an empty name, an invalid
// status, or a duplicate name.
func (r *Registry) Register(d *Descriptor) error {
	if d == nil {
		return fmt.Errorf("module: nil descriptor")
	}
	if d.Name == "" {
		return fmt.Errorf("module: descriptor has empty name")
	}
	if d.Status != StatusImplemented && d.Status != StatusPlanned {
		return fmt.Errorf("module %q: invalid status %q", d.Name, d.Status)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.mods[d.Name]; exists {
		return fmt.Errorf("module %q: already registered", d.Name)
	}
	r.mods[d.Name] = d
	r.order = append(r.order, d.Name)
	return nil
}

// MustRegister registers d and panics on error. Intended for package-init
// catalogs where a duplicate is a programming error.
func (r *Registry) MustRegister(d *Descriptor) {
	if err := r.Register(d); err != nil {
		panic(err)
	}
}

// Get returns the descriptor with the given name.
func (r *Registry) Get(name string) (*Descriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.mods[name]
	return d, ok
}

// List returns all descriptors in registration order.
func (r *Registry) List() []*Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Descriptor, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.mods[name])
	}
	return out
}

// Implemented returns only implemented descriptors, in registration order.
func (r *Registry) Implemented() []*Descriptor {
	out := make([]*Descriptor, 0)
	for _, d := range r.List() {
		if d.Implemented() {
			out = append(out, d)
		}
	}
	return out
}

// Capabilities returns the merged capability groups contributed by implemented
// modules only. Groups with the same name are merged and their items are unioned
// and sorted, so `nett capabilities` shows a clean, deduplicated tree that
// reflects reality.
func (r *Registry) Capabilities() []Capability {
	groups := make(map[string]map[string]struct{})
	var groupOrder []string

	for _, d := range r.Implemented() {
		for _, cap := range d.Capabilities {
			if _, ok := groups[cap.Group]; !ok {
				groups[cap.Group] = make(map[string]struct{})
				groupOrder = append(groupOrder, cap.Group)
			}
			for _, item := range cap.Items {
				groups[cap.Group][item] = struct{}{}
			}
		}
	}

	sort.Strings(groupOrder)
	out := make([]Capability, 0, len(groupOrder))
	for _, g := range groupOrder {
		items := make([]string, 0, len(groups[g]))
		for item := range groups[g] {
			items = append(items, item)
		}
		sort.Strings(items)
		out = append(out, Capability{Group: g, Items: items})
	}
	return out
}
