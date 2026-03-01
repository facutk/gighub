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
	"time"

	"gighub/db"
	"gighub/utils"
	"gighub/views"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joelseq/sqliteadmin-go"
	"github.com/joho/godotenv"
	"github.com/justinas/nosurf"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/google"
	"golang.org/x/crypto/bcrypt"
)

var sessionManager *scs.SessionManager

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sessionManager.Exists(r.Context(), "userID") {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Initialize session manager
	sessionManager = scs.New()
	sessionManager.Lifetime = 24 * time.Hour
	sessionManager.Cookie.Persist = true
	sessionManager.Cookie.SameSite = http.SameSiteLaxMode
	sessionManager.Cookie.Secure = os.Getenv("ENV") == "production"

	// Configure Goth for Social Login
	goth.UseProviders(
		google.New(os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET"), os.Getenv("BASE_URL")+"/auth/google/callback"),
	)
	gothic.GetProviderName = func(req *http.Request) (string, error) {
		provider := chi.URLParam(req, "provider")
		if provider == "" {
			return "", fmt.Errorf("provider not found")
		}
		return provider, nil
	}

	// Initialize the router
	r := chi.NewRouter()

	// Use default middleware
	// Logger: Logs the start and end of each request
	// Recoverer: Recovers from panics and returns a 500 error instead of crashing
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(sessionManager.LoadAndSave)

	dbConn, queries, err := db.Setup("data", "gighub.db")
	if err != nil {
		log.Fatal(err)
	}
	defer dbConn.Close()

	// Define the route
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		views.Home().Render(r.Context(), w)
	})

	// Static pages
	r.Get("/privacy", func(w http.ResponseWriter, r *http.Request) {
		views.Privacy().Render(r.Context(), w)
	})

	r.Get("/terms", func(w http.ResponseWriter, r *http.Request) {
		views.Terms().Render(r.Context(), w)
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
	r.Group(func(r chi.Router) {
		r.Use(requireAuth)
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

	// Social Auth Routes
	r.Get("/auth/{provider}", func(w http.ResponseWriter, r *http.Request) {
		gothic.BeginAuthHandler(w, r)
	})

	r.Get("/auth/{provider}/callback", func(w http.ResponseWriter, r *http.Request) {
		gUser, err := gothic.CompleteUserAuth(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Check if user exists
		user, err := queries.GetUserByEmail(r.Context(), gUser.Email)
		if err != nil {
			if err == sql.ErrNoRows {
				// Create new user with random password and token
				pwBytes := make([]byte, 32)
				rand.Read(pwBytes)
				pwHash, _ := bcrypt.GenerateFromPassword(pwBytes, bcrypt.DefaultCost)

				tokenBytes := make([]byte, 16)
				rand.Read(tokenBytes)
				token := hex.EncodeToString(tokenBytes)

				user, err = queries.CreateUser(r.Context(), db.CreateUserParams{
					Email:             gUser.Email,
					PasswordHash:      string(pwHash),
					VerificationToken: sql.NullString{String: token, Valid: true},
				})
				if err != nil {
					http.Error(w, "Failed to create user", http.StatusInternalServerError)
					return
				}

				// Mark as verified immediately since it's Google
				queries.VerifyUser(r.Context(), sql.NullString{String: token, Valid: true})
			} else {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
		} else if !user.VerifiedAt.Valid {
			// If user exists but wasn't verified, verify them now since we trust Google
			queries.VerifyUser(r.Context(), user.VerificationToken)
		}

		// Log the user in
		if err := sessionManager.RenewToken(r.Context()); err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		sessionManager.Put(r.Context(), "userID", user.ID)
		http.Redirect(w, r, "/guestbook", http.StatusSeeOther)
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

	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`
			<h1>Login</h1>
			<form action="/login" method="post">
				<input type="hidden" name="csrf_token" value="%s">
				<label>Email: <input type="email" name="email" required></label><br>
				<label>Password: <input type="password" name="password" required></label><br>
				<button type="submit">Login</button>
			</form>
			<hr>
			<a href="/auth/google">Login with Google</a>
		`, nosurf.Token(r))))
	})

	r.Post("/login", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		email := r.FormValue("email")
		password := r.FormValue("password")

		user, err := queries.GetUserByEmail(r.Context(), email)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Invalid email or password", http.StatusUnauthorized)
			} else {
				http.Error(w, "Database error", http.StatusInternalServerError)
			}
			return
		}

		if !user.VerifiedAt.Valid {
			http.Error(w, "Please verify your email before logging in.", http.StatusUnauthorized)
			return
		}

		err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
		if err != nil {
			http.Error(w, "Invalid email or password", http.StatusUnauthorized)
			return
		}

		// Login successful
		if err := sessionManager.RenewToken(r.Context()); err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		sessionManager.Put(r.Context(), "userID", user.ID)

		http.Redirect(w, r, "/guestbook", http.StatusSeeOther)
	})

	r.Get("/logout", func(w http.ResponseWriter, r *http.Request) {
		if err := sessionManager.Destroy(r.Context()); err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		// Redirect to home page after logout
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
