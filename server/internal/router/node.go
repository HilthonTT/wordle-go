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

// Insert adds a route to the radix tree
func (n *Node) insert(segments []string, method string, handler http.HandlerFunc, index int) {
	// If all segments are processed, store the handler
	if index >= len(segments) {
		n.handlers[method] = handler
		return
	}

	segment := segments[index]
	isParam := len(segment) > 0 && segment[0] == ':'

	if isParam {
		segment = segment[1:] // Remove the ':'
	}

	// Look for an existing child that matches
	for _, child := range n.children {
		if child.isParam == isParam {
			if isParam || child.part == segment {
				child.insert(segments, method, handler, index+1)
				return
			}

			// Check for common prefix in static nodes
			if !isParam {
				i := 0

				for i < len(segment) && i < len(child.part) && segment[i] == child.part[i] {
					i++
				}

				if i > 0 {
					// We have a common prefix, split the node
					commonPrefix := child.part[:i]
					childSuffix := child.part[i:]
					segmentSuffix := segment[i:]

					// Create a new intermediate node
					intermediateNode := &Node{
						part:     commonPrefix,
						isParam:  false,
						children: []*Node{child},
						handlers: make(map[string]http.HandlerFunc),
					}

					// Update the existing child
					child.part = childSuffix

					// Replace the child with the intermediate node
					for i, c := range n.children {
						if c == child {
							n.children[i] = intermediateNode
							break
						}
					}

					// If there's more to the segment, create a new node
					if len(segmentSuffix) > 0 {
						newNode := &Node{
							part:     segmentSuffix,
							isParam:  false,
							children: []*Node{},
							handlers: make(map[string]http.HandlerFunc),
						}
						intermediateNode.children = append(intermediateNode.children, newNode)
						newNode.insert(segments, method, handler, index+1)
					} else {
						// The entire segment has been consumed, store handler in the intermediate node
						intermediateNode.insert(segments, method, handler, index+1)
					}
					return
				}
			}
		}
	}

	isWildcard := segment == "*"
	newNode := &Node{
		part:       segment,
		isParam:    isParam,
		children:   []*Node{},
		handlers:   make(map[string]http.HandlerFunc),
		isWildcard: isWildcard,
	}

	n.insertChildNode(newNode)
	newNode.insert(segments, method, handler, index+1)
}
