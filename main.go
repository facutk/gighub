package main

import (
	"fmt"
	"net/http"
	"os"
	"sync"

	"gighub/utils"
	"gighub/views"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// In-memory storage for the guestbook
var (
	guestbookMessage = "Hello! Welcome to the guestbook."
	guestbookMutex   sync.RWMutex
)

func main() {
	// Initialize the router
	r := chi.NewRouter()

	// Use default middleware
	// Logger: Logs the start and end of each request
	// Recoverer: Recovers from panics and returns a 500 error instead of crashing
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Define the route
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		views.Home().Render(r.Context(), w)
	})

	// Guestbook routes
	r.Get("/guestbook", func(w http.ResponseWriter, r *http.Request) {
		guestbookMutex.RLock()
		msg := guestbookMessage
		guestbookMutex.RUnlock()
		views.Guestbook(msg).Render(r.Context(), w)
	})

	r.Post("/guestbook", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		guestbookMutex.Lock()
		guestbookMessage = r.FormValue("message")
		guestbookMutex.Unlock()
		http.Redirect(w, r, "/guestbook", http.StatusSeeOther)
	})

	// Route to display the application version (Git SHA)
	r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		gitSHA := os.Getenv("GITSHA")
		if gitSHA == "" {
			gitSHA = "dev" // Default for local development
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(gitSHA))
	})

	// Serve static files from the ./assets directory
	utils.FileServer(r, "/assets", http.Dir("./assets"))

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	// Start the server
	fmt.Printf("Server starting on port %s...\n", port)
	err := http.ListenAndServe(":"+port, r)
	if err != nil {
		fmt.Printf("Error starting server: %s\n", err)
	}
}
