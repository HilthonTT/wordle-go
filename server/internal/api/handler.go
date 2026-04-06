package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/hilthontt/wordle-go/internal/game"
	"github.com/hilthontt/wordle-go/internal/router"
	"github.com/hilthontt/wordle-go/internal/store"
)

// Handler holds the dependencies for all HTTP handlers.
type Handler struct {
	store *store.Store
}

func NewHandler(s *store.Store) *Handler {
	return &Handler{store: s}
}

// POST /games
func (h *Handler) CreateGame(w http.ResponseWriter, r *http.Request) {
	var req createGameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	wordLen := req.WordLength
	if wordLen <= 0 {
		wordLen = 5
	}
	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 6
	}

	opts := []game.Option{
		game.WithMaxAttempts(maxAttempts),
	}

	switch {
	case req.UnknownAnswer:
		opts = append(opts, game.WithUnknownAnswer(wordLen))
	case req.Answer != "":
		if len(req.Answer) != wordLen {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("answer length %d does not match word_length %d", len(req.Answer), wordLen))
			return
		}
		opts = append(opts,
			game.WithWordFilters(game.WithLength(wordLen)),
			game.WithAnswer(req.Answer),
		)
	default:
		opts = append(opts, game.WithWordFilters(game.WithLength(wordLen)))
	}

	g, err := game.New(opts...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create game: "+err.Error())
		return
	}

	id := uuid.New().String()
	sess := h.store.Create(id, g, wordLen, maxAttempts)
	writeJSON(w, http.StatusCreated, toGameResponse(sess))
}

// GET /games
func (h *Handler) ListGames(w http.ResponseWriter, r *http.Request) {
	sessions := h.store.List()
	resp := make([]gameResponse, len(sessions))
	for i, sess := range sessions {
		resp[i] = toGameResponse(sess)
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": resp, "count": len(resp)})
}

// GET /games/:id
func (h *Handler) GetGame(w http.ResponseWriter, r *http.Request) {
	id := router.Params(r)["id"]
	sess, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toGameResponse(sess))
}

// GET /games/:id/validate?word=crane
//
// Checks whether a word is the right length and exists in the dictionary
// without consuming an attempt. Lets the frontend give inline feedback
// before the player submits.
func (h *Handler) ValidateWord(w http.ResponseWriter, r *http.Request) {
	id := router.Params(r)["id"]
	sess, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	word := r.URL.Query().Get("word")
	if word == "" {
		writeError(w, http.StatusBadRequest, "word query parameter is required")
		return
	}

	type validationResponse struct {
		Word   string `json:"word"`
		Valid  bool   `json:"valid"`
		Reason string `json:"reason,omitempty"`
	}

	resp := validationResponse{Word: word}

	switch {
	case len(word) != sess.WordLength:
		resp.Valid = false
		resp.Reason = fmt.Sprintf("word must be %d letters", sess.WordLength)
	case !sess.Game.DictionaryHas(word):
		resp.Valid = false
		resp.Reason = "not in word list"
	default:
		resp.Valid = true
	}

	writeJSON(w, http.StatusOK, resp)
}

// GET /games/:id/hints
func (h *Handler) GetHints(w http.ResponseWriter, r *http.Request) {
	id := router.Params(r)["id"]
	sess, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if sess.Game.GameOver() {
		writeError(w, http.StatusConflict, "game is already over")
		return
	}

	hints := sess.Game.Hints()
	if hints == nil {
		hints = []string{}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"hints": hints})
}

// POST /games/:id/attempts
func (h *Handler) SubmitAttempt(w http.ResponseWriter, r *http.Request) {
	id := router.Params(r)["id"]
	sess, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if sess.Game.GameOver() {
		writeError(w, http.StatusConflict, "game is already over")
		return
	}

	var req attemptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.Word == "" {
		writeError(w, http.StatusBadRequest, "word is required")
		return
	}

	var attempt *game.Attempt
	if sess.Game.AnswerUnknown() {
		if len(req.Result) == 0 {
			writeError(w, http.StatusBadRequest, `result is required in unknown-answer mode — use ["absent","present","correct",...]`)
			return
		}
		if len(req.Result) != len(req.Word) {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("result length %d must equal word length %d", len(req.Result), len(req.Word)))
			return
		}
		statuses, err := parseResultStrings(req.Result)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		attempt, err = sess.Game.Attempt(req.Word, statuses...)
	} else {
		attempt, err = sess.Game.Attempt(req.Word)
	}

	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	h.store.Touch(id)

	writeJSON(w, http.StatusOK, map[string]any{
		"attempt": toAttemptResponse(*attempt),
		"game":    toGameResponse(sess),
	})
}

// POST /games/:id/reset
func (h *Handler) ResetGame(w http.ResponseWriter, r *http.Request) {
	id := router.Params(r)["id"]
	sess, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err := sess.Game.Reset(); err != nil {
		writeError(w, http.StatusInternalServerError, "reset failed: "+err.Error())
		return
	}
	h.store.Touch(id)
	writeJSON(w, http.StatusOK, toGameResponse(sess))
}

// DELETE /games/:id
func (h *Handler) DeleteGame(w http.ResponseWriter, r *http.Request) {
	id := router.Params(r)["id"]
	if err := h.store.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
