package game

// Filter returns true if the word passes through the filter. Returns false for
// words that do not qualify.
type Filter func(word string) bool

// WithLength helps filter the words by length
func WithLength(wordLen int) Filter {
	return func(word string) bool {
		return len(word) == wordLen
	}
}

// WithNoRepeatingCharacters prevents words from having repeated characters.
func WithNoRepeatingCharacters() Filter {
	return func(word string) bool {
		var seen [26]bool // zero-alloc
		for _, r := range word {
			idx := r - 'a'
			if seen[idx] {
				return false
			}
			seen[idx] = true
		}
		return true
	}
}

// Filters is a list of filters.
type Filters []Filter

// Apply returns the list of words allowed by all filters.
func (f Filters) Apply(words []string) []string {
	result := make([]string, 0, len(words))
	for _, word := range words {
		if f.allow(word) {
			result = append(result, word)
		}
	}
	return result
}

// allow extracts the inner filter loop, removing the dnq flag and reducing nesting in Apply.
func (f Filters) allow(word string) bool {
	for _, filter := range f {
		if !filter(word) {
			return false
		}
	}
	return true
}
