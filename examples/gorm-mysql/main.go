// Package main demonstrates the GORM collector with a MySQL database.
// Run with docker-compose to start both MySQL and the example server.
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

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Product is a sample model.
type Product struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Name      string    `gorm:"size:200" json:"name"`
	Price     float64   `json:"price"`
	Stock     int       `json:"stock"`
	CreatedAt time.Time `json:"created_at"`
}

// Order is a sample model with a foreign key to Product.
type Order struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	ProductID uint      `json:"product_id"`
	Product   Product   `json:"product,omitempty"`
	Quantity  int       `json:"quantity"`
	Total     float64   `json:"total"`
	CreatedAt time.Time `json:"created_at"`
}

func main() {
	// Connect to MySQL
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "profiler:profiler@tcp(localhost:3306)/profiler_demo?charset=utf8mb4&parseTime=True&loc=Local"
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(&Product{}, &Order{}); err != nil {
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
		gormcollector.WithConnection("mysql-main", db),
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
	mux.HandleFunc("/api/products", handleProducts(db))
	mux.HandleFunc("/api/orders", handleOrders(db))
	mux.HandleFunc("/api/orders/n1", handleOrdersN1(db))
	mux.HandleFunc("/api/purchase", handlePurchase(db))
	mux.HandleFunc("/api/error", handleErrorQuery(db))

	// Wrap with profiler middleware and GORM context middleware
	srv := &http.Server{
		Addr:    ":8080",
		Handler: p.Middleware(gormCollector.Middleware(mux)),
	}

	fmt.Println("=== Go Profiler - GORM MySQL Example ===")
	fmt.Println()
	fmt.Println("Server running at:  http://localhost:8080")
	fmt.Println("Profiler UI at:     http://localhost:8080/_profiler/")
	fmt.Println()
	fmt.Println("Try these endpoints:")
	fmt.Println("  GET  /api/products    - List products")
	fmt.Println("  GET  /api/orders      - List orders (eager loaded)")
	fmt.Println("  GET  /api/orders/n1   - Orders with N+1 problem")
	fmt.Println("  POST /api/purchase    - Purchase in transaction")
	fmt.Println("  GET  /api/error       - Query that produces an error")
	fmt.Println()

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func seedDatabase(db *gorm.DB) {
	var count int64
	db.Model(&Product{}).Count(&count)
	if count > 0 {
		return
	}

	products := []Product{
		{Name: "Mechanical Keyboard", Price: 149.99, Stock: 50},
		{Name: "USB-C Monitor", Price: 399.99, Stock: 20},
		{Name: "Wireless Mouse", Price: 59.99, Stock: 100},
		{Name: "Standing Desk", Price: 599.99, Stock: 10},
		{Name: "Noise-Canceling Headphones", Price: 299.99, Stock: 30},
	}
	db.Create(&products)

	orders := []Order{
		{ProductID: products[0].ID, Quantity: 2, Total: 299.98},
		{ProductID: products[1].ID, Quantity: 1, Total: 399.99},
		{ProductID: products[2].ID, Quantity: 3, Total: 179.97},
		{ProductID: products[0].ID, Quantity: 1, Total: 149.99},
		{ProductID: products[4].ID, Quantity: 1, Total: 299.99},
	}
	db.Create(&orders)
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>GORM MySQL Example</title></head>
<body>
<h1>Go Profiler - GORM MySQL Example</h1>
<p>Check the <a href="/_profiler/">Profiler UI</a> to see captured database queries.</p>
<h2>Test Endpoints:</h2>
<ul>
<li><a href="/api/products">/api/products</a> - List products</li>
<li><a href="/api/orders">/api/orders</a> - List orders (eager loaded)</li>
<li><a href="/api/orders/n1">/api/orders/n1</a> - N+1 query problem demo</li>
<li><a href="/api/purchase">/api/purchase</a> - Purchase in transaction (POST)</li>
<li><a href="/api/error">/api/error</a> - Error query</li>
</ul>
</body>
</html>`)
}

func handleProducts(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var products []Product
		if err := db.WithContext(r.Context()).Find(&products).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(products)
	}
}

func handleOrders(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var orders []Order
		// Eager load Product to avoid N+1
		if err := db.WithContext(r.Context()).Preload("Product").Find(&orders).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(orders)
	}
}

func handleOrdersN1(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var orders []Order
		// Intentional N+1: load orders without preloading Product
		if err := db.WithContext(r.Context()).Find(&orders).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Then load each product individually (N+1 problem!)
		for i := range orders {
			db.WithContext(r.Context()).First(&orders[i].Product, orders[i].ProductID)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(orders)
	}
}

func handlePurchase(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}

		ctx := gormcollector.WithTransaction(r.Context())

		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Find product
			var product Product
			if err := tx.First(&product, 1).Error; err != nil {
				return err
			}

			// Check stock
			if product.Stock < 1 {
				return fmt.Errorf("out of stock")
			}

			// Decrease stock
			if err := tx.Model(&product).Update("stock", product.Stock-1).Error; err != nil {
				return err
			}

			// Create order
			order := Order{
				ProductID: product.ID,
				Quantity:  1,
				Total:     product.Price,
			}
			return tx.Create(&order).Error
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "purchased"})
	}
}

func handleErrorQuery(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Try to query a table that doesn't exist
		var result []map[string]any
		err := db.WithContext(r.Context()).Raw("SELECT * FROM nonexistent_table").Scan(&result).Error

		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": err.Error(),
				"note":  "Check the profiler to see the failed query details",
			})
			return
		}

		json.NewEncoder(w).Encode(result)
	}
}
