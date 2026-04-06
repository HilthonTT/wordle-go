package api

import (
	"fmt"

	"github.com/hilthontt/wordle-go/internal/game"
	"github.com/hilthontt/wordle-go/internal/store"
)

func characterStatusString(s game.CharacterStatus) string {
	switch s {
	case game.NotPresent:
		return "absent"
	case game.WrongLocation:
		return "present"
	case game.CorrectLocation:
		return "correct"
	default:
		return "unknown"
	}
}

func toGameResponse(sess *store.Session) gameResponse {
	g := sess.Game

	attempts := make([]attemptResponse, len(g.Attempts()))
	for i, a := range g.Attempts() {
		attempts[i] = toAttemptResponse(a)
	}

	// Convert alphabet map from int enum → string status.
	alphabets := make(map[string]alphabetEntry, 26)
	for letter, status := range g.Alphabets() {
		alphabets[letter] = alphabetEntry{Status: characterStatusString(status)}
	}

	attemptsRemaining := sess.MaxAttempts - len(g.Attempts())
	if attemptsRemaining < 0 {
		attemptsRemaining = 0
	}

	resp := gameResponse{
		ID:                sess.ID,
		WordLength:        sess.WordLength,
		MaxAttempts:       sess.MaxAttempts,
		AttemptsRemaining: attemptsRemaining,
		Alphabets:         alphabets,
		Attempts:          attempts,
		Solved:            g.Solved(),
		GameOver:          g.GameOver(),
		CreatedAt:         sess.CreatedAt,
		UpdatedAt:         sess.UpdatedAt,
	}

	if g.GameOver() && !g.AnswerUnknown() {
		resp.Answer = g.Answer()
	}

	return resp
}

func parseResultStrings(strs []string) ([]game.CharacterStatus, error) {
	out := make([]game.CharacterStatus, len(strs))
	for i, s := range strs {
		switch s {
		case "absent":
			out[i] = game.NotPresent
		case "present":
			out[i] = game.WrongLocation
		case "correct":
			out[i] = game.CorrectLocation
		default:
			return nil, fmt.Errorf("invalid result value %q at index %d: must be \"absent\", \"present\", or \"correct\"", s, i)
		}
	}
	return out, nil
}

func toAttemptResponse(a game.Attempt) attemptResponse {
	tiles := make([]tileResponse, len(a.Answer))
	for i := 0; i < len(a.Answer); i++ {
		tiles[i] = tileResponse{
			Letter: string(a.Answer[i]),
			Status: characterStatusString(a.Result[i]),
		}
	}
	return attemptResponse{Answer: a.Answer, Tiles: tiles}
}
