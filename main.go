package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type Device struct {
	ID           string    `json:"id"`
	SerialNumber string    `json:"serial_number"`
	CustomerName string    `json:"customer_name"`
	PhoneNumber  string    `json:"phone_number"`
	EMITerm      int       `json:"emi_term"`
	EMIStartDate time.Time `json:"emi_start_date"`
	TermDuration int       `json:"term_duration"` // 7, 15, or 30 days
	IsActive     bool      `json:"is_active"`
	IsLocked     bool      `json:"is_locked"`
	CreatedAt    time.Time `json:"created_at"`
}

type ActivationCode struct {
	ID         string     `json:"id"`
	DeviceID   string     `json:"device_id"`
	Code       string     `json:"code"`
	TermNumber int        `json:"term_number"`
	IsUsed     bool       `json:"is_used"`
	UsedAt     *time.Time `json:"used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type LockDate struct {
	ID        string    `json:"id"`
	DeviceID  string    `json:"device_id"`
	LockDate  time.Time `json:"lock_date"`
	IsLocked  bool      `json:"is_locked"`
	CreatedAt time.Time `json:"created_at"`
}

type RemoteLock struct {
	ID        string    `json:"id"`
	DeviceID  string    `json:"device_id"`
	IsLocked  bool      `json:"is_locked"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RegisterDeviceRequest struct {
	SerialNumber string `json:"serial_number"`
	CustomerName string `json:"customer_name"`
	PhoneNumber  string `json:"phone_number"`
	EMITerm      int    `json:"emi_term"`
	EMIStartDate string `json:"emi_start_date"` // Format: "2006-01-02"
	TermDuration int    `json:"term_duration"`  // 7, 15, or 30
}

type ActivateRequest struct {
	ActivationCode string `json:"activation_code"`
}

type TermWithLockDate struct {
	Term     int    `json:"term"`
	LockDate string `json:"lock_date"`
}

type TermWithLockDateAndCode struct {
	Term           int    `json:"term"`
	LockDate       string `json:"lock_date"`
	ActivationCode string `json:"activation_code"`
}

type ActivationResponse struct {
	Success bool                      `json:"success"`
	Message string                    `json:"message"`
	Terms   []TermWithLockDateAndCode `json:"terms,omitempty"`
}

type RemoteLockRequest struct {
	SerialNumber string `json:"serial_number"`
	IsLocked     bool   `json:"is_locked"`
}

type CheckLockResponse struct {
	IsLocked bool `json:"is_locked"`
}

type UnlockRequest struct {
	SerialNumber string `json:"serial_number"`
}

var db *sql.DB
var dbOnce sync.Once
var dbInitError error

func initDB() error {
	dbOnce.Do(func() {
		// Try DATABASE_URL first, then POSTGRES_URL as fallback
		connStr := os.Getenv("DATABASE_URL")
		if connStr == "" {
			connStr = os.Getenv("POSTGRES_URL")
		}
		if connStr == "" {
			dbInitError = fmt.Errorf("DATABASE_URL or POSTGRES_URL environment variable is not set")
			log.Println("ERROR: DATABASE_URL or POSTGRES_URL environment variable is not set")
			log.Println("Available env vars:", os.Environ())
			return
		}

		log.Printf("Connecting to database... (connection string length: %d)", len(connStr))

		var err error
		db, err = sql.Open("postgres", connStr)
		if err != nil {
			dbInitError = fmt.Errorf("Failed to open database connection: %v", err)
			log.Printf("ERROR: Failed to open database: %v", err)
			return
		}

		// Set connection pool settings for serverless
		db.SetMaxOpenConns(5)
		db.SetMaxIdleConns(2)
		db.SetConnMaxLifetime(5 * time.Minute)

		log.Println("Pinging database...")
		if err = db.Ping(); err != nil {
			dbInitError = fmt.Errorf("Failed to ping database: %v", err)
			log.Printf("ERROR: Database ping failed: %v", err)
			log.Printf("Connection string format: postgresql://[user]:[password]@[host]:[port]/[database]")
			return
		}

		log.Println("✓ Database connection established successfully")
	})
	return dbInitError
}

func generateActivationCode() string {
	return uuid.New().String()[:8]
}

func calculateLockDates(startDate time.Time, termDuration int, emiTerm int) []time.Time {
	var lockDates []time.Time
	currentDate := startDate

	for i := 0; i < emiTerm; i++ {
		lockDate := currentDate.AddDate(0, 0, termDuration)
		lockDates = append(lockDates, lockDate)
		currentDate = lockDate
	}

	return lockDates
}

func registerDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate term duration
	if req.TermDuration != 7 && req.TermDuration != 15 && req.TermDuration != 30 {
		http.Error(w, "Term duration must be 7, 15, or 30 days", http.StatusBadRequest)
		return
	}

	// Parse EMI start date
	emiStartDate, err := time.Parse("2006-01-02", req.EMIStartDate)
	if err != nil {
		http.Error(w, "Invalid date format. Use YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	// Check if device already exists
	var existingID string
	err = db.QueryRow("SELECT id FROM devices WHERE serial_number = $1", req.SerialNumber).Scan(&existingID)
	if err == nil {
		http.Error(w, "Device with this serial number already exists", http.StatusConflict)
		return
	}

	// Insert device
	deviceID := uuid.New().String()
	_, err = db.Exec(
		"INSERT INTO devices (id, serial_number, customer_name, phone_number, emi_term, emi_start_date, term_duration, is_active, is_locked, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)",
		deviceID, req.SerialNumber, req.CustomerName, req.PhoneNumber, req.EMITerm, emiStartDate, req.TermDuration, false, false, time.Now(),
	)
	if err != nil {
		log.Printf("Error inserting device: %v", err)
		http.Error(w, "Failed to register device", http.StatusInternalServerError)
		return
	}

	// Generate activation codes and lock dates together
	lockDates := calculateLockDates(emiStartDate, req.TermDuration, req.EMITerm)
	termsWithDates := make([]TermWithLockDateAndCode, 0)

	for i := 1; i <= req.EMITerm; i++ {
		code := generateActivationCode()
		_, err = db.Exec(
			"INSERT INTO activation_codes (id, device_id, code, term_number, is_used, created_at) VALUES ($1, $2, $3, $4, $5, $6)",
			uuid.New().String(), deviceID, code, i, false, time.Now(),
		)
		if err != nil {
			log.Printf("Error inserting activation code: %v", err)
			http.Error(w, "Failed to generate activation codes", http.StatusInternalServerError)
			return
		}

		// Insert lock date
		lockDate := lockDates[i-1]
		_, err = db.Exec(
			"INSERT INTO lock_dates (id, device_id, lock_date, is_locked, created_at) VALUES ($1, $2, $3, $4, $5)",
			uuid.New().String(), deviceID, lockDate, false, time.Now(),
		)
		if err != nil {
			log.Printf("Error inserting lock date: %v", err)
			http.Error(w, "Failed to generate lock dates", http.StatusInternalServerError)
			return
		}

		// Add to terms array with activation code
		termsWithDates = append(termsWithDates, TermWithLockDateAndCode{
			Term:           i,
			LockDate:       lockDate.Format("2006-01-02"),
			ActivationCode: code,
		})
	}

	// Create initial remote lock entry
	_, err = db.Exec(
		"INSERT INTO remote_locks (id, device_id, is_locked, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)",
		uuid.New().String(), deviceID, false, time.Now(), time.Now(),
	)
	if err != nil {
		log.Printf("Error inserting remote lock: %v", err)
	}

	response := map[string]interface{}{
		"success":   true,
		"message":   "Device registered successfully",
		"device_id": deviceID,
		"terms":     termsWithDates,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func activateDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ActivateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Find device by activation code (activation codes are unique)
	var deviceID string
	var activationCodeID string
	var termNumber int
	err := db.QueryRow(
		"SELECT ac.id, ac.device_id, ac.term_number FROM activation_codes ac WHERE ac.code = $1 AND ac.is_used = false",
		req.ActivationCode,
	).Scan(&activationCodeID, &deviceID, &termNumber)
	if err != nil {
		http.Error(w, "Invalid or already used activation code", http.StatusBadRequest)
		return
	}

	// Mark activation code as used
	now := time.Now()
	_, err = db.Exec(
		"UPDATE activation_codes SET is_used = true, used_at = $1 WHERE id = $2",
		now, activationCodeID,
	)
	if err != nil {
		log.Printf("Error updating activation code: %v", err)
		http.Error(w, "Failed to activate device", http.StatusInternalServerError)
		return
	}

	// Activate device if not already active
	_, err = db.Exec("UPDATE devices SET is_active = true WHERE id = $1", deviceID)
	if err != nil {
		log.Printf("Error activating device: %v", err)
	}

	// Get terms with their corresponding lock dates and activation codes
	termsWithDates := make([]TermWithLockDateAndCode, 0)

	// Get activation codes with term numbers ordered by term_number
	codeRows, err := db.Query("SELECT term_number, code FROM activation_codes WHERE device_id = $1 ORDER BY term_number", deviceID)
	if err == nil {
		defer codeRows.Close()

		// Get lock dates ordered by lock_date
		lockRows, err := db.Query("SELECT lock_date FROM lock_dates WHERE device_id = $1 ORDER BY lock_date", deviceID)
		if err == nil {
			defer lockRows.Close()

			// Store activation codes by term number
			codesByTerm := make(map[int]string)
			for codeRows.Next() {
				var termNumber int
				var code string
				if err := codeRows.Scan(&termNumber, &code); err == nil {
					codesByTerm[termNumber] = code
				}
			}

			// Match lock dates with terms and activation codes by index
			termIndex := 0
			termNumbers := make([]int, 0, len(codesByTerm))
			for termNum := range codesByTerm {
				termNumbers = append(termNumbers, termNum)
			}
			// Sort term numbers
			for i := 0; i < len(termNumbers)-1; i++ {
				for j := i + 1; j < len(termNumbers); j++ {
					if termNumbers[i] > termNumbers[j] {
						termNumbers[i], termNumbers[j] = termNumbers[j], termNumbers[i]
					}
				}
			}

			for lockRows.Next() {
				var lockDate time.Time
				if err := lockRows.Scan(&lockDate); err == nil {
					if termIndex < len(termNumbers) {
						termNumber := termNumbers[termIndex]
						code := codesByTerm[termNumber]
						termsWithDates = append(termsWithDates, TermWithLockDateAndCode{
							Term:           termNumber,
							LockDate:       lockDate.Format("2006-01-02"),
							ActivationCode: code,
						})
						termIndex++
					}
				}
			}
		}
	}

	response := ActivationResponse{
		Success: true,
		Message: "Device activated successfully",
		Terms:   termsWithDates,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func checkActivation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serialNumber := r.URL.Query().Get("serial_number")
	if serialNumber == "" {
		http.Error(w, "serial_number parameter is required", http.StatusBadRequest)
		return
	}

	// Find device
	var deviceID string
	var isActive bool
	err := db.QueryRow(
		"SELECT id, is_active FROM devices WHERE serial_number = $1",
		serialNumber,
	).Scan(&deviceID, &isActive)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// Automatically activate the device when TV calls this endpoint
	if !isActive {
		_, err = db.Exec("UPDATE devices SET is_active = true WHERE id = $1", deviceID)
		if err != nil {
			log.Printf("Error activating device: %v", err)
			http.Error(w, "Failed to activate device", http.StatusInternalServerError)
			return
		}
		isActive = true
		log.Printf("Device %s activated via /api/check", serialNumber)
	}

	// Get terms with their corresponding lock dates and activation codes
	termsWithDates := make([]TermWithLockDateAndCode, 0)

	// Get activation codes with term numbers ordered by term_number
	codeRows, err := db.Query("SELECT term_number, code FROM activation_codes WHERE device_id = $1 ORDER BY term_number", deviceID)
	if err == nil {
		defer codeRows.Close()

		// Get lock dates ordered by lock_date
		lockRows, err := db.Query("SELECT lock_date FROM lock_dates WHERE device_id = $1 ORDER BY lock_date", deviceID)
		if err == nil {
			defer lockRows.Close()

			// Store activation codes by term number
			codesByTerm := make(map[int]string)
			for codeRows.Next() {
				var termNumber int
				var code string
				if err := codeRows.Scan(&termNumber, &code); err == nil {
					codesByTerm[termNumber] = code
				}
			}

			// Match lock dates with terms and activation codes by index
			termIndex := 0
			termNumbers := make([]int, 0, len(codesByTerm))
			for termNum := range codesByTerm {
				termNumbers = append(termNumbers, termNum)
			}
			// Sort term numbers
			for i := 0; i < len(termNumbers)-1; i++ {
				for j := i + 1; j < len(termNumbers); j++ {
					if termNumbers[i] > termNumbers[j] {
						termNumbers[i], termNumbers[j] = termNumbers[j], termNumbers[i]
					}
				}
			}

			for lockRows.Next() {
				var lockDate time.Time
				if err := lockRows.Scan(&lockDate); err == nil {
					if termIndex < len(termNumbers) {
						termNumber := termNumbers[termIndex]
						code := codesByTerm[termNumber]
						termsWithDates = append(termsWithDates, TermWithLockDateAndCode{
							Term:           termNumber,
							LockDate:       lockDate.Format("2006-01-02"),
							ActivationCode: code,
						})
						termIndex++
					}
				}
			}
		}
	}

	response := ActivationResponse{
		Success: true,
		Message: "Device activated successfully",
		Terms:   termsWithDates,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func setRemoteLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RemoteLockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Find device
	var deviceID string
	err := db.QueryRow(
		"SELECT id FROM devices WHERE serial_number = $1",
		req.SerialNumber,
	).Scan(&deviceID)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// Update remote lock
	_, err = db.Exec(
		"UPDATE remote_locks SET is_locked = $1, updated_at = $2 WHERE device_id = $3",
		req.IsLocked, time.Now(), deviceID,
	)
	if err != nil {
		log.Printf("Error updating remote lock: %v", err)
		http.Error(w, "Failed to update remote lock", http.StatusInternalServerError)
		return
	}

	// Also update device lock status
	_, err = db.Exec("UPDATE devices SET is_locked = $1 WHERE id = $2", req.IsLocked, deviceID)
	if err != nil {
		log.Printf("Error updating device lock: %v", err)
	}

	response := map[string]interface{}{
		"success":   true,
		"message":   fmt.Sprintf("Remote lock set to %v", req.IsLocked),
		"is_locked": req.IsLocked,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func checkRemoteLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serialNumber := r.URL.Query().Get("serial_number")
	if serialNumber == "" {
		http.Error(w, "serial_number parameter is required", http.StatusBadRequest)
		return
	}

	// Find device
	var deviceID string
	err := db.QueryRow(
		"SELECT id FROM devices WHERE serial_number = $1",
		serialNumber,
	).Scan(&deviceID)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// Get remote lock status
	var isLocked bool
	err = db.QueryRow(
		"SELECT is_locked FROM remote_locks WHERE device_id = $1",
		deviceID,
	).Scan(&isLocked)
	if err != nil {
		http.Error(w, "Remote lock not found", http.StatusNotFound)
		return
	}

	response := CheckLockResponse{
		IsLocked: isLocked,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func unlockDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req UnlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Find device
	var deviceID string
	err := db.QueryRow(
		"SELECT id FROM devices WHERE serial_number = $1",
		req.SerialNumber,
	).Scan(&deviceID)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// Unlock device
	_, err = db.Exec("UPDATE devices SET is_locked = false, is_active = false WHERE id = $1", deviceID)
	if err != nil {
		log.Printf("Error unlocking device: %v", err)
		http.Error(w, "Failed to unlock device", http.StatusInternalServerError)
		return
	}

	// Update remote lock
	_, err = db.Exec(
		"UPDATE remote_locks SET is_locked = false, updated_at = $1 WHERE device_id = $2",
		time.Now(), deviceID,
	)
	if err != nil {
		log.Printf("Error updating remote lock: %v", err)
	}

	response := map[string]interface{}{
		"success": true,
		"message": "Device unlocked successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Handler is the entry point for Vercel serverless functions
func Handler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Request received: %s %s", r.Method, r.URL.Path)

	// Initialize database connection (only once)
	if err := initDB(); err != nil {
		log.Printf("Database initialization error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Database connection failed",
			"message": err.Error(),
			"hint":    "Check Vercel environment variables: DATABASE_URL or POSTGRES_URL must be set",
		})
		return
	}

	// Check if database is nil (shouldn't happen, but safety check)
	if db == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Database not initialized",
			"message": "Database connection is not available",
		})
		return
	}

	// Create router
	router := mux.NewRouter()

	// API routes
	router.HandleFunc("/api/health", healthCheck).Methods("GET")
	router.HandleFunc("/api/register", registerDevice).Methods("POST")
	router.HandleFunc("/api/activate", activateDevice).Methods("POST")
	router.HandleFunc("/api/check", checkActivation).Methods("GET")
	router.HandleFunc("/api/remote-lock", setRemoteLock).Methods("POST")
	router.HandleFunc("/api/check-lock", checkRemoteLock).Methods("GET")
	router.HandleFunc("/api/unlock", unlockDevice).Methods("POST")

	// Recovery middleware to catch panics
	recoveryMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Printf("Panic recovered: %v", err)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"error":   "Internal server error",
						"message": "An unexpected error occurred",
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}

	// CORS middleware
	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}

	handler := recoveryMiddleware(corsMiddleware(router))
	handler.ServeHTTP(w, r)
}
