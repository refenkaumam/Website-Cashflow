package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Kunci rahasia JWT (Pastikan aman di lingkungan produksi)
var jwtKey = []byte("rahasia-super-aman-cashflow-2026")

type Claims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Struct untuk menerima data tambah dompet/akun
type AccountRequest struct {
	BankName string  `json:"bank_name"`
	Balance  float64 `json:"balance"`
}

type Transaction struct {
	AccountID       int     `json:"account_id"`
	TargetAccountID *int    `json:"target_account_id"`
	Type            string  `json:"transaction_type"`
	Amount          float64 `json:"amount"`
	AdminFee        float64 `json:"admin_fee"`
	Desc            string  `json:"description"`
}

type Account struct {
	ID      int     `json:"id"`
	Name    string  `json:"bank_name"`
	Balance float64 `json:"balance"`
}

type TransactionRow struct {
	ID        int     `json:"id"`
	BankName  string  `json:"bank_name"`
	Type      string  `json:"transaction_type"`
	Amount    float64 `json:"amount"`
	AdminFee  float64 `json:"admin_fee"`
	Desc      string  `json:"description"`
	CreatedAt string  `json:"created_at"`
}

type Bill struct {
	ID            int     `json:"id"`
	Platform      string  `json:"platform"`
	ItemName      string  `json:"item_name"`
	MonthlyAmount float64 `json:"monthly_amount"`
	Tenor         int     `json:"tenor"`
	TotalAmount   float64 `json:"total_amount"`
}

type Piutang struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
}

type DashboardData struct {
	Balances     []Account        `json:"balances"`
	Transactions []TransactionRow `json:"transactions"`
	Bills        []Bill           `json:"bills"`
	Piutangs     []Piutang        `json:"piutangs"`
	TotalBalance float64          `json:"total_balance"`
	TotalBill    float64          `json:"total_bill"`
	TotalPiutang float64          `json:"total_piutang"`
	NetBalance   float64          `json:"net_balance"`
	GrandTotal   float64          `json:"grand_total"`
}

type PayBillRequest struct {
	BillID     int     `json:"bill_id"`
	AccountID  int     `json:"account_id"`
	PaidAmount float64 `json:"paid_amount"`
}

type User struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

var db *sql.DB

// toDSN mengubah format connection string URL Railway jadi DSN Go yang aman
func toDSN(raw string) string {
	if !strings.HasPrefix(raw, "mysql://") {
		return raw // sudah format DSN, tidak perlu diubah
	}

	u, err := url.Parse(raw)
	if err != nil {
		log.Fatal("DATABASE_URL tidak valid: ", err)
	}

	user := u.User.Username()
	pass, _ := u.User.Password()
	host := u.Host
	
	// Ambil nama database dari path, jika kosong gunakan default "railway"
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		dbName = "railway"
	}

	return fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true", user, pass, host, dbName)
}

func main() {
	mysql.RegisterTLSConfig("custom", &tls.Config{
		InsecureSkipVerify: true,
	})

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL belum diset di environment variable Railway!")
	}
	dbURL = toDSN(dbURL)

	var err error
	db, err = sql.Open("mysql", dbURL)
	if err != nil {
		log.Fatal("Gagal menghubungkan ke database:", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatal("Database tidak bisa di-ping:", err)
	}
	log.Println("Berhasil terhubung ke database Railway!")

	// TABEL USERS
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INT AUTO_INCREMENT PRIMARY KEY,
			username VARCHAR(50) NOT NULL UNIQUE,
			email VARCHAR(100) NOT NULL UNIQUE,
			password VARCHAR(255) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Println("Gagal membuat tabel users:", err)
	}

	// ROUTING HALAMAN
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "home.html")
	})

	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "login.html")
	})

	http.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	// Endpoint Publik (DILINDUNGI RATE LIMITER ANTI-BOT)
	http.HandleFunc("/api/register", rateLimitMiddleware(handleRegister))
	http.HandleFunc("/api/login", rateLimitMiddleware(handleLogin))

	// Endpoint Privat (Dilindungi Middleware JWT)
	http.HandleFunc("/api/account", authMiddleware(handleAddAccount))
	http.HandleFunc("/api/transaction", authMiddleware(handleTransaction))
	http.HandleFunc("/api/transaction/delete", authMiddleware(handleDeleteTransaction))
	http.HandleFunc("/api/history", authMiddleware(handleHistory))
	http.HandleFunc("/api/bill", authMiddleware(handleBill))
	http.HandleFunc("/api/bill/delete", authMiddleware(handleDeleteBill))
	http.HandleFunc("/api/bill/pay", authMiddleware(handlePayBill))
	http.HandleFunc("/api/piutang", authMiddleware(handlePiutang))
	http.HandleFunc("/api/piutang/delete", authMiddleware(handleDeletePiutang))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Server berjalan di port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// =========================================================================
