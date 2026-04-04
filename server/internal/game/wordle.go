package game

import (
	"fmt"
	"math/rand"
)

type Wordle interface {
	Alphabets() map[string]CharacterStatus
	Answer() string
	AnswerUnknown() bool
	Attempt(word string, result ...CharacterStatus) (*Attempt, error)
	Attempts() []Attempt
	DecrementMaxAttempts() bool
	Dictionary() []string
	DictionaryHas(word string) bool
	GameOver() bool
	Hints() []string
	IncrementMaxAttempts()
	Reset() error
	Solved() bool
}

type wordle struct {
	alphabets     map[string]CharacterStatus
	answer        string
	answerUnknown bool
	attempts      []Attempt
	dictionary    []string
	dictionarySet map[string]struct{}
	maxAttempts   int
	options       []Option
	solved        bool
	wordFilters   Filters
	wordsAllowed  []string
}

func New(opts ...Option) (Wordle, error) {
	w := &wordle{}
	w.options = append(defaultOpts, opts...)

	if err := w.init(); err != nil {
		return nil, err
	}

	return w, nil
}

func (w *wordle) Alphabets() map[string]CharacterStatus {
	return w.alphabets
}

func (w *wordle) Answer() string {
	return w.answer
}

func (w *wordle) AnswerUnknown() bool {
	return w.answerUnknown
}

func (w *wordle) Attempts() []Attempt {
	return w.attempts
}

func (w *wordle) Solved() bool {
	return w.solved
}

func (w *wordle) Dictionary() []string {
	return w.dictionary
}

func (w *wordle) DictionaryHas(word string) bool {
	_, ok := w.dictionarySet[word]
	return ok
}

func (w *wordle) Hints() []string {
	if w.solved {
		return nil
	}
	return generateHints(w.wordsAllowed, w.attempts, w.alphabets)
}

func (w *wordle) Attempt(word string, result ...CharacterStatus) (*Attempt, error) {
	if w.GameOver() {
		return &w.attempts[len(w.attempts)-1], nil
	}

	var attempt *Attempt
	var err error

	if w.answerUnknown {
		attempt, err = w.attemptUnknown(word, result)
	} else {
		attempt, err = w.attemptKnown(word)
	}

	if err != nil {
		return nil, err
	}

	w.attempts = append(w.attempts, *attempt)
	return attempt, nil
}

func (w *wordle) GameOver() bool {
	return w.Solved() || len(w.attempts) == w.maxAttempts
}

func (w *wordle) DecrementMaxAttempts() bool {
	if len(w.attempts) >= w.maxAttempts-1 || w.maxAttempts == 1 {
		return false
	}
	w.maxAttempts--
	return true
}

func (w *wordle) IncrementMaxAttempts() {
	w.maxAttempts++
}

func (w *wordle) Reset() error {
	return w.init()
}

func (w *wordle) attemptKnown(word string) (*Attempt, error) {
	if err := w.validateAttempt(word); err != nil {
		return nil, err
	}
	attempt := computeAttempt(w.alphabets, w.answer, word)
	if attempt.Answer == w.answer {
		w.solved = true
	}
	return attempt, nil
}

func (w *wordle) attemptUnknown(word string, result []CharacterStatus) (*Attempt, error) {
	if err := w.validateAttemptUnknown(word); err != nil {
		return nil, err
	}

	attempt := &Attempt{Answer: word, Result: result}

	// First pass: record present letters.
	for idx, status := range attempt.Result {
		charStr := string(word[idx])
		if status != NotPresent {
			w.alphabets[charStr] = status
		}
	}
	// Second pass: remove absent letters, but never downgrade a confirmed letter.
	for idx, status := range attempt.Result {
		charStr := string(word[idx])
		if status == NotPresent {
			if w.alphabets[charStr] != WrongLocation && w.alphabets[charStr] != CorrectLocation {
				delete(w.alphabets, charStr)
			}
		}
	}

	numCorrect := 0
	for _, status := range attempt.Result {
		if status == CorrectLocation {
			numCorrect++
		}
	}
	w.solved = numCorrect == len(attempt.Result)

	return attempt, nil
}

func (w *wordle) validateAttempt(word string) error {
	if len(word) != len(w.answer) {
		return fmt.Errorf("word length [%d] does not match answer length [%d]", len(word), len(w.answer))
	}

	for _, attempt := range w.attempts {
		if word == attempt.Answer {
			return fmt.Errorf("word [%s] has been attempted already", word)
		}
	}

	if !w.DictionaryHas(word) {
		return fmt.Errorf("not a valid word: %q", word)
	}

	return nil
}

func (w *wordle) validateAttemptUnknown(word string) error {
	if len(word) != len(w.answer) {
		return fmt.Errorf("word length [%d] does not match answer length [%d]", len(word), len(w.answer))
	}
	for _, attempt := range w.attempts {
		if word == attempt.Answer {
			return fmt.Errorf("word [%s] has been attempted already", word)
		}
	}
	return nil
}

func computeAttempt(alphabets map[string]CharacterStatus, answer, word string) *Attempt {
	attempt := &Attempt{Answer: word}
	attempt.ComputeResult(answer)

	for idx, char := range attempt.Answer {
		charStr := string(char)
		switch attempt.Result[idx] {
		case NotPresent:
			if alphabets[charStr] == Unknown {
				alphabets[charStr] = NotPresent
			}
		case WrongLocation:
			if alphabets[charStr] != CorrectLocation {
				alphabets[charStr] = WrongLocation
			}
		case CorrectLocation:
			alphabets[charStr] = CorrectLocation
		}
	}
	return attempt
}

func (w *wordle) init() error {
	// Clear the answer before re-applying options so Reset() picks a new
	// random word instead of reusing the previous one.
	w.answer = ""

	for _, opt := range w.options {
		opt(w)
	}

	w.wordsAllowed = w.wordFilters.Apply(w.dictionary)
	if len(w.wordsAllowed) == 0 {
		return fmt.Errorf("found no words to choose from after applying all filters")
	}

	w.dictionarySet = make(map[string]struct{}, len(w.dictionary))
	for _, word := range w.dictionary {
		w.dictionarySet[word] = struct{}{}
	}

	w.alphabets = make(map[string]CharacterStatus, 26)
	for _, r := range englishAlphabets {
		w.alphabets[string(r)] = Unknown
	}

	if !w.answerUnknown && w.answer == "" {
		w.answer = w.wordsAllowed[rand.Intn(len(w.wordsAllowed))]
	}

	w.attempts = make([]Attempt, 0, w.maxAttempts)
	w.solved = false

	return nil
}
