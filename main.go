package main

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"gighub/db"
	"gighub/utils"
	"gighub/views"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joelseq/sqliteadmin-go"
	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"
)

//go:embed db/migrations/*.sql
var migrationsFS embed.FS

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Initialize the router
	r := chi.NewRouter()

	// Use default middleware
	// Logger: Logs the start and end of each request
	// Recoverer: Recovers from panics and returns a 500 error instead of crashing
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	dataDir := "data"
	// Check if the data directory is writable by creating a temporary file.
	// This provides a clearer error message than the cryptic SQLite one.
	tmpFile, err := os.Create(filepath.Join(dataDir, ".writable"))
	if err != nil {
		log.Fatalf("Error: The data directory ('%s') is not writable. Please check permissions. Original error: %s", dataDir, err)
	}
	tmpFile.Close()
	os.Remove(tmpFile.Name())

	// Initialize Database
	dbConn, err := sql.Open("sqlite", filepath.Join(dataDir, "gighub.db"))
	if err != nil {
		log.Fatalf("Error opening database: %s", err)
	}
	defer dbConn.Close()

	// Run migrations
	entries, err := migrationsFS.ReadDir("db/migrations")
	if err != nil {
		log.Fatalf("Error reading migrations: %s", err)
	}
	for _, entry := range entries {
		content, _ := migrationsFS.ReadFile("db/migrations/" + entry.Name())
		if _, err := dbConn.Exec(string(content)); err != nil {
			log.Fatalf("Error running migration %s: %s", entry.Name(), err)
		}
	}

	queries := db.New(dbConn)

	// Define the route
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		views.Home().Render(r.Context(), w)
	})

	// Admin dashboard
	adminConfig := sqliteadmin.Config{
		DB:       dbConn,
		Username: os.Getenv("SQLITEADMIN_USERNAME"),
		Password: os.Getenv("SQLITEADMIN_PASSWORD"),
	}
	admin := sqliteadmin.New(adminConfig)
	r.Options("/admin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept, X-Requested-With")
		w.WriteHeader(http.StatusOK)
	})
	r.Post("/admin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		admin.HandlePost(w, r)
	})

	// Guestbook routes
	r.Get("/guestbook", func(w http.ResponseWriter, r *http.Request) {
		msg, err := queries.GetMessage(r.Context())
		if err != nil {
			if err == sql.ErrNoRows {
				msg = "Hello! Welcome to the guestbook."
			} else {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
		}
		views.Guestbook(msg).Render(r.Context(), w)
	})

	r.Post("/guestbook", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		message := r.FormValue("message")
		if err := queries.UpsertMessage(r.Context(), message); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
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
	err = http.ListenAndServe(":"+port, r)
	if err != nil {
		fmt.Printf("Error starting server: %s\n", err)
	}
}
