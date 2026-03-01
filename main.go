package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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
	"github.com/justinas/nosurf"
	"golang.org/x/crypto/bcrypt"
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
		views.Guestbook(msg, nosurf.Token(r)).Render(r.Context(), w)
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
		to := r.URL.Query().Get("to")
		if err := sendEmail(to, "Test Email", "This is a test email from your Go app."); err != nil {
			http.Error(w, "Failed to send email: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte("Email sent successfully to " + to))
	})

	// Auth routes
	r.Get("/signup", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Pass the CSRF token to the form
		w.Write([]byte(fmt.Sprintf(`
			<h1>Sign Up</h1>
			<form action="/signup" method="post">
				<input type="hidden" name="csrf_token" value="%s">
				<label>Email: <input type="email" name="email" required></label><br>
				<label>Password: <input type="password" name="password" required></label><br>
				<button type="submit">Sign Up</button>
			</form>
		`, nosurf.Token(r))))
	})

	r.Post("/signup", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		email := r.FormValue("email")
		password := r.FormValue("password")

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		// Generate verification token
		tokenBytes := make([]byte, 16)
		rand.Read(tokenBytes)
		token := hex.EncodeToString(tokenBytes)

		if _, err := queries.CreateUser(r.Context(), db.CreateUserParams{
			Email:             email,
			PasswordHash:      string(hashedPassword),
			VerificationToken: sql.NullString{String: token, Valid: true},
		}); err != nil {
			log.Printf("Error creating user: %v", err)
			http.Error(w, "Error creating user", http.StatusInternalServerError)
			return
		}

		// Send verification email asynchronously
		go func() {
			baseURL := os.Getenv("BASE_URL")
			link := fmt.Sprintf("%s/verify?token=%s", baseURL, token)
			if err := sendEmail(email, "Verify your email", "Please verify your email by clicking here: "+link); err != nil {
				log.Printf("Failed to send welcome email: %v", err)
			}
		}()

		w.Write([]byte("User created! Please check your email to verify your account."))
	})

	r.Get("/verify", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "Missing token", http.StatusBadRequest)
			return
		}

		_, err := queries.VerifyUser(r.Context(), sql.NullString{String: token, Valid: true})
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Invalid or expired token", http.StatusBadRequest)
			} else {
				log.Printf("Verification error: %v", err)
				http.Error(w, "Server error", http.StatusInternalServerError)
			}
			return
		}

		w.Write([]byte("Email verified successfully! You can now login."))
	})

	// Route to display the application version (Git SHA)
	r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		gitSHA := os.Getenv("GITSHA")
		if gitSHA == "" {
			gitSHA = "local" // Default for local development
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

	// Add CSRF protection middleware
	csrfHandler := nosurf.New(r)
	csrfHandler.ExemptPath("/admin")
	csrfHandler.SetBaseCookie(http.Cookie{
		HttpOnly: true,
		Path:     "/",
		Secure:   os.Getenv("ENV") == "production",
	})

	// Start the server
	fmt.Printf("Server starting on port %s...\n", port)
	err = http.ListenAndServe(":"+port, csrfHandler) // Wrap router with CSRF handler
	if err != nil {
		fmt.Printf("Error starting server: %s\n", err)
	}
}

func sendEmail(to, subject, body string) error {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	from := os.Getenv("SMTP_FROM")

	if host == "" || port == "" || user == "" || pass == "" || from == "" {
		return fmt.Errorf("SMTP environment variables are not set")
	}

	auth := smtp.PlainAuth("", user, pass, host)
	msg := []byte(fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s", to, subject, body))

	return smtp.SendMail(host+":"+port, auth, from, []string{to}, msg)
}