// FITUR KEAMANAN: RATE LIMITER (ANTI SPAM / BRUTE FORCE)
// =========================================================================
var (
	ipRates = make(map[string]*rateData)
	rateMu  sync.Mutex
)

type rateData struct {
	count     int
	lastReset time.Time
}

func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = strings.Split(forwarded, ",")[0]
		} else {
			ip = strings.Split(ip, ":")[0]
		}

		rateMu.Lock()
		v, exists := ipRates[ip]

		if !exists || time.Since(v.lastReset) > time.Minute {
			ipRates[ip] = &rateData{count: 1, lastReset: time.Now()}
			rateMu.Unlock()
			next(w, r)
			return
		}

		if v.count >= 10 {
			rateMu.Unlock()
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Terlalu banyak percobaan. Harap tunggu 1 menit lagi karena alasan keamanan."))
			return
		}

		v.count++
		rateMu.Unlock()
		next(w, r)
	}
}

// --- MIDDLEWARE KEAMANAN JWT ---
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Akses ditolak: Token tidak ditemukan", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Akses ditolak: Token tidak valid atau kedaluwarsa", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), "userID", claims.UserID)
		next(w, r.WithContext(ctx))
	}
}

// --- FUNGSI REGISTER ---
func handleRegister(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		return
	}

	var u User
	json.NewDecoder(r.Body).Decode(&u)

	if u.Username == "" || u.Email == "" || u.Password == "" {
		http.Error(w, "Username, Email, dan password tidak boleh kosong", http.StatusBadRequest)
		return
	}

	if len(u.Username) > 50 || len(u.Username) < 3 {
		http.Error(w, "Username harus antara 3 - 50 karakter", http.StatusBadRequest)
		return
	}

	if len(u.Password) < 8 {
		http.Error(w, "Keamanan Lemah: Kata sandi minimal harus 8 karakter!", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Gagal mengenkripsi password", http.StatusInternalServerError)
		return
	}

	res, err := db.Exec("INSERT INTO users (username, email, password) VALUES (?, ?, ?)", u.Username, u.Email, string(hashedPassword))
	if err != nil {
		log.Println("DEBUG INSERT ERROR:", err)
		http.Error(w, "Pendaftaran Gagal: Username atau Email sudah digunakan", http.StatusConflict)
		return
	}

	newUserID, _ := res.LastInsertId()
	db.Exec("INSERT INTO accounts (bank_name, balance, user_id) VALUES ('Dompet Utama', 0.00, ?)", newUserID)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Registrasi berhasil"})
}

// Handler Login (Menerbitkan Token JWT)
func handleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		return
	}

	var u User
	json.NewDecoder(r.Body).Decode(&u)

	var storedPassword string
	var userID int
	err := db.QueryRow("SELECT id, password FROM users WHERE username = ?", u.Username).Scan(&userID, &storedPassword)
	if err != nil {
		http.Error(w, "Username atau password salah", http.StatusUnauthorized)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(u.Password))
	if err != nil {
		http.Error(w, "Username atau password salah", http.StatusUnauthorized)
		return
	}

	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		UserID:   userID,
		Username: u.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		http.Error(w, "Gagal membuat token sesi", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Login berhasil",
		"token":   tokenString,
	})
}

// --- Handler Cashflow ---

