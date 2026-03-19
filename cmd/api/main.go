package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	application "github.com/basiq-app/internal/app"
	"github.com/basiq-app/internal/service"
)

func main() {
	fmt.Println("basiq api starting...")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	app, err := application.Setup(ctx)
	cancel()
	if err != nil {
		log.Fatalf("setup failed: %v", err)
	}
	defer app.Close()

	mux := http.NewServeMux()

	mux.HandleFunc("POST /users", func(w http.ResponseWriter, r *http.Request) {

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
			writeError(w, "email is required", http.StatusBadRequest)
			return
		}

		existingID, existingEmail, err := app.Cache.GetUser(r.Context())
		if err != nil {
			log.Printf("cache lookup error: %v", err)
			writeError(w, "internal error", http.StatusInternalServerError)
			return
		}

		if existingID != "" {
			if existingEmail != req.Email {
				writeError(w, "a user already exists with a different email", http.StatusConflict)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"id": existingID, "email": existingEmail})
			return
		}

		user, err := app.Client.CreateUser(r.Context(), req.Email)
		if err != nil {
			log.Printf("create user error: %v", err)
			writeError(w, "failed to create user", http.StatusInternalServerError)
			return
		}

		if err := app.Cache.SetUser(r.Context(), user.ID, req.Email); err != nil {
			log.Printf("cache save error: %v", err)
			writeError(w, "internal error", http.StatusInternalServerError)
			return
		}

		log.Printf("created user: %s", user.ID)
		writeJSON(w, http.StatusCreated, user)
	})

	mux.HandleFunc("POST /connections", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		rc := http.NewResponseController(w)
		rc.SetWriteDeadline(time.Now().Add(5 * time.Minute))

		userID, _, err := app.Cache.GetUser(r.Context())
		if err != nil {
			log.Printf("cache lookup error: %v", err)
			writeError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if userID == "" {
			writeError(w, "no user registered, call POST /users first", http.StatusBadRequest)
			return
		}

		existingConnID, err := app.Cache.GetConnection(r.Context())
		if err != nil {
			log.Printf("cache lookup error: %v", err)
			writeError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if existingConnID != "" {
			writeJSON(w, http.StatusOK, map[string]string{
				"user_id":       userID,
				"connection_id": existingConnID,
			})
			return
		}

		job, err := app.Client.CreateConnection(r.Context(), userID, app.Cfg.SandboxBankID, app.Cfg.BankLoginID, app.Cfg.BankPassword)
		if err != nil {
			log.Printf("create connection error: %v", err)
			writeError(w, "failed to create connection", http.StatusInternalServerError)
			return
		}
		log.Printf("connection job created: %s", job.ID)

		completedJob, err := app.Client.WaitForJob(r.Context(), job.ID)
		if err != nil {
			log.Printf("connection job error: %v", err)
			writeError(w, "connection job failed", http.StatusInternalServerError)
			return
		}

		var connectionID string
		for _, step := range completedJob.Steps {
			if step.Title == "verify-credentials" && step.Result != nil {
				connectionID = path.Base(step.Result.URL)
				break
			}
		}
		if connectionID == "" {
			writeError(w, "could not extract connection ID from job", http.StatusInternalServerError)
			return
		}

		if err := app.Cache.SetConnection(r.Context(), connectionID); err != nil {
			log.Printf("cache save error: %v", err)
			writeError(w, "internal error", http.StatusInternalServerError)
			return
		}

		log.Printf("connection ready: %s", connectionID)
		writeJSON(w, http.StatusCreated, map[string]string{
			"user_id":       userID,
			"connection_id": connectionID,
		})
	})

	mux.HandleFunc("GET /averages", func(w http.ResponseWriter, r *http.Request) {
		userID, _, err := app.Cache.GetUser(r.Context())
		if err != nil {
			log.Printf("cache lookup error: %v", err)
			writeError(w, "internal error", http.StatusInternalServerError)
			return
		}
		connectionID, err := app.Cache.GetConnection(r.Context())
		if err != nil {
			log.Printf("cache lookup error: %v", err)
			writeError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if userID == "" || connectionID == "" {
			writeError(w, "no user or connection found, call POST /users and POST /connections first", http.StatusBadRequest)
			return
		}

		transactions, err := app.Service.SyncTransactions(r.Context(), userID, connectionID)
		if err != nil {
			log.Printf("sync error: %v", err)
			writeError(w, "failed to sync transactions", http.StatusInternalServerError)
			return
		}

		averages := service.CalculateAverages(transactions)
		writeJSON(w, http.StatusOK, averages)
	})

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8081"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Println("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	log.Printf("API server listening on :%s", port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
