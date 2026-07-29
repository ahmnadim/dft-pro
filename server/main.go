package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func main() {
	// Initialize database
	initDB("dft_server.db")
	defer db.Close()

	mux := http.NewServeMux()

	// Public APIs
	mux.HandleFunc("POST /api/register", handleRegister)
	mux.HandleFunc("POST /api/login", handleLogin)
	mux.HandleFunc("POST /api/check-license", handleCheckLicense)
	mux.HandleFunc("POST /api/charge", handleChargeOperation)
	mux.HandleFunc("GET /api/update/check", handleUpdateCheck)

	// Admin APIs & UI (Basic Auth Protected)
	mux.HandleFunc("GET /admin", basicAuth(handleAdminDashboard))
	mux.HandleFunc("GET /api/admin/users", basicAuth(handleAdminGetUsers))
	mux.HandleFunc("POST /api/admin/toggle", basicAuth(handleAdminToggleUser))
	mux.HandleFunc("POST /api/admin/balance", basicAuth(handleAdminUpdateBalance))

	// CORS wrapper
	corsHandler := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	port := ":8080"
	log.Printf("DFT Licensing Server listening on http://localhost%s", port)
	log.Printf("Admin panel access: http://localhost%s/admin (user: admin@dft.com, pass: admin123)", port)
	if err := http.ListenAndServe(port, corsHandler(mux)); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// Basic Authentication Middleware for Admins
func basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="Admin Area"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Basic" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		payload, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		credentials := strings.SplitN(string(payload), ":", 2)
		if len(credentials) != 2 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		email, password := credentials[0], credentials[1]

		// Authenticate via database check
		user, err := loginUser(email, password)
		if err != nil || !user.IsAdmin || !user.IsActive {
			log.Printf("[Admin Auth Failure] email=%s, err=%v, isAdmin=%v", email, err, user != nil && user.IsAdmin)
			w.Header().Set("WWW-Authenticate", `Basic realm="Admin Area"`)
			http.Error(w, "Unauthorized: Invalid admin credentials", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// Request / Response Helpers
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func readJSON(r *http.Request, dst interface{}) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

// API Handlers
type AuthRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LicenseCheckRequest struct {
	Email string `json:"email"`
	HWID  string `json:"hwid"`
}

type ChargeRequest struct {
	Email       string  `json:"email"`
	HWID        string  `json:"hwid"`
	Amount      float64 `json:"amount"`
	Description string  `json:"description"`
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Email == "" || len(req.Password) < 6 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Email is required and password must be at least 6 characters"})
		return
	}

	u, err := registerUser(req.Email, req.Password)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, u)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	u, err := loginUser(req.Email, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, u)
}

func handleCheckLicense(w http.ResponseWriter, r *http.Request) {
	var req LicenseCheckRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	// Verify user exists and is active
	var user User
	var isActiveInt int
	err := db.QueryRow("SELECT id, email, wallet_balance, is_active FROM users WHERE email = ?", req.Email).
		Scan(&user.ID, &user.Email, &user.WalletBalance, &isActiveInt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
		return
	}
	user.IsActive = isActiveInt == 1

	if !user.IsActive {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Your account has been deactivated by admin"})
		return
	}

	if req.HWID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "HWID parameter is required"})
		return
	}

	// Check HWID or register new (registers 7-day trial)
	hwidRec, err := checkOrRegisterHWID(user.ID, req.HWID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "HWID validation error"})
		return
	}

	now := time.Now()
	trialExpired := now.After(hwidRec.TrialExpiresAt)
	daysRemaining := int(time.Until(hwidRec.TrialExpiresAt).Hours() / 24)
	if daysRemaining < 0 {
		daysRemaining = 0
	}

	// Access criteria: active trial OR has positive wallet balance
	hasAccess := !trialExpired || user.WalletBalance > 0.0

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"email":                  user.Email,
		"hwid":                   hwidRec.HWID,
		"wallet_balance":         user.WalletBalance,
		"trial_expires_at":       hwidRec.TrialExpiresAt.Format(time.RFC3339),
		"trial_expired":          trialExpired,
		"trial_days_remaining":   daysRemaining,
		"has_access":             hasAccess,
		"is_active":              user.IsActive,
	})
}

func handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"update_available": false,
		"latest_version":   "1.0.0",
		"download_url":     "http://localhost:8080/downloads/dft-setup.exe",
		"release_notes":    "Initial stable release with dynamic USB mapping, security checks, and wallet-based trials.",
	})
}

func handleChargeOperation(w http.ResponseWriter, r *http.Request) {
	var req ChargeRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	var user User
	var isActiveInt int
	err := db.QueryRow("SELECT id, email, wallet_balance, is_active FROM users WHERE email = ?", req.Email).
		Scan(&user.ID, &user.Email, &user.WalletBalance, &isActiveInt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
		return
	}
	if isActiveInt == 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Account deactivated"})
		return
	}

	// Deduct balance
	err = updateUserBalance(user.ID, -req.Amount, req.Description)
	if err != nil {
		writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": err.Error()})
		return
	}

	// Fetch updated balance
	var updatedBalance float64
	_ = db.QueryRow("SELECT wallet_balance FROM users WHERE id = ?", user.ID).Scan(&updatedBalance)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":        true,
		"email":          user.Email,
		"charged":        req.Amount,
		"wallet_balance": updatedBalance,
	})
}

// Admin API Handlers
func handleAdminGetUsers(w http.ResponseWriter, r *http.Request) {
	users, err := getUsersList()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, users)
}

type ToggleRequest struct {
	UserID int64 `json:"user_id"`
	Active bool  `json:"active"`
}

func handleAdminToggleUser(w http.ResponseWriter, r *http.Request) {
	var req ToggleRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	err := toggleUserStatus(req.UserID, req.Active)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

type BalanceUpdateRequest struct {
	UserID      int64   `json:"user_id"`
	Amount      float64 `json:"amount"`
	Description string  `json:"description"`
}

func handleAdminUpdateBalance(w http.ResponseWriter, r *http.Request) {
	var req BalanceUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	if req.Description == "" {
		req.Description = "Admin Adjustment"
	}

	err := updateUserBalance(req.UserID, req.Amount, req.Description)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// Serve HTML Admin panel UI
func handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, adminDashboardHTML)
}

const adminDashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>DFT Pro - Admin Control Center</title>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;600;800&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-color: #0d0e15;
            --card-bg: #151824;
            --accent-purple: #8b5cf6;
            --accent-blue: #3b82f6;
            --neon-green: #10b981;
            --neon-red: #ef4444;
            --text-main: #f3f4f6;
            --text-muted: #9ca3af;
            --border-color: #24293f;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
            font-family: 'Outfit', sans-serif;
        }

        body {
            background-color: var(--bg-color);
            color: var(--text-main);
            padding: 2rem;
            min-height: 100vh;
        }

        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 2rem;
            border-bottom: 1px solid var(--border-color);
            padding-bottom: 1rem;
        }

        h1 {
            font-size: 2.2rem;
            font-weight: 800;
            background: linear-gradient(135deg, #a78bfa, #60a5fa);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }

        .badge-admin {
            background: rgba(139, 92, 246, 0.15);
            color: #c084fc;
            padding: 0.3rem 0.8rem;
            border-radius: 9999px;
            font-size: 0.85rem;
            border: 1px solid rgba(139, 92, 246, 0.3);
        }

        .container {
            max-width: 1200px;
            margin: 0 auto;
        }

        .card {
            background-color: var(--card-bg);
            border: 1px solid var(--border-color);
            border-radius: 12px;
            padding: 1.5rem;
            box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3);
            margin-bottom: 2rem;
        }

        .card-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 1.5rem;
        }

        .card-title {
            font-size: 1.25rem;
            font-weight: 600;
            color: var(--text-main);
        }

        table {
            width: 100%;
            border-collapse: collapse;
            text-align: left;
            margin-top: 1rem;
        }

        th, td {
            padding: 1rem;
            border-bottom: 1px solid var(--border-color);
        }

        th {
            color: var(--text-muted);
            font-weight: 600;
            font-size: 0.9rem;
            text-transform: uppercase;
        }

        tr:hover {
            background-color: rgba(255, 255, 255, 0.02);
        }

        .status-badge {
            padding: 0.25rem 0.6rem;
            border-radius: 6px;
            font-size: 0.8rem;
            font-weight: 600;
        }

        .status-active {
            background-color: rgba(16, 185, 129, 0.15);
            color: var(--neon-green);
        }

        .status-inactive {
            background-color: rgba(239, 68, 68, 0.15);
            color: var(--neon-red);
        }

        .btn {
            padding: 0.5rem 1rem;
            border-radius: 6px;
            border: none;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.2s ease;
            font-size: 0.85rem;
        }

        .btn-purple {
            background-color: var(--accent-purple);
            color: white;
        }

        .btn-purple:hover {
            opacity: 0.9;
            box-shadow: 0 0 10px rgba(139, 92, 246, 0.4);
        }

        .btn-outline {
            background-color: transparent;
            border: 1px solid var(--border-color);
            color: var(--text-main);
        }

        .btn-outline:hover {
            border-color: var(--text-muted);
            background-color: rgba(255, 255, 255, 0.05);
        }

        /* Modal styling */
        .modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background-color: rgba(0, 0, 0, 0.7);
            backdrop-filter: blur(4px);
            align-items: center;
            justify-content: center;
            z-index: 100;
        }

        .modal-content {
            background-color: var(--card-bg);
            border: 1px solid var(--border-color);
            border-radius: 12px;
            width: 400px;
            padding: 2rem;
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.5);
        }

        .modal-title {
            margin-bottom: 1.5rem;
            font-size: 1.3rem;
        }

        .form-group {
            margin-bottom: 1.25rem;
        }

        .form-group label {
            display: block;
            margin-bottom: 0.5rem;
            color: var(--text-muted);
            font-size: 0.9rem;
        }

        .form-control {
            width: 100%;
            padding: 0.75rem;
            background-color: var(--bg-color);
            border: 1px solid var(--border-color);
            border-radius: 6px;
            color: white;
            font-size: 1rem;
        }

        .form-control:focus {
            outline: none;
            border-color: var(--accent-purple);
        }

        .modal-actions {
            display: flex;
            justify-content: flex-end;
            gap: 1rem;
            margin-top: 2rem;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div>
                <h1>DFT Pro Server</h1>
                <p style="color: var(--text-muted); font-size: 0.9rem; margin-top: 0.2rem;">Licensing & Wallet Management Console</p>
            </div>
            <span class="badge-admin">Admin Dashboard</span>
        </header>

        <div class="card">
            <div class="card-header">
                <h2 class="card-title">Registered Users</h2>
            </div>
            <table>
                <thead>
                    <tr>
                        <th>User ID</th>
                        <th>Email</th>
                        <th>Wallet Balance</th>
                        <th>Role</th>
                        <th>Status</th>
                        <th>Registered At</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody id="userTableBody">
                    <tr>
                        <td colspan="7" style="text-align: center; color: var(--text-muted);">Loading user directory...</td>
                    </tr>
                </tbody>
            </table>
        </div>
    </div>

    <!-- Balance Adjustment Modal -->
    <div id="balanceModal" class="modal">
        <div class="modal-content">
            <h3 class="modal-title">Adjust Wallet Balance</h3>
            <input type="hidden" id="modalUserId">
            <div class="form-group">
                <label for="balanceAmount">Amount to Add/Remove (e.g., 50.00 or -20.00)</label>
                <input type="number" id="balanceAmount" class="form-control" placeholder="0.00" step="0.01">
            </div>
            <div class="form-group">
                <label for="balanceDesc">Reason / Description</label>
                <input type="text" id="balanceDesc" class="form-control" placeholder="e.g. Reseller Topup, Refund">
            </div>
            <div class="modal-actions">
                <button class="btn btn-outline" onclick="closeBalanceModal()">Cancel</button>
                <button class="btn btn-purple" onclick="submitBalanceAdjustment()">Apply</button>
            </div>
        </div>
    </div>

    <script>
        async function fetchUsers() {
            try {
                const response = await fetch('/api/admin/users', {
                    headers: { 'Authorization': getAuthHeader() }
                });
                if (!response.ok) throw new Error("Unauthorized");
                const users = await response.json();
                renderUsers(users);
            } catch (err) {
                console.error(err);
                document.getElementById('userTableBody').innerHTML = 
                    '<tr><td colspan="7" style="text-align: center; color: var(--neon-red);">Authentication failed or server unreachable.</td></tr>';
            }
        }

        function getAuthHeader() {
            // Decodes from basic auth header used to load page
            return localStorage.getItem('dft_admin_auth') || '';
        }

        function renderUsers(users) {
            const tbody = document.getElementById('userTableBody');
            tbody.innerHTML = '';

            if (!users || users.length === 0) {
                tbody.innerHTML = '<tr><td colspan="7" style="text-align: center; color: var(--text-muted);">No users found.</td></tr>';
                return;
            }

            users.forEach(user => {
                const tr = document.createElement('tr');
                const formattedDate = new Date(user.created_at).toLocaleDateString();

                tr.innerHTML = '<td>#' + user.id + '</td>' +
                    '<td>' + user.email + '</td>' +
                    '<td style="font-weight: 600; color: ' + (user.wallet_balance > 0 ? 'var(--neon-green)' : 'var(--text-muted)') + '">$' + user.wallet_balance.toFixed(2) + '</td>' +
                    '<td>' + (user.is_admin ? '<span style="color: var(--accent-purple); font-weight:600;">Admin</span>' : 'User') + '</td>' +
                    '<td>' +
                        '<span class="status-badge ' + (user.is_active ? 'status-active' : 'status-inactive') + '">' +
                            (user.is_active ? 'Active' : 'Deactivated') +
                        '</span>' +
                    '</td>' +
                    '<td>' + formattedDate + '</td>' +
                    '<td>' +
                        '<div style="display: flex; gap: 0.5rem;">' +
                            '<button class="btn btn-outline" onclick="openBalanceModal(' + user.id + ')">Adjust Wallet</button>' +
                            '<button class="btn ' + (user.is_active ? 'btn-outline' : 'btn-purple') + '" onclick="toggleUser(' + user.id + ', ' + !user.is_active + ')">' +
                                (user.is_active ? 'Deactivate' : 'Activate') +
                            '</button>' +
                        '</div>' +
                    '</td>';
                tbody.appendChild(tr);
            });
        }

        async function toggleUser(userId, newStatus) {
            try {
                const response = await fetch('/api/admin/toggle', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': getAuthHeader()
                    },
                    body: JSON.stringify({ user_id: userId, active: newStatus })
                });
                if (response.ok) {
                    fetchUsers();
                } else {
                    alert("Failed to update user status.");
                }
            } catch (err) {
                alert("Server request failed.");
            }
        }

        function openBalanceModal(userId) {
            document.getElementById('modalUserId').value = userId;
            document.getElementById('balanceAmount').value = '';
            document.getElementById('balanceDesc').value = '';
            document.getElementById('balanceModal').style.display = 'flex';
        }

        function closeBalanceModal() {
            document.getElementById('balanceModal').style.display = 'none';
        }

        async function submitBalanceAdjustment() {
            const userId = parseInt(document.getElementById('modalUserId').value);
            const amount = parseFloat(document.getElementById('balanceAmount').value);
            const desc = document.getElementById('balanceDesc').value;

            if (isNaN(amount)) {
                alert("Please enter a valid amount.");
                return;
            }

            try {
                const response = await fetch('/api/admin/balance', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': getAuthHeader()
                    },
                    body: JSON.stringify({ user_id: userId, amount: amount, description: desc })
                });

                if (response.ok) {
                    closeBalanceModal();
                    fetchUsers();
                } else {
                    const data = await response.json();
                    alert(data.error || "Failed to adjust balance.");
                }
            } catch (err) {
                alert("Server request failed.");
            }
        }

        // Auto-save basic auth header in localStorage for API requests
        const auth = window.performance ? '' : ''; // Dummy trigger
        // Extract browser auth header
        fetch('/api/admin/users').then(res => {
            if (res.status === 401) {
                // Let browser prompt basic auth
            }
        });

        // Setup manual interceptor for standard login
        // Since we are using standard browser basic auth, it is automatically cached by the browser for this session.
        // We will pass empty auth to fallback on browser-managed headers if not set.
        window.onload = () => {
            fetchUsers();
        }
    </script>
</body>
</html>
`