func handleAddAccount(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int)
	var req AccountRequest
	json.NewDecoder(r.Body).Decode(&req)

	if req.BankName == "" {
		http.Error(w, "Nama dompet tidak boleh kosong", http.StatusBadRequest)
		return
	}

	_, err := db.Exec("INSERT INTO accounts (bank_name, balance, user_id) VALUES (?, ?, ?)", req.BankName, req.Balance, userID)
	if err != nil {
		http.Error(w, "Gagal menambahkan dompet", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func handleTransaction(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int)
	var t Transaction
	json.NewDecoder(r.Body).Decode(&t)
	tx, _ := db.Begin()

	tx.Exec(`INSERT INTO transactions (account_id, transaction_type, amount, admin_fee, description, user_id) VALUES (?, ?, ?, ?, ?, ?)`, t.AccountID, t.Type, t.Amount, t.AdminFee, t.Desc, userID)

	if t.Type == "INCOME" {
		tx.Exec("UPDATE accounts SET balance = balance + ? WHERE id = ? AND user_id = ?", t.Amount, t.AccountID, userID)
	} else if t.Type == "EXPENSE" {
		tx.Exec("UPDATE accounts SET balance = balance - ? WHERE id = ? AND user_id = ?", t.Amount+t.AdminFee, t.AccountID, userID)
	} else if t.Type == "TRANSFER" {
		tx.Exec("UPDATE accounts SET balance = balance - ? WHERE id = ? AND user_id = ?", t.Amount+t.AdminFee, t.AccountID, userID)
		tx.Exec("UPDATE accounts SET balance = balance + ? WHERE id = ? AND user_id = ?", t.Amount, *t.TargetAccountID, userID)
		incDesc := fmt.Sprintf("Transfer masuk (Ref: %s)", t.Desc)
		tx.Exec(`INSERT INTO transactions (account_id, transaction_type, amount, admin_fee, description, user_id) VALUES (?, 'INCOME', ?, 0, ?, ?)`, *t.TargetAccountID, t.Amount, incDesc, userID)
	}
	tx.Commit()
	w.WriteHeader(http.StatusCreated)
}

func handleDeleteTransaction(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int)
	idStr := r.URL.Query().Get("id")
	tx, _ := db.Begin()
	var accID int
	var tType string
	var amount, adminFee float64

	err := tx.QueryRow("SELECT account_id, transaction_type, amount, admin_fee FROM transactions WHERE id = ? AND user_id = ?", idStr, userID).Scan(&accID, &tType, &amount, &adminFee)
	if err == nil {
		if tType == "INCOME" {
			tx.Exec("UPDATE accounts SET balance = balance - ? WHERE id = ? AND user_id = ?", amount, accID, userID)
		} else {
			tx.Exec("UPDATE accounts SET balance = balance + ? WHERE id = ? AND user_id = ?", amount+adminFee, accID, userID)
		}
		tx.Exec("DELETE FROM transactions WHERE id = ? AND user_id = ?", idStr, userID)
	}
	tx.Commit()
	w.WriteHeader(http.StatusOK)
}

func handleBill(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int)
	var b Bill
	json.NewDecoder(r.Body).Decode(&b)
	if b.Tenor > 0 {
		b.MonthlyAmount = b.TotalAmount / float64(b.Tenor)
	}
	db.Exec("INSERT INTO bills (platform, item_name, monthly_amount, tenor, total_amount, user_id) VALUES (?, ?, ?, ?, ?, ?)", b.Platform, b.ItemName, b.MonthlyAmount, b.Tenor, b.TotalAmount, userID)
	w.WriteHeader(http.StatusCreated)
}

func handleDeleteBill(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int)
	db.Exec("DELETE FROM bills WHERE id = ? AND user_id = ?", r.URL.Query().Get("id"), userID)
	w.WriteHeader(http.StatusOK)
}

