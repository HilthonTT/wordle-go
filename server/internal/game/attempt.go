package game

import "errors"

// Attempt help record a user-attempt of a word, and the results per character.
type Attempt struct {
	Answer string
	Result []CharacterStatus
}

func (a *Attempt) ComputeResult(answer string) error {
	if len(answer) != len(a.Answer) {
		return errors.New("word length mismatch")
	}

	n := len(a.Answer)
	a.Result = make([]CharacterStatus, n)

	// Pre-build answer letter frequencies
	freq := make(map[byte]int, len(answer))
	for i := 0; i < len(answer); i++ {
		freq[answer[i]]++
	}

	// Phase 1: exact matches first — consumes their slot in freq so a
	// duplicate letter in the guess can't later claim a WrongLocation
	// that already belongs to a green.
	for i := range n {
		if a.Answer[i] == answer[i] {
			a.Result[i] = CorrectLocation
			freq[answer[i]]--
		}
	}

	// Phase 2: everything else is WrongLocation or NotPresent.
	// O(1) map lookup replaces the old O(n) numberOfFinds scan.
	for i := range n {
		if a.Result[i] == CorrectLocation {
			continue
		}
		ch := a.Answer[i]
		if freq[ch] > 0 {
			a.Result[i] = WrongLocation
			freq[ch]--
		} else {
			a.Result[i] = NotPresent
		}
	}

	return nil
}
