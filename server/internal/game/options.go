package game

import (
	"strings"

	"github.com/hilthontt/wordle-go/internal/words"
)

type Option func(w *wordle)

const (
	defaultMaxAttempts = 5
	defaultWordLength  = 5
)

var (
	wordsEnglish = words.English()
	defaultOpts  = []Option{
		WithDictionary(wordsEnglish),
		WithMaxAttempts(defaultMaxAttempts),
		WithWordFilters(WithLength(defaultWordLength)),
	}
)

func WithAnswer(answer string) Option {
	return func(w *wordle) { w.answer = answer }
}

func WithDictionary(dict []string) Option {
	return func(w *wordle) { w.dictionary = dict }
}

func WithMaxAttempts(n int) Option {
	return func(w *wordle) { w.maxAttempts = n }
}

func WithWordFilters(filters ...Filter) Option {
	return func(w *wordle) { w.wordFilters = filters }
}

func WithUnknownAnswer(wordLen int) Option {
	return func(w *wordle) {
		w.answer = strings.Repeat(" ", wordLen)
		w.answerUnknown = true
	}
}
