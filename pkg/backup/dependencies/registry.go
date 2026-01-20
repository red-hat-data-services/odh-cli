package dependencies

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Registry holds all registered dependency resolvers.
type Registry struct {
	resolvers []Resolver
}

//nolint:gochecknoglobals
var globalRegistry = &Registry{}

// NewRegistry creates a new resolver registry with all built-in resolvers.
func NewRegistry() *Registry {
	return globalRegistry
}

// Register adds a resolver to the registry.
func (r *Registry) Register(resolver Resolver) {
	r.resolvers = append(r.resolvers, resolver)
}

// MustRegister registers a resolver and panics if registration fails.
// This is intended for use in init() functions.
func MustRegister(resolver Resolver) {
	globalRegistry.Register(resolver)
}

// GetResolver finds the appropriate resolver for the given GVR.
func (r *Registry) GetResolver(gvr schema.GroupVersionResource) (Resolver, error) {
	for _, resolver := range r.resolvers {
		if resolver.CanHandle(gvr) {
			return resolver, nil
		}
	}

	return nil, fmt.Errorf("no dependency resolver registered for %s", gvr.String())
}
