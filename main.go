package main

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-sql-driver/mysql"
)

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

var db *sql.DB

func main() {
	// Daftarkan custom TLS config untuk mengatasi error x509 certificate di Cloud
	mysql.RegisterTLSConfig("custom", &tls.Config{
		InsecureSkipVerify: true,
	})

	// Ambil koneksi database dari Environment Variable (Railway / Aiven)
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

	// Tes koneksi untuk memastikan berhasil
	err = db.Ping()
	if err != nil {
		log.Fatal("Database tidak bisa di-ping:", err)
	}
	log.Println("Berhasil terhubung ke database Cloud Aiven!")

	// TAMPILKAN FILE HTML KE INTERNET
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	http.HandleFunc("/api/transaction", handleTransaction)
	http.HandleFunc("/api/transaction/delete", handleDeleteTransaction)
	http.HandleFunc("/api/history", handleHistory)
	http.HandleFunc("/api/bill", handleBill)
	http.HandleFunc("/api/bill/delete", handleDeleteBill)
	http.HandleFunc("/api/bill/pay", handlePayBill)
	http.HandleFunc("/api/piutang", handlePiutang)
	http.HandleFunc("/api/piutang/delete", handleDeletePiutang)

	// Ambil Port dari Environment (Untuk Railway)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Server berjalan di port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleTransaction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		return
	}

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
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "DELETE, OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}

	idStr := r.URL.Query().Get("id")
	tx, _ := db.Begin()
	var accID int
	var tType string
	var amount, adminFee float64
	tx.QueryRow("SELECT account_id, transaction_type, amount, admin_fee FROM transactions WHERE id = ?", idStr).Scan(&accID, &tType, &amount, &adminFee)
	if tType == "INCOME" {
		tx.Exec("UPDATE accounts SET balance = balance - ? WHERE id = ?", amount, accID)
	} else {
		tx.Exec("UPDATE accounts SET balance = balance + ? WHERE id = ?", amount+adminFee, accID)
	}
	tx.Exec("DELETE FROM transactions WHERE id = ?", idStr)
	tx.Commit()
	w.WriteHeader(http.StatusOK)
}

func handleBill(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		return
	}
	var b Bill
	json.NewDecoder(r.Body).Decode(&b)

	if b.Tenor > 0 {
		b.MonthlyAmount = b.TotalAmount / float64(b.Tenor)
	}

	db.Exec("INSERT INTO bills (platform, item_name, monthly_amount, tenor, total_amount) VALUES (?, ?, ?, ?, ?)", b.Platform, b.ItemName, b.MonthlyAmount, b.Tenor, b.TotalAmount)
	w.WriteHeader(http.StatusCreated)
}

func handleDeleteBill(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "DELETE, OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}
	db.Exec("DELETE FROM bills WHERE id = ?", r.URL.Query().Get("id"))
	w.WriteHeader(http.StatusOK)
}

func handlePayBill(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		return
	}

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
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		return
	}
	var p Piutang
	json.NewDecoder(r.Body).Decode(&p)
	db.Exec("INSERT INTO piutang (name, amount) VALUES (?, ?)", p.Name, p.Amount)
	w.WriteHeader(http.StatusCreated)
}

func handleDeletePiutang(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "DELETE, OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}
	db.Exec("DELETE FROM piutang WHERE id = ?", r.URL.Query().Get("id"))
	w.WriteHeader(http.StatusOK)
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	var data DashboardData
	var totalBal, totalBill, totalPiutang float64

	rowsAcc, _ := db.Query("SELECT id, bank_name, balance FROM accounts")
	for rowsAcc.Next() {
		var acc Account
		rowsAcc.Scan(&acc.ID, &acc.Name, &acc.Balance)
		data.Balances = append(data.Balances, acc)
		totalBal += acc.Balance
	}

	rowsBill, _ := db.Query("SELECT id, platform, item_name, monthly_amount, tenor, total_amount FROM bills")
	for rowsBill.Next() {
		var b Bill
		rowsBill.Scan(&b.ID, &b.Platform, &b.ItemName, &b.MonthlyAmount, &b.Tenor, &b.TotalAmount)
		data.Bills = append(data.Bills, b)
		totalBill += b.TotalAmount
	}

	rowsPiutang, _ := db.Query("SELECT id, name, amount FROM piutang")
	for rowsPiutang.Next() {
		var p Piutang
		rowsPiutang.Scan(&p.ID, &p.Name, &p.Amount)
		data.Piutangs = append(data.Piutangs, p)
		totalPiutang += p.Amount
	}

	rowsTx, _ := db.Query("SELECT t.id, a.bank_name, t.transaction_type, t.amount, t.admin_fee, t.description, t.created_at FROM transactions t JOIN accounts a ON t.account_id = a.id ORDER BY t.created_at DESC LIMIT 50")
	for rowsTx.Next() {
		var tx TransactionRow
		rowsTx.Scan(&tx.ID, &tx.BankName, &tx.Type, &tx.Amount, &tx.AdminFee, &tx.Desc, &tx.CreatedAt)
		data.Transactions = append(data.Transactions, tx)
	}

	data.TotalBalance = totalBal
	data.TotalBill = totalBill
	data.TotalPiutang = totalPiutang
	data.NetBalance = totalBal - totalBill
	data.GrandTotal = (totalBal - totalBill) + totalPiutang
	json.NewEncoder(w).Encode(data)
}