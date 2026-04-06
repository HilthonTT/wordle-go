package api

import (
	"net/http"

	"github.com/hilthontt/wordle-go/internal/router"
	"github.com/hilthontt/wordle-go/internal/store"
)

// NewServer wires up routes and returns a ready-to-serve http.Handler.
func NewServer(s *store.Store, allowedOrigins ...string) http.Handler {
	r := router.New()
	h := NewHandler(s)

	r.Use(Recovery)
	r.Use(RequestID)
	r.Use(Logger)
	r.Use(CORS(allowedOrigins...))

	r.GET("/health", h.Health)

	games := r.Group("/games")
	games.GET("", h.ListGames)
	games.POST("", h.CreateGame)
	games.GET("/:id", h.GetGame)
	games.GET("/:id/validate", h.ValidateWord)
	games.GET("/:id/hints", h.GetHints)
	games.POST("/:id/attempts", h.SubmitAttempt)
	games.POST("/:id/reset", h.ResetGame)
	games.DELETE("/:id", h.DeleteGame)

	return r
}
