package router

import "net/http"

// Node represents a node in the trie
type Node struct {
	// Part is the path segment this node represents
	part string

	// IsParam indicates if this node is a path parameter (like :id)
	isParam bool

	// IsWildCard indicates if this node is a wild card
	isWildcard bool

	// Children contains child nodes
	children []*Node

	// Handlers stores handler funcs for different HTTP methods
	handlers map[string]http.HandlerFunc
}

// insertChildNode inserts a child node in sorted order
func (n *Node) insertChildNode(child *Node) {
	for i, existing := range n.children {
		// Static nodes before parameter nodes
		if existing.isParam && !child.isParam {
			n.children = append(n.children, nil)
			copy(n.children[i+1:], n.children[i:])
			n.children[i] = child
			return
		}
		// Sort alphabetically for faster lookups
		if !existing.isParam && !child.isParam && existing.part > child.part {
			n.children = append(n.children, nil)
			copy(n.children[i+1:], n.children[i:])
			n.children[i] = child
			return
		}
	}

	// Append if we didn't insert
	n.children = append(n.children, child)
}
