<p align="center">
  <h1 align="center">💸 CashFlow - Financial Intelligence</h1>
  <p align="center">Aplikasi manajemen keuangan pribadi modern dengan performa tinggi & keamanan tingkat lanjut.</p>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Golang" />
  <img src="https://img.shields.io/badge/MySQL-4479A1?style=for-the-badge&logo=mysql&logoColor=white" alt="MySQL" />
  <img src="https://img.shields.io/badge/Tailwind-06B6D4?style=for-the-badge&logo=tailwindcss&logoColor=white" alt="Tailwind" />
  <img src="https://img.shields.io/badge/Railway-131313?style=for-the-badge&logo=railway&logoColor=white" alt="Railway" />
</p>

---

## 📖 About
**CashFlow** adalah solusi cerdas untuk mengelola keuangan pribadi. Dibangun dengan bahasa pemrograman Go (Golang) untuk performa backend yang super cepat, dikombinasikan dengan antarmuka yang elegan menggunakan Tailwind CSS. Proyek ini dirancang untuk mereka yang menginginkan aplikasi finansial yang ringan, aman, dan dapat diandalkan.

## ✨ Key Features
- **Modern Authentication:** Login aman via **Google OAuth2** atau sistem registrasi standar dengan enkripsi password (Bcrypt).
- **Security First:** Dilindungi dengan *Rate Limiting* untuk mencegah serangan *Brute Force* dan *Spam*.
- **Comprehensive Financial Tracking:**
  - Pemasukan & Pengeluaran.
  - Transfer antar dompet (dengan perhitungan biaya admin).
  - Manajemen cicilan/tagihan (Bills) dengan tenor otomatis.
  - Pencatatan piutang.
- **Smart Dashboard:** Ringkasan aset bersih (*Net Balance*) dan total grand total keuangan dalam satu pandangan.
- **Responsive UI:** Tampilan *dark mode* yang elegan dengan animasi *splash screen* 6 detik yang profesional.

## 🛠 Tech Stack
| Component | Technology |
| :--- | :--- |
| **Backend** | Go (Golang), net/http, jwt-go |
| **Database** | MySQL (Railway) |
| **Security** | Bcrypt, Rate Limiting, Google OAuth2 |
| **Frontend** | HTML5, Tailwind CSS |
| **Deployment** | Railway.app |

## 🚀 Preview
*(Masukkan GIF atau Screenshot dashboard-mu di sini untuk menarik perhatian!)*
<!-- Contoh: ![Dashboard](path/ke/gambar-mu.gif) -->

## ⚙️ Installation
1. **Clone Repository**
   ```bash
   git clone [https://github.com/username/cashflow.git](https://github.com/username/cashflow.git)
   cd cashflow
