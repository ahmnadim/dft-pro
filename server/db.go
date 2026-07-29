package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var db *sql.DB

func initDB(dbPath string) {
	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	createTablesQuery := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		wallet_balance REAL DEFAULT 0.0,
		is_active INTEGER DEFAULT 1,
		is_admin INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS hwids (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		hwid TEXT UNIQUE NOT NULL,
		trial_expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		amount REAL NOT NULL,
		type TEXT NOT NULL, -- "deposit", "charge", "refund"
		description TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	`

	_, err = db.Exec(createTablesQuery)
	if err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	// Seed a default admin if not exists
	seedAdmin()
}

func seedAdmin() {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE is_admin = 1").Scan(&count)
	if err != nil {
		log.Printf("Error checking for admin: %v", err)
		return
	}

	if count == 0 {
		adminEmail := "admin@dft.com"
		adminPass := "admin123"
		hashed, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("Failed to hash admin password: %v", err)
		}

		_, err = db.Exec("INSERT INTO users (email, password_hash, wallet_balance, is_active, is_admin) VALUES (?, ?, 1000.0, 1, 1)", adminEmail, string(hashed))
		if err != nil {
			log.Fatalf("Failed to seed admin user: %v", err)
		}
		log.Println("Seeded default admin user: email: admin@dft.com, password: admin123")
	}
}

type User struct {
	ID            int64     `json:"id"`
	Email         string    `json:"email"`
	WalletBalance float64   `json:"wallet_balance"`
	IsActive      bool      `json:"is_active"`
	IsAdmin       bool      `json:"is_admin"`
	CreatedAt     time.Time `json:"created_at"`
}

type HWIDRecord struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	HWID           string    `json:"hwid"`
	TrialExpiresAt time.Time `json:"trial_expires_at"`
	CreatedAt      time.Time `json:"created_at"`
}

func registerUser(email, password string) (*User, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	res, err := db.Exec("INSERT INTO users (email, password_hash) VALUES (?, ?)", email, string(hashed))
	if err != nil {
		return nil, errors.New("email already registered")
	}

	id, _ := res.LastInsertId()
	return &User{
		ID:            id,
		Email:         email,
		WalletBalance: 0.0,
		IsActive:      true,
		IsAdmin:       false,
		CreatedAt:     time.Now(),
	}, nil
}

func loginUser(email, password string) (*User, error) {
	var u User
	var pwHash string
	var isActiveInt, isAdminInt int
	var createdAtStr string

	err := db.QueryRow("SELECT id, email, password_hash, wallet_balance, is_active, is_admin, datetime(created_at) FROM users WHERE email = ?", email).
		Scan(&u.ID, &u.Email, &pwHash, &u.WalletBalance, &isActiveInt, &isAdminInt, &createdAtStr)

	if err != nil {
		log.Printf("loginUser DB query scan error: %v", err)
		return nil, errors.New("invalid email or password")
	}

	u.IsActive = isActiveInt == 1
	u.IsAdmin = isAdminInt == 1
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
	if u.CreatedAt.IsZero() {
		u.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	}

	err = bcrypt.CompareHashAndPassword([]byte(pwHash), []byte(password))
	if err != nil {
		log.Printf("loginUser password mismatch error: %v", err)
		return nil, errors.New("invalid email or password")
	}

	if !u.IsActive {
		return nil, errors.New("this account has been deactivated by admin")
	}

	return &u, nil
}

func checkOrRegisterHWID(userID int64, hwid string) (*HWIDRecord, error) {
	var h HWIDRecord
	var trialExpiresStr string
	var createdStr string

	err := db.QueryRow("SELECT id, user_id, hwid, trial_expires_at, created_at FROM hwids WHERE hwid = ?", hwid).
		Scan(&h.ID, &h.UserID, &h.HWID, &trialExpiresStr, &createdStr)

	if err == sql.ErrNoRows {
		// New device registration: initiate 7-day trial
		trialExpiry := time.Now().Add(7 * 24 * time.Hour)
		res, err := db.Exec("INSERT INTO hwids (user_id, hwid, trial_expires_at) VALUES (?, ?, ?)", userID, hwid, trialExpiry)
		if err != nil {
			return nil, fmt.Errorf("failed to register HWID: %v", err)
		}
		id, _ := res.LastInsertId()
		return &HWIDRecord{
			ID:             id,
			UserID:         userID,
			HWID:           hwid,
			TrialExpiresAt: trialExpiry,
			CreatedAt:      time.Now(),
		}, nil
	} else if err != nil {
		return nil, err
	}

	h.TrialExpiresAt, _ = time.Parse(time.RFC3339, trialExpiresStr)
	if h.TrialExpiresAt.IsZero() {
		// Try parsing simple format if RFC3339 fails (e.g. 2026-07-29 20:00:00)
		h.TrialExpiresAt, _ = time.Parse("2006-01-02 15:04:05", trialExpiresStr)
	}

	// Update user ID binding if not set or changed
	if h.UserID != userID {
		_, _ = db.Exec("UPDATE hwids SET user_id = ? WHERE id = ?", userID, h.ID)
		h.UserID = userID
	}

	return &h, nil
}

func getUsersList() ([]User, error) {
	rows, err := db.Query("SELECT id, email, wallet_balance, is_active, is_admin, datetime(created_at) FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var isActiveInt, isAdminInt int
		var createdAtStr string
		err := rows.Scan(&u.ID, &u.Email, &u.WalletBalance, &isActiveInt, &isAdminInt, &createdAtStr)
		if err != nil {
			log.Printf("getUsersList scan error: %v", err)
			return nil, err
		}
		u.IsActive = isActiveInt == 1
		u.IsAdmin = isAdminInt == 1
		u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
		if u.CreatedAt.IsZero() {
			u.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		}
		users = append(users, u)
	}
	return users, nil
}

func toggleUserStatus(userID int64, active bool) error {
	val := 0
	if active {
		val = 1
	}
	_, err := db.Exec("UPDATE users SET is_active = ? WHERE id = ?", val, userID)
	return err
}

func updateUserBalance(userID int64, amount float64, desc string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentBalance float64
	err = tx.QueryRow("SELECT wallet_balance FROM users WHERE id = ?", userID).Scan(&currentBalance)
	if err != nil {
		return err
	}

	newBalance := currentBalance + amount
	if newBalance < 0 {
		return errors.New("insufficient balance")
	}

	_, err = tx.Exec("UPDATE users SET wallet_balance = ? WHERE id = ?", newBalance, userID)
	if err != nil {
		return err
	}

	txType := "deposit"
	if amount < 0 {
		txType = "charge"
	}

	_, err = tx.Exec("INSERT INTO transactions (user_id, amount, type, description) VALUES (?, ?, ?, ?)", userID, amount, txType, desc)
	if err != nil {
		return err
	}

	return tx.Commit()
}
