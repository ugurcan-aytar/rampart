package domain

import (
	"errors"
	"fmt"
	"strings"
)

// Component mirrors a Backstage Component entity. rampart only needs the
// reference tuple (kind, namespace, name), an owner, and arbitrary annotations —
// rich Backstage metadata stays in the catalog.
type Component struct {
	Ref         string
	Kind        string
	Namespace   string
	Name        string
	Owner       string
	System      string
	Lifecycle   string
	Tags        []string
	Annotations map[string]string
}

var ErrInvalidComponentRef = errors.New("invalid component ref")

// ParseComponentRef splits "kind:Component/default/web-app" into its three parts.
func ParseComponentRef(ref string) (kind, namespace, name string, err error) {
	head, tail, ok := strings.Cut(ref, ":")
	if !ok || head != "kind" {
		return "", "", "", fmt.Errorf("%w: missing 'kind:' prefix in %q", ErrInvalidComponentRef, ref)
	}
	parts := strings.Split(tail, "/")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("%w: expected kind/namespace/name, got %q", ErrInvalidComponentRef, tail)
	}
	if parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("%w: empty segment in %q", ErrInvalidComponentRef, ref)
	}
	return parts[0], parts[1], parts[2], nil
}

// NewComponent builds a Component from a ref and an owner.
func NewComponent(ref, owner string) (*Component, error) {
	kind, ns, name, err := ParseComponentRef(ref)
	if err != nil {
		return nil, err
	}
	return &Component{
		Ref:       ref,
		Kind:      kind,
		Namespace: ns,
		Name:      name,
		Owner:     owner,
	}, nil
}
