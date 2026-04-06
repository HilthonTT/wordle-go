package api

import "time"

type createGameRequest struct {
	WordLength    int    `json:"word_length"`
	MaxAttempts   int    `json:"max_attempts"`
	Answer        string `json:"answer,omitempty"`
	UnknownAnswer bool   `json:"unknown_answer"`
}

type attemptRequest struct {
	Word   string   `json:"word"`
	Result []string `json:"result,omitempty"`
}

type attemptResponse struct {
	Answer string         `json:"answer"`
	Tiles  []tileResponse `json:"tiles"`
}

type gameResponse struct {
	ID                string                   `json:"id"`
	WordLength        int                      `json:"word_length"`
	MaxAttempts       int                      `json:"max_attempts"`
	AttemptsRemaining int                      `json:"attempts_remaining"`
	Alphabets         map[string]alphabetEntry `json:"alphabets"`
	Attempts          []attemptResponse        `json:"attempts"`
	Solved            bool                     `json:"solved"`
	GameOver          bool                     `json:"game_over"`
	Answer            string                   `json:"answer,omitempty"` // revealed only on game-over
	CreatedAt         time.Time                `json:"created_at"`
	UpdatedAt         time.Time                `json:"updated_at"`
}
type alphabetEntry struct {
	Status string `json:"status"`
}

type tileResponse struct {
	Letter string `json:"letter"`
	Status string `json:"status"` // "absent" | "present" | "correct" | "unknown"
}
