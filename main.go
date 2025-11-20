package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type Device struct {
	ID              string    `json:"id"`
	SerialNumber    string    `json:"serial_number"`
	CustomerName    string    `json:"customer_name"`
	PhoneNumber     string    `json:"phone_number"`
	EMITerm         int       `json:"emi_term"`
	EMIStartDate    time.Time `json:"emi_start_date"`
	TermDuration    int       `json:"term_duration"` // 7, 15, or 30 days
	IsActive        bool      `json:"is_active"`
	IsLocked        bool      `json:"is_locked"`
	CreatedAt       time.Time `json:"created_at"`
}

type ActivationCode struct {
	ID          string    `json:"id"`
	DeviceID    string    `json:"device_id"`
	Code        string    `json:"code"`
	TermNumber  int       `json:"term_number"`
	IsUsed      bool      `json:"is_used"`
	UsedAt      *time.Time `json:"used_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type LockDate struct {
	ID          string    `json:"id"`
	DeviceID    string    `json:"device_id"`
	LockDate    time.Time `json:"lock_date"`
	IsLocked    bool      `json:"is_locked"`
	CreatedAt   time.Time `json:"created_at"`
}

type RemoteLock struct {
	ID          string    `json:"id"`
	DeviceID    string    `json:"device_id"`
	IsLocked    bool      `json:"is_locked"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	Term          int    `json:"term"`
	LockDate      string `json:"lock_date"`
	ActivationCode string `json:"activation_code"`
}

type ActivationResponse struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Terms   []TermWithLockDate `json:"terms,omitempty"`
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

func initDB() {
	var err error
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	if err = db.Ping(); err != nil {
		log.Fatal("Failed to ping database:", err)
	}

	log.Println("Database connection established")
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
			Term:          i,
			LockDate:      lockDate.Format("2006-01-02"),
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
		"success": true,
		"message": "Device registered successfully",
		"device_id": deviceID,
		"terms": termsWithDates,
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

	// Get terms with their corresponding lock dates
	// Join activation_codes with lock_dates based on term_number order
	termsWithDates := make([]TermWithLockDate, 0)
	
	// Get terms ordered by term_number
	termRows, err := db.Query("SELECT term_number FROM activation_codes WHERE device_id = $1 ORDER BY term_number", deviceID)
	if err == nil {
		defer termRows.Close()
		
		// Get lock dates ordered by lock_date
		lockRows, err := db.Query("SELECT lock_date FROM lock_dates WHERE device_id = $1 ORDER BY lock_date", deviceID)
		if err == nil {
			defer lockRows.Close()
			
			lockDates := make([]time.Time, 0)
			for lockRows.Next() {
				var lockDate time.Time
				if err := lockRows.Scan(&lockDate); err == nil {
					lockDates = append(lockDates, lockDate)
				}
			}
			
			// Match terms with lock dates by index
			termIndex := 0
			for termRows.Next() {
				var termNumber int
				if err := termRows.Scan(&termNumber); err == nil {
					if termIndex < len(lockDates) {
						termsWithDates = append(termsWithDates, TermWithLockDate{
							Term:     termNumber,
							LockDate: lockDates[termIndex].Format("2006-01-02"),
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

	if !isActive {
		http.Error(w, "Device is not activated", http.StatusForbidden)
		return
	}

	// Get terms with their corresponding lock dates
	termsWithDates := make([]TermWithLockDate, 0)
	
	// Get terms ordered by term_number
	termRows, err := db.Query("SELECT term_number FROM activation_codes WHERE device_id = $1 ORDER BY term_number", deviceID)
	if err == nil {
		defer termRows.Close()
		
		// Get lock dates ordered by lock_date
		lockRows, err := db.Query("SELECT lock_date FROM lock_dates WHERE device_id = $1 ORDER BY lock_date", deviceID)
		if err == nil {
			defer lockRows.Close()
			
			lockDates := make([]time.Time, 0)
			for lockRows.Next() {
				var lockDate time.Time
				if err := lockRows.Scan(&lockDate); err == nil {
					lockDates = append(lockDates, lockDate)
				}
			}
			
			// Match terms with lock dates by index
			termIndex := 0
			for termRows.Next() {
				var termNumber int
				if err := termRows.Scan(&termNumber); err == nil {
					if termIndex < len(lockDates) {
						termsWithDates = append(termsWithDates, TermWithLockDate{
							Term:     termNumber,
							LockDate: lockDates[termIndex].Format("2006-01-02"),
						})
						termIndex++
					}
				}
			}
		}
	}

	response := ActivationResponse{
		Success: true,
		Message: "Device is active",
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
		"success": true,
		"message": fmt.Sprintf("Remote lock set to %v", req.IsLocked),
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

func main() {
	// Load environment variables (optional, for local development)
	// godotenv.Load()

	initDB()
	defer db.Close()

	r := mux.NewRouter()

	// API routes
	r.HandleFunc("/api/health", healthCheck).Methods("GET")
	r.HandleFunc("/api/register", registerDevice).Methods("POST")
	r.HandleFunc("/api/activate", activateDevice).Methods("POST")
	r.HandleFunc("/api/check", checkActivation).Methods("GET")
	r.HandleFunc("/api/remote-lock", setRemoteLock).Methods("POST")
	r.HandleFunc("/api/check-lock", checkRemoteLock).Methods("GET")
	r.HandleFunc("/api/unlock", unlockDevice).Methods("POST")

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

	handler := corsMiddleware(r)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

