package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"

	"gighub/db"
	"gighub/utils"
	"gighub/views"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joelseq/sqliteadmin-go"
	"github.com/joho/godotenv"
)

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

	dbConn, queries, err := db.Setup("data", "gighub.db")
	if err != nil {
		log.Fatal(err)
	}
	defer dbConn.Close()

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
