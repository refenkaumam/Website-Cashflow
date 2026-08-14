package main

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Kunci rahasia JWT (Pastikan aman di lingkungan produksi)
var jwtKey = []byte("rahasia-super-aman-cashflow-2026")

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
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
	Password string `json:"password"`
}

var db *sql.DB

func main() {
	mysql.RegisterTLSConfig("custom", &tls.Config{
		InsecureSkipVerify: true,
	})

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL belum diset di environment variable!")
	}

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

	// Otomatis buat tabel users jika belum ada
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS users (
            id INT AUTO_INCREMENT PRIMARY KEY,
            username VARCHAR(50) NOT NULL UNIQUE,
            password VARCHAR(255) NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    `)
	if err != nil {
		log.Println("Gagal membuat tabel users:", err)
	} else {
		log.Println("Tabel users siap digunakan!")
	}

	// ROUTING HALAMAN
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "login.html")
	})

	http.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	// Endpoint Publik (Auth)
	http.HandleFunc("/api/register", handleRegister)
	http.HandleFunc("/api/login", handleLogin)

	// Endpoint Privat (Dilindungi Satpam Middleware JWT)
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

		next(w, r)
	}
}

// Handler Register
func handleRegister(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		return
	}

	var u User
	json.NewDecoder(r.Body).Decode(&u)

	if u.Username == "" || u.Password == "" {
		http.Error(w, "Username dan password tidak boleh kosong", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Gagal mengenkripsi password", http.StatusInternalServerError)
		return
	}

	_, err = db.Exec("INSERT INTO users (username, password) VALUES (?, ?)", u.Username, string(hashedPassword))
	if err != nil {
		http.Error(w, "Username sudah digunakan", http.StatusConflict)
		return
	}

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
	err := db.QueryRow("SELECT password FROM users WHERE username = ?", u.Username).Scan(&storedPassword)
	if err != nil {
		http.Error(w, "Username atau password salah", http.StatusUnauthorized)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(u.Password))
	if err != nil {
		http.Error(w, "Username atau password salah", http.StatusUnauthorized)
		return
	}

	// Buat Token JWT berlaku selama 24 jam
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
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
func handleTransaction(w http.ResponseWriter, r *http.Request) {
	var t Transaction
	json.NewDecoder(r.Body).Decode(&t)
	tx, _ := db.Begin()
	tx.Exec(`INSERT INTO transactions (account_id, transaction_type, amount, admin_fee, description) VALUES (?, ?, ?, ?, ?)`, t.AccountID, t.Type, t.Amount, t.AdminFee, t.Desc)
	
	if t.Type == "INCOME" {
		tx.Exec("UPDATE accounts SET balance = balance + ? WHERE id = ?", t.Amount, t.AccountID)
	} else if t.Type == "EXPENSE" {
		tx.Exec("UPDATE accounts SET balance = balance - ? WHERE id = ?", t.Amount+t.AdminFee, t.AccountID)
	} else if t.Type == "TRANSFER" {
		tx.Exec("UPDATE accounts SET balance = balance - ? WHERE id = ?", t.Amount+t.AdminFee, t.AccountID)
		tx.Exec("UPDATE accounts SET balance = balance + ? WHERE id = ?", t.Amount, *t.TargetAccountID)
		incDesc := fmt.Sprintf("Transfer masuk (Ref: %s)", t.Desc)
		tx.Exec(`INSERT INTO transactions (account_id, transaction_type, amount, admin_fee, description) VALUES (?, 'INCOME', ?, 0, ?)`, *t.TargetAccountID, t.Amount, incDesc)
	}
	tx.Commit()
	w.WriteHeader(http.StatusCreated)
}

func handleDeleteTransaction(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	tx, _ := db.Begin()
	var accID int
	var tType string
	var amount, adminFee float64
	
	err := tx.QueryRow("SELECT account_id, transaction_type, amount, admin_fee FROM transactions WHERE id = ?", idStr).Scan(&accID, &tType, &amount, &adminFee)
	if err == nil {
		if tType == "INCOME" {
			tx.Exec("UPDATE accounts SET balance = balance - ? WHERE id = ?", amount, accID)
		} else {
			tx.Exec("UPDATE accounts SET balance = balance + ? WHERE id = ?", amount+adminFee, accID)
		}
		tx.Exec("DELETE FROM transactions WHERE id = ?", idStr)
	}
	tx.Commit()
	w.WriteHeader(http.StatusOK)
}

func handleBill(w http.ResponseWriter, r *http.Request) {
	var b Bill
	json.NewDecoder(r.Body).Decode(&b)
	if b.Tenor > 0 {
		b.MonthlyAmount = b.TotalAmount / float64(b.Tenor)
	}
	db.Exec("INSERT INTO bills (platform, item_name, monthly_amount, tenor, total_amount) VALUES (?, ?, ?, ?, ?)", b.Platform, b.ItemName, b.MonthlyAmount, b.Tenor, b.TotalAmount)
	w.WriteHeader(http.StatusCreated)
}

func handleDeleteBill(w http.ResponseWriter, r *http.Request) {
	db.Exec("DELETE FROM bills WHERE id = ?", r.URL.Query().Get("id"))
	w.WriteHeader(http.StatusOK)
}

func handlePayBill(w http.ResponseWriter, r *http.Request) {
	var req PayBillRequest
	json.NewDecoder(r.Body).Decode(&req)
	tx, _ := db.Begin()
	var platform, itemName string
	var currentTotal float64
	var tenor int
	err := tx.QueryRow("SELECT platform, item_name, total_amount, tenor FROM bills WHERE id = ?", req.BillID).Scan(&platform, &itemName, &currentTotal, &tenor)
	if err != nil {
		tx.Rollback()
		http.Error(w, "Not found", 404)
		return
	}
	desc := fmt.Sprintf("Cicilan %s - %s", platform, itemName)
	tx.Exec("INSERT INTO transactions (account_id, transaction_type, amount, admin_fee, description) VALUES (?, 'EXPENSE', ?, 0, ?)", req.AccountID, req.PaidAmount, desc)
	tx.Exec("UPDATE accounts SET balance = balance - ? WHERE id = ?", req.PaidAmount, req.AccountID)
	newTotal := currentTotal - req.PaidAmount
	newTenor := tenor - 1
	if newTenor <= 0 || newTotal <= 0 {
		tx.Exec("DELETE FROM bills WHERE id = ?", req.BillID)
	} else {
		newMonthly := newTotal / float64(newTenor)
		tx.Exec("UPDATE bills SET tenor = ?, total_amount = ?, monthly_amount = ? WHERE id = ?", newTenor, newTotal, newMonthly, req.BillID)
	}
	tx.Commit()
	w.WriteHeader(http.StatusOK)
}

func handlePiutang(w http.ResponseWriter, r *http.Request) {
	var p Piutang
	json.NewDecoder(r.Body).Decode(&p)
	db.Exec("INSERT INTO piutang (name, amount) VALUES (?, ?)", p.Name, p.Amount)
	w.WriteHeader(http.StatusCreated)
}

func handleDeletePiutang(w http.ResponseWriter, r *http.Request) {
	db.Exec("DELETE FROM piutang WHERE id = ?", r.URL.Query().Get("id"))
	w.WriteHeader(http.StatusOK)
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var data DashboardData
	var totalBal, totalBill, totalPiutang float64

	rowsAcc, err := db.Query("SELECT id, bank_name, balance FROM accounts")
	if err == nil {
		for rowsAcc.Next() {
			var acc Account
			rowsAcc.Scan(&acc.ID, &acc.Name, &acc.Balance)
			data.Balances = append(data.Balances, acc)
			totalBal += acc.Balance
		}
		rowsAcc.Close()
	}

	rowsBill, err := db.Query("SELECT id, platform, item_name, monthly_amount, tenor, total_amount FROM bills")
	if err == nil {
		for rowsBill.Next() {
			var b Bill
			rowsBill.Scan(&b.ID, &b.Platform, &b.ItemName, &b.MonthlyAmount, &b.Tenor, &b.TotalAmount)
			data.Bills = append(data.Bills, b)
			totalBill += b.TotalAmount
		}
		rowsBill.Close()
	}

	rowsPiutang, err := db.Query("SELECT id, name, amount FROM piutang")
	if err == nil {
		for rowsPiutang.Next() {
			var p Piutang
			rowsPiutang.Scan(&p.ID, &p.Name, &p.Amount)
			data.Piutangs = append(data.Piutangs, p)
			totalPiutang += p.Amount
		}
		rowsPiutang.Close()
	}

	rowsTx, err := db.Query("SELECT t.id, a.bank_name, t.transaction_type, t.amount, t.admin_fee, t.description, DATE_FORMAT(t.created_at, '%Y-%m-%d %H:%i') FROM transactions t JOIN accounts a ON t.account_id = a.id ORDER BY t.created_at DESC LIMIT 50")
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