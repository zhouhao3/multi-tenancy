package forest

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
)

// While storing the V in GVK is not strictly necessary to match what's in the HNC type configuration,
// as a client of the API server, HNC will be to be reading and writing versions of the API to communicate
// with the API server. Since we need the V to work with the API server anyways anyways, we will choose to
// use the GVK as the key in this map.
type objects map[schema.GroupVersionKind]map[string]*unstructured.Unstructured

// Namespace represents a namespace in a forest. Other than its structure, it contains some
// properties useful to the reconcilers.
type Namespace struct {
	forest                 *Forest
	name                   string
	parent                 *Namespace
	children               namedNamespaces
	exists                 bool
	allowCascadingDeletion bool

	// labels store the original namespaces' labels
	labels map[string]string

	// sourceObjects store the objects created by users, identified by GVK and name.
	// It serves as the source of truth for object controllers to propagate objects.
	sourceObjects objects

	// conditions store conditions so that object propagation can be disabled if there's a problem
	// on this namespace.
	conditions []api.Condition

	// IsSub indicates that this namespace is being or was created solely to live as a
	// subnamespace of the specified parent.
	IsSub bool

	// Anchors store a list of anchors in the namespace.
	Anchors []string

	// Manager stores the manager of the namespace. The default value
	// "hnc.x-k8s.io" means it's managed by HNC.
	Manager string

	// ExternalTreeLabels stores external tree labels if this namespace is an external namespace.
	// It will be empty if the namespace is not external. External namespace will at least have one
	// tree label of itself. The key is the tree label without ".tree.hnc.x-k8s.io/depth" suffix.
	// The value is the depth.
	ExternalTreeLabels map[string]int
}

// Name returns the name of the namespace, of "<none>" if the namespace is nil.
func (ns *Namespace) Name() string {
	if ns == nil {
		return "<none>"
	}
	return ns.name
}

// Parent returns a pointer to the parent namespace.
func (ns *Namespace) Parent() *Namespace {
	return ns.parent
}

// Exists returns true if the namespace exists.
func (ns *Namespace) Exists() bool {
	return ns.exists
}

// SetExists marks this namespace as existing, returning true if didn't previously exist.
func (ns *Namespace) SetExists() bool {
	changed := !ns.exists
	ns.exists = true
	return changed
}

// UnsetExists marks this namespace as missing, returning true if it previously existed. It also
// removes it from its parent, if any, since a nonexistent namespace can't have a parent.
func (ns *Namespace) UnsetExists() bool {
	changed := ns.exists
	ns.SetParent(nil) // Unreconciled namespaces can't specify parents
	ns.exists = false
	ns.clean() // clean up if this is a useless data structure
	return changed
}

func (ns *Namespace) GetLabels() labels.Set {
	return labels.Set(ns.labels)
}

// Deep copy the input labels so that it'll not be changed after
func (ns *Namespace) SetLabels(labels map[string]string) {
	ns.labels = make(map[string]string)
	for key, val := range labels {
		ns.labels[key] = val
	}
}

// clean garbage collects this namespace if it has a zero value.
func (ns *Namespace) clean() {
	// Don't clean up something that either exists or is otherwise referenced.
	if ns.exists || len(ns.children) > 0 {
		return
	}

	// Remove from the forest.
	delete(ns.forest.namespaces, ns.name)
}

// UpdateAllowCascadingDeletion updates if this namespace allows cascading deletion. It returns true
// if the value has changed, false otherwise.
func (ns *Namespace) UpdateAllowCascadingDeletion(acd bool) bool {
	if ns.allowCascadingDeletion == acd {
		return false
	}
	ns.allowCascadingDeletion = acd
	return true
}

// AllowsCascadingDeletion returns true if the namespace's or any of the ancestors'
// allowCascadingDeletion field is set to true.
func (ns *Namespace) AllowsCascadingDeletion() bool {
	if ns.allowCascadingDeletion == true {
		return true
	}
	if ns.parent == nil || ns.CycleNames() != nil {
		return false
	}

	// This namespace is neither a root nor in a cycle, so this line can't cause a stack overflow.
	return ns.parent.AllowsCascadingDeletion()
}

// SetAnchors updates the anchors and returns a difference between the new/old list.
func (ns *Namespace) SetAnchors(anchors []string) (diff []string) {
	add := make(map[string]bool)
	for _, nm := range anchors {
		add[nm] = true
	}
	for _, nm := range ns.Anchors {
		if add[nm] {
			delete(add, nm)
		} else {
			// This old anchor is not in the new anchor list.
			diff = append(diff, nm)
		}
	}

	for nm, _ := range add {
		// This new anchor is not in the old anchor list.
		diff = append(diff, nm)
	}

	ns.Anchors = anchors
	return
}

// HasAnchor returns true if the name exists in the anchor list.
func (ns *Namespace) HasAnchor(n string) bool {
	for _, a := range ns.Anchors {
		if a == n {
			return true
		}
	}
	return false
}

// IsExternal returns true if the namespace is not managed by HNC.
func (ns *Namespace) IsExternal() bool {
	return len(ns.ExternalTreeLabels) > 0
}