func handlePayBill(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int)
	var req PayBillRequest
	json.NewDecoder(r.Body).Decode(&req)
	tx, _ := db.Begin()
	var platform, itemName string
	var currentTotal float64
	var tenor int

	err := tx.QueryRow("SELECT platform, item_name, total_amount, tenor FROM bills WHERE id = ? AND user_id = ?", req.BillID, userID).Scan(&platform, &itemName, &currentTotal, &tenor)
	if err != nil {
		tx.Rollback()
		http.Error(w, "Not found", 404)
		return
	}

	desc := fmt.Sprintf("Cicilan %s - %s", platform, itemName)
	tx.Exec("INSERT INTO transactions (account_id, transaction_type, amount, admin_fee, description, user_id) VALUES (?, 'EXPENSE', ?, 0, ?, ?)", req.AccountID, req.PaidAmount, desc, userID)
	tx.Exec("UPDATE accounts SET balance = balance - ? WHERE id = ? AND user_id = ?", req.PaidAmount, req.AccountID, userID)

	newTotal := currentTotal - req.PaidAmount
	newTenor := tenor - 1
	if newTenor <= 0 || newTotal <= 0 {
		tx.Exec("DELETE FROM bills WHERE id = ? AND user_id = ?", req.BillID, userID)
	} else {
		newMonthly := newTotal / float64(newTenor)
		tx.Exec("UPDATE bills SET tenor = ?, total_amount = ?, monthly_amount = ? WHERE id = ? AND user_id = ?", newTenor, newTotal, newMonthly, req.BillID, userID)
	}
	tx.Commit()
	w.WriteHeader(http.StatusOK)
}

func handlePiutang(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int)
	var p Piutang
	json.NewDecoder(r.Body).Decode(&p)
	db.Exec("INSERT INTO piutang (name, amount, user_id) VALUES (?, ?, ?)", p.Name, p.Amount, userID)
	w.WriteHeader(http.StatusCreated)
}

func handleDeletePiutang(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int)
	db.Exec("DELETE FROM piutang WHERE id = ? AND user_id = ?", r.URL.Query().Get("id"), userID)
	w.WriteHeader(http.StatusOK)
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int)
	w.Header().Set("Content-Type", "application/json")
	var data DashboardData
	var totalBal, totalBill, totalPiutang float64

	rowsAcc, err := db.Query("SELECT id, bank_name, balance FROM accounts WHERE user_id = ?", userID)
	if err == nil {
		for rowsAcc.Next() {
			var acc Account
			rowsAcc.Scan(&acc.ID, &acc.Name, &acc.Balance)
			data.Balances = append(data.Balances, acc)
			totalBal += acc.Balance
		}
		rowsAcc.Close()
	}

	rowsBill, err := db.Query("SELECT id, platform, item_name, monthly_amount, tenor, total_amount FROM bills WHERE user_id = ?", userID)
	if err == nil {
		for rowsBill.Next() {
			var b Bill
			rowsBill.Scan(&b.ID, &b.Platform, &b.ItemName, &b.MonthlyAmount, &b.Tenor, &b.TotalAmount)
			data.Bills = append(data.Bills, b)
			totalBill += b.TotalAmount
		}
		rowsBill.Close()
	}

	rowsPiutang, err := db.Query("SELECT id, name, amount FROM piutang WHERE user_id = ?", userID)
	if err == nil {
		for rowsPiutang.Next() {
			var p Piutang
			rowsPiutang.Scan(&p.ID, &p.Name, &p.Amount)
			data.Piutangs = append(data.Piutangs, p)
			totalPiutang += p.Amount
		}
		rowsPiutang.Close()
	}

	rowsTx, err := db.Query("SELECT t.id, a.bank_name, t.transaction_type, t.amount, t.admin_fee, t.description, DATE_FORMAT(t.created_at, '%Y-%m-%d %H:%i') FROM transactions t JOIN accounts a ON t.account_id = a.id WHERE t.user_id = ? ORDER BY t.created_at DESC LIMIT 50", userID)
	if err == nil {
		for rowsTx.Next() {
			var tx TransactionRow
			rowsTx.Scan(&tx.ID, &tx.BankName, &tx.Type, &tx.Amount, &tx.AdminFee, &tx.Desc, &tx.CreatedAt)
			data.Transactions = append(data.Transactions, tx)
		}
		rowsTx.Close()
	}

	data.TotalBalance = totalBal
	data.TotalBill = totalBill
	data.TotalPiutang = totalPiutang
	data.NetBalance = totalBal - totalBill
	data.GrandTotal = (totalBal - totalBill) + totalPiutang

	json.NewEncoder(w).Encode(data)
}