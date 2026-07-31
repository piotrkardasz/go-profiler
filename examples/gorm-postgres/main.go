// Package main demonstrates the GORM collector with a PostgreSQL database.
// Run with docker-compose to start both PostgreSQL and the example server.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	profiler "github.com/piotrkardasz/go-profiler"
	"github.com/piotrkardasz/go-profiler/collector"
	gormcollector "github.com/piotrkardasz/go-profiler/collector/gorm"
	"github.com/piotrkardasz/go-profiler/handler"
	"github.com/piotrkardasz/go-profiler/storage"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// User is a sample model.
type User struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Name      string    `gorm:"size:100" json:"name"`
	Email     string    `gorm:"size:200;uniqueIndex" json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// Post is a sample model with a foreign key to User.
type Post struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Title     string    `gorm:"size:200" json:"title"`
	Body      string    `json:"body"`
	UserID    uint      `json:"user_id"`
	User      User      `json:"user,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func main() {
	// Connect to PostgreSQL
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=profiler password=profiler dbname=profiler_demo port=5432 sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(&User{}, &Post{}); err != nil {
		log.Fatalf("Failed to migrate: %v", err)
	}

	// Seed data
	seedDatabase(db)

	// Create profiler storage
	store, err := storage.NewFilesystemStorage("./var/profiler")
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	// Create the GORM collector
	gormCollector, err := gormcollector.New(
		gormcollector.WithConnection("postgres-main", db),
		gormcollector.WithSlowThreshold(50*time.Millisecond),
		gormcollector.WithN1Threshold(3),
	)
	if err != nil {
		log.Fatalf("Failed to create GORM collector: %v", err)
	}

	// Create profiler
	cfg := profiler.DefaultConfig()
	p := profiler.New(cfg, store)

	// Register collectors
	p.AddCollector(collector.NewRequestCollector())
	p.AddCollector(collector.NewTimingCollector())
	p.AddCollector(collector.NewMemoryCollector())
	p.AddCollector(gormCollector)

	// Set up routes
	mux := http.NewServeMux()

	// Profiler routes
	apiHandler := handler.NewAPIHandler(p)
	apiHandler.RegisterRoutes(mux, cfg.RoutePrefix)
	uiHandler := handler.NewUIHandler(handler.UIConfig{
		RoutePrefix:  cfg.RoutePrefix,
		DevMode:      cfg.UIDevMode,
		DevServerURL: cfg.UIDevServerURL,
		Assets:       handler.UIDistFS(),
	})
	uiHandler.RegisterRoutes(mux, cfg.RoutePrefix)

	// Application routes
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/api/users", handleUsers(db))
	mux.HandleFunc("/api/users/create", handleCreateUser(db))
	mux.HandleFunc("/api/posts", handlePosts(db))
	mux.HandleFunc("/api/posts/n1", handlePostsN1(db))
	mux.HandleFunc("/api/transaction", handleTransaction(db))
	mux.HandleFunc("/api/error", handleErrorQuery(db))

	// Wrap with profiler middleware and GORM context middleware
	srv := &http.Server{
		Addr:    ":8080",
		Handler: p.Middleware(gormCollector.Middleware(mux)),
	}

	fmt.Println("=== Go Profiler - GORM PostgreSQL Example ===")
	fmt.Println()
	fmt.Println("Server running at:  http://localhost:8080")
	fmt.Println("Profiler UI at:     http://localhost:8080/_profiler/")
	fmt.Println()
	fmt.Println("Try these endpoints:")
	fmt.Println("  GET  /api/users         - List users (simple query)")
	fmt.Println("  POST /api/users/create  - Create a user")
	fmt.Println("  GET  /api/posts         - List posts with eager loading")
	fmt.Println("  GET  /api/posts/n1      - List posts with N+1 problem")
	fmt.Println("  POST /api/transaction   - Transaction example")
	fmt.Println("  GET  /api/error         - Query that produces an error")
	fmt.Println()

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func seedDatabase(db *gorm.DB) {
	var count int64
	db.Model(&User{}).Count(&count)
	if count > 0 {
		return
	}

	users := []User{
		{Name: "Alice", Email: "alice@example.com"},
		{Name: "Bob", Email: "bob@example.com"},
		{Name: "Charlie", Email: "charlie@example.com"},
		{Name: "Diana", Email: "diana@example.com"},
		{Name: "Eve", Email: "eve@example.com"},
	}
	db.Create(&users)

	posts := []Post{
		{Title: "Hello World", Body: "First post", UserID: users[0].ID},
		{Title: "Go is Great", Body: "Learning Go", UserID: users[0].ID},
		{Title: "PostgreSQL Tips", Body: "Database tips", UserID: users[1].ID},
		{Title: "Docker 101", Body: "Container basics", UserID: users[2].ID},
		{Title: "GORM Guide", Body: "ORM patterns", UserID: users[3].ID},
	}
	db.Create(&posts)
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>GORM PostgreSQL Example</title></head>
<body>
<h1>Go Profiler - GORM PostgreSQL Example</h1>
<p>Check the <a href="/_profiler/">Profiler UI</a> to see captured database queries.</p>
<h2>Test Endpoints:</h2>
<ul>
<li><a href="/api/users">/api/users</a> - List users</li>
<li><a href="/api/posts">/api/posts</a> - List posts (eager loaded)</li>
<li><a href="/api/posts/n1">/api/posts/n1</a> - N+1 query problem demo</li>
<li><a href="/api/transaction">/api/transaction</a> - Transaction example (POST)</li>
<li><a href="/api/error">/api/error</a> - Error query</li>
</ul>
</body>
</html>`)
}

func handleUsers(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var users []User
		// Use the request context so the GORM collector captures this query
		if err := db.WithContext(r.Context()).Find(&users).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	}
}

func handleCreateUser(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}

		user := User{
			Name:  fmt.Sprintf("User-%d", time.Now().Unix()),
			Email: fmt.Sprintf("user-%d@example.com", time.Now().UnixNano()),
		}

		if err := db.WithContext(r.Context()).Create(&user).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(user)
	}
}

func handlePosts(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var posts []Post
		// Eager load User to avoid N+1
		if err := db.WithContext(r.Context()).Preload("User").Find(&posts).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(posts)
	}
}

func handlePostsN1(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var posts []Post
		// Intentional N+1: load posts without preloading User
		if err := db.WithContext(r.Context()).Find(&posts).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Then load each user individually (N+1 problem!)
		for i := range posts {
			db.WithContext(r.Context()).First(&posts[i].User, posts[i].UserID)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(posts)
	}
}

func handleTransaction(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}

		ctx := gormcollector.WithTransaction(r.Context())

		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			user := User{
				Name:  fmt.Sprintf("TxUser-%d", time.Now().Unix()),
				Email: fmt.Sprintf("tx-%d@example.com", time.Now().UnixNano()),
			}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}

			post := Post{
				Title:  "Transaction Post",
				Body:   "Created in a transaction",
				UserID: user.ID,
			}
			if err := tx.Create(&post).Error; err != nil {
				return err
			}

			return nil
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "committed"})
	}
}

func handleErrorQuery(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Try to insert a duplicate email to trigger a constraint violation
		user := User{
			Name:  "Duplicate",
			Email: "alice@example.com", // already exists from seed
		}
		err := db.WithContext(r.Context()).Create(&user).Error

		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{
				"error": err.Error(),
				"note":  "Check the profiler to see the failed query details",
			})
			return
		}

		json.NewEncoder(w).Encode(user)
	}
}
