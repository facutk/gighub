package main

import (
	"fmt"
	"net/http"
	"os"

	"gighub/utils"
	"gighub/views"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./assets/favicon.ico")
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
