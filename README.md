# 💰 Personal Cashflow & Financial Dashboard

Aplikasi web manajemen keuangan pribadi (Cashflow) berbasis **Golang (Go)** dan **Tailwind CSS**. Dilengkapi dengan sistem autentikasi multi-user, pencatatan transaksi masuk/keluar, transfer antar akun, manajemen cicilan/tagihan, catatan piutang, sapaan personal, hingga widget motivasi menabung otomatis dari AI.

---

## ✨ Fitur Utama

- **Sistem Autentikasi Pengguna:** Login dan Registrasi akun personal dengan penyimpanan sesi aman.
- **Dashboard Finansial Komprehensif:** Kartu ringkasan Total Saldo Bersih, Saldo Bank, Total Tagihan/Cicilan, dan Total Piutang.
- **Pencatatan Transaksi:** Input Pemasukan, Pengeluaran, dan Transfer antar akun lengkap dengan biaya admin.
- **Manajemen Tagihan & Piutang:** Melacak cicilan bulanan, paylater, serta daftar nama peminjam.

---

## 📸 Preview Tampilan Aplikasi

### 1. Halaman Login / Registrasi
![Preview Login](login.png)

### 2. Halaman Dashboard Utama
![Preview Dashboard](dashboard.png)

*(Catatan: Pastikan file gambar screenshot kamu di dalam folder project bernama `login.png` dan `dashboard.png`, atau sesuaikan dengan nama file gambarmu).*

---

## 🛠️ Teknologi yang Digunakan

- **Backend:** Go (Golang) (`net/http`, `database/sql`)
- **Database:** MySQL / Aiven Cloud (`go-sql-driver/mysql`)
- **Keamanan:** Golang Crypto (`golang.org/x/crypto/bcrypt`)
- **Frontend:** HTML5, Tailwind CSS (via CDN v4)

---

## ⚙️ Cara Menjalankan Project (Local Installation)

1. **Siapkan Database MySQL:** Pastikan XAMPP MySQL aktif, lalu buat database baru bernama **`cashflow_db`**.
2. **Install Dependencies Go:** 
   ```bash
   go mod tidy
