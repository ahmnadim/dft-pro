# DFT Pro - Complete Setup, Build & Installation Guide

This document explains how to start, build, and deploy both the **DFT Licensing Server** and the **DFT Desktop Client** across different operating systems (Windows, Linux, and macOS).

---

## 1. Prerequisites

Ensure you have the following installed on your machine:

- **Go** (v1.20 or newer): [https://go.dev/doc/install](https://go.dev/doc/install)
- **Node.js** (v18 or newer) & **npm**: [https://nodejs.org/](https://nodejs.org/)
- **Wails CLI** (v2.x):
  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  ```

---

## 2. How to Start the Licensing Server (Backend)

The licensing server handles user registration, hardware fingerprint (HWID) binding, 7-day trial calculations, wallet credit charges, and hosts the Admin Control Panel.

### Steps to Run:
1. Open a terminal and navigate to the `server/` directory:
   ```bash
   cd /media/nadim/Personal2/Nadim/work/app/desktop/dft/server
   ```
2. Start the server:
   ```bash
   go run .
   ```
3. **Admin Control Center**:
   Open your browser and navigate to:
   👉 **`http://localhost:8080/admin`**
   - **Default Admin Email:** `admin@dft.com`
   - **Default Admin Password:** `admin123`

---

## 3. How to Run the App in Development Mode

> **Note:** Make sure the Licensing Server (`go run .` in `server/`) is running before starting the client.

### Method A: Browser Development Mode (All OS)
Works instantly on any system without needing native GTK graphics libraries.

1. Navigate to the `client/` directory:
   ```bash
   cd /media/nadim/Personal2/Nadim/work/app/desktop/dft/client
   ```
2. Run Wails development mode:
   ```bash
   wails dev
   ```
3. Open your browser and go to:
   👉 **`http://localhost:34115`** (or `http://localhost:5173`)

### Method B: Native Window Mode (Linux/Ubuntu)
To run as a native desktop application window on Linux, install GTK dependencies first:

```bash
# Install Ubuntu GTK & WebKit development packages
sudo apt update
sudo apt install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.0-dev

# Start Wails dev window
cd client
wails dev
```

---

## 4. How to Build & Install for Different OS

### A. Building & Running on Windows (`.exe`)

> **Note:** `.exe` binary files are excluded from Git by `.gitignore`. After cloning the repo on Windows, generate the `.exe` with a single command below.

#### Step 1: Generate the `.exe` on Windows (Command Prompt / PowerShell)
Open PowerShell or CMD inside the cloned `dft` folder:
```cmd
cd client
go build -tags desktop,production -o dft-client.exe
```

#### Step 2: Run the App
Double-click **`dft-client.exe`** inside the `client/` folder to launch the standalone desktop app! No installation wizard is needed as it is a self-contained portable Windows application.

---

#### Alternative: Cross-Compile from Linux/macOS
If compiling on Linux for Windows:
```bash
cd client
cd frontend && npm run build && cd ..
GOOS=windows GOARCH=amd64 go build -tags desktop,production -o dft-client.exe
```

#### Option 2: Using Wails CLI
```bash
cd client
wails build -platform windows/amd64
```
The compiled file will be generated in `client/build/bin/dft-client.exe`.

---

### B. Building for Linux (Native Binary / Package)

1. Ensure GTK dependencies are installed:
   ```bash
   sudo apt install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.0-dev
   ```
2. Run Wails build:
   ```bash
   cd client
   wails build -platform linux/amd64
   ```
The output binary will be created at `client/build/bin/dft-client`.

---

### C. Building for macOS (`.app` / `.dmg`)

To build for macOS (requires a Mac machine with Xcode Command Line Tools):

```bash
cd client

# For Apple Silicon (M1/M2/M3)
wails build -platform darwin/arm64

# For Intel Macs
wails build -platform darwin/amd64

# Universal Binary (Both Apple Silicon & Intel)
wails build -platform darwin/universal
```
The bundle will be generated under `client/build/bin/dft-client.app`.

---

## 5. Troubleshooting & FAQs

### Q1: "Address already in use" on port 8080
If the server fails to start because port 8080 is blocked by a previous background process, kill it using:
```bash
fuser -k 8080/tcp
```

### Q2: "Server Error: Unable to reach licensing server on port 8080"
Ensure you have launched the backend server first in `server/` via `go run .`.

### Q3: How to reset the database?
To reset all users, trial records, and wallet balances:
```bash
cd server
rm -f dft_server.db
go run .
```
The server will automatically re-seed the default admin user on startup (`admin@dft.com` / `admin123`).
