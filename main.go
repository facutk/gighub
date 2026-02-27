package main

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

	// Initialize migration tracking
	if _, err := dbConn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`); err != nil {
		log.Fatalf("Error creating schema_migrations: %s", err)
	}

	var currentVersion int
	if err := dbConn.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion); err != nil {
		log.Fatalf("Error getting current version: %s", err)
	}

	// Run migrations
	entries, err := migrationsFS.ReadDir("db/migrations")
	if err != nil {
		log.Fatalf("Error reading migrations: %s", err)
	}
	for _, entry := range entries {
		parts := strings.Split(entry.Name(), "_")
		if len(parts) == 0 {
			continue
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		if version > currentVersion {
			fmt.Printf("Running migration %s...\n", entry.Name())
			content, _ := migrationsFS.ReadFile("db/migrations/" + entry.Name())

			tx, err := dbConn.Begin()
			if err != nil {
				log.Fatalf("Error starting transaction: %s", err)
			}
			if _, err := tx.Exec(string(content)); err != nil {
				tx.Rollback()
				log.Fatalf("Error running migration %s: %s", entry.Name(), err)
			}
			if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
				tx.Rollback()
				log.Fatalf("Error updating schema_migrations: %s", err)
			}
			if err := tx.Commit(); err != nil {
				log.Fatalf("Error committing transaction: %s", err)
			}
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

	// Email test route
	r.Get("/email", func(w http.ResponseWriter, r *http.Request) {
		host := os.Getenv("SMTP_HOST")
		port := os.Getenv("SMTP_PORT")
		user := os.Getenv("SMTP_USER")
		pass := os.Getenv("SMTP_PASS")
		from := os.Getenv("SMTP_FROM")
		to := r.URL.Query().Get("to")

		if host == "" || port == "" || user == "" || pass == "" || from == "" {
			http.Error(w, "SMTP environment variables are not set", http.StatusInternalServerError)
			return
		}

		auth := smtp.PlainAuth("", user, pass, host)
		msg := []byte(fmt.Sprintf("To: %s\r\nSubject: Test Email\r\n\r\nThis is a test email from your Go app.", to))

		if err := smtp.SendMail(host+":"+port, auth, from, []string{to}, msg); err != nil {
			http.Error(w, "Failed to send email: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte("Email sent successfully to " + to))
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
