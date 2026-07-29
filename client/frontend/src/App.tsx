import React, { useState, useEffect, useRef } from 'react';
import { 
  GetHWID, 
  GetSavedEmail, 
  ClearSavedSession, 
  RegisterOnServer, 
  LoginOnServer, 
  CheckLicenseOnServer,
  PerformMockOperation
} from '../wailsjs/go/main/App';
import { 
  Cpu, 
  Smartphone, 
  Wallet, 
  Terminal as TermIcon, 
  Lock, 
  Unlock, 
  RefreshCw, 
  LogOut, 
  CreditCard, 
  Layers, 
  Settings, 
  CheckCircle, 
  AlertTriangle,
  Info
} from 'lucide-react';

interface LogEntry {
  timestamp: string;
  type: 'info' | 'success' | 'warning' | 'error';
  message: string;
}

interface LicenseInfo {
  email: string;
  hwid: string;
  wallet_balance: number;
  trial_expires_at: string;
  trial_expired: boolean;
  trial_days_remaining: number;
  has_access: boolean;
  is_active: boolean;
}

export default function App() {
  // Authentication & License States
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [isRegistering, setIsRegistering] = useState(false);
  const [loading, setLoading] = useState(false);
  const [authError, setAuthError] = useState('');
  const [hwid, setHwid] = useState('');

  const [isLoggedIn, setIsLoggedIn] = useState(false);
  const [license, setLicense] = useState<LicenseInfo | null>(null);

  // Tab & Control States
  const [activeTab, setActiveTab] = useState('dashboard');
  const [deviceState, setDeviceState] = useState<'disconnected' | 'adb' | 'fastboot' | 'edl'>('disconnected');
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [opLoading, setOpLoading] = useState(false);

  // Wallet top-up form state
  const [topupAmount, setTopupAmount] = useState('50');
  const [topupLoading, setTopupLoading] = useState(false);

  const consoleEndRef = useRef<HTMLDivElement>(null);

  // Load HWID and Check Session on Mount
  useEffect(() => {
    GetHWID().then(id => setHwid(id));
    
    GetSavedEmail().then(savedEmail => {
      if (savedEmail) {
        setEmail(savedEmail);
        verifyLicense(savedEmail);
      }
    });
  }, []);

  // Auto-scroll console logs
  useEffect(() => {
    consoleEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [logs]);

  const addLog = (message: string, type: 'info' | 'success' | 'warning' | 'error' = 'info') => {
    const timestamp = new Date().toLocaleTimeString();
    setLogs(prev => [...prev, { timestamp, type, message }]);
  };

  const verifyLicense = async (userEmail: string) => {
    try {
      addLog(`Checking license verification for ${userEmail}...`, 'info');
      const responseStr = await CheckLicenseOnServer(userEmail);
      const lic: LicenseInfo = JSON.parse(responseStr);
      setLicense(lic);
      setIsLoggedIn(true);
      setEmail(userEmail);
      if (lic.has_access) {
        addLog("License check verified. Access granted.", "success");
        if (lic.trial_expired) {
          addLog("7-day trial expired. Running on wallet credits.", "warning");
        } else {
          addLog(`7-day trial active: ${lic.trial_days_remaining} days remaining.`, "success");
        }
      } else {
        addLog("Access Denied: Trial expired and wallet balance is empty.", "error");
      }
    } catch (err: any) {
      const msg = err.message || err || "Unable to reach licensing server on port 8080.";
      addLog(`License check failed: ${msg}`, 'error');
      setAuthError(`Server Error: ${msg}. Make sure server is running on http://localhost:8080`);
    }
  };

  const handleAuth = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setAuthError('');
    try {
      if (isRegistering) {
        addLog(`Registering account ${email}...`, 'info');
        await RegisterOnServer(email, password);
        addLog("Registration successful! Attempting login...", "success");
      }
      addLog(`Authenticating ${email}...`, 'info');
      await LoginOnServer(email, password);
      addLog("Login successful.", "success");
      await verifyLicense(email);
    } catch (err: any) {
      const msg = err.message || err || "Authentication failed";
      setAuthError(msg);
      addLog(`Authentication error: ${msg}`, 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleLogout = async () => {
    await ClearSavedSession();
    setIsLoggedIn(false);
    setLicense(null);
    setEmail('');
    setPassword('');
    addLog("Session cleared. Logged out.", "info");
  };

  // Perform Device Operation
  const runOperation = async (opName: string, cost: number, actionSim: () => Promise<void>) => {
    if (!license?.has_access) {
      addLog("Operation aborted: license inactive.", "error");
      return;
    }
    if (deviceState === 'disconnected') {
      addLog("Error: No smartphone connected. Please connect a device first.", "error");
      return;
    }

    setOpLoading(true);
    addLog(`Initiating operation: ${opName} ($${cost.toFixed(2)})...`, 'info');
    
    try {
      // Deduct balance
      const chargeRes = await PerformMockOperation(email, opName, cost);
      const parsedRes = JSON.parse(chargeRes);
      
      // Update balance
      if (license) {
        setLicense({
          ...license,
          wallet_balance: parsedRes.wallet_balance
        });
      }

      addLog(`Authorization validated. Wallet charged: $${cost.toFixed(2)}.`, 'success');
      
      // Simulate physical flashing/servicing
      await actionSim();
      
      addLog(`Operation completed successfully: ${opName}`, 'success');
    } catch (err: any) {
      addLog(`Operation failed: ${err.message || err}`, 'error');
    } finally {
      setOpLoading(false);
    }
  };

  // Simulated Recharges for Testing
  const handleTopup = async () => {
    const amt = parseFloat(topupAmount);
    if (isNaN(amt) || amt <= 0) {
      alert("Invalid topup amount.");
      return;
    }
    setTopupLoading(true);
    addLog(`Sending wallet top-up request of $${amt.toFixed(2)} to central server...`, 'info');

    try {
      // Direct call to admin balance API using basic auth headers for mock topup convenience
      const adminAuth = btoa('admin@dft.com:admin123');
      const response = await fetch('http://localhost:8080/api/admin/balance', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Basic ${adminAuth}`
        },
        body: JSON.stringify({
          user_id: 1, // First registered user
          amount: amt,
          description: "User Topup via Desktop Client"
        })
      });

      if (response.ok) {
        addLog(`Top-up transaction success. Refreshing balance...`, 'success');
        await verifyLicense(email);
      } else {
        throw new Error("Failed to process payment");
      }
    } catch (err: any) {
      addLog(`Wallet topup failed: ${err.message}`, 'error');
    } finally {
      setTopupLoading(false);
    }
  };

  // Auth Screen
  if (!isLoggedIn) {
    return (
      <div className="auth-container">
        <div className="panel auth-card">
          <div className="auth-header">
            <div className="logo-glow">DFT PRO</div>
            <p style={{ color: 'var(--text-secondary)', fontSize: '0.9rem', marginTop: '0.4rem' }}>
              Device Flash & Servicing Utility
            </p>
          </div>

          <div className="auth-tabs">
            <div className={`auth-tab ${!isRegistering ? 'active' : ''}`} onClick={() => setIsRegistering(false)}>
              Login
            </div>
            <div className={`auth-tab ${isRegistering ? 'active' : ''}`} onClick={() => setIsRegistering(true)}>
              Register
            </div>
          </div>

          <form onSubmit={handleAuth}>
            <div className="form-group">
              <label>Email Address</label>
              <input 
                type="email" 
                className="form-control" 
                value={email} 
                onChange={e => setEmail(e.target.value)} 
                required 
                placeholder="user@example.com"
              />
            </div>

            <div className="form-group">
              <label>Password</label>
              <input 
                type="password" 
                className="form-control" 
                value={password} 
                onChange={e => setPassword(e.target.value)} 
                required 
                placeholder="••••••••"
              />
            </div>

            {authError && (
              <p style={{ color: 'var(--neon-rose)', fontSize: '0.85rem', marginBottom: '1rem', fontWeight: 600 }}>
                {authError}
              </p>
            )}

            <button type="submit" className="btn-primary" disabled={loading}>
              {loading ? 'Processing...' : isRegistering ? 'Create Account' : 'Authenticate'}
            </button>
          </form>

          <div style={{ marginTop: '1.5rem', borderTop: '1px solid var(--border-color)', paddingTop: '1rem', fontSize: '0.8rem', color: 'var(--text-secondary)' }}>
            <strong>Local HWID:</strong>
            <div style={{ wordBreak: 'break-all', fontFamily: 'monospace', background: '#090a10', padding: '0.5rem', borderRadius: '4px', marginTop: '0.25rem', border: '1px solid var(--border-color)' }}>
              {hwid}
            </div>
          </div>
        </div>
      </div>
    );
  }

  // Trial Expired Guard View
  if (license && !license.has_access) {
    return (
      <div className="auth-container">
        <div className="panel auth-card" style={{ border: '1px solid var(--neon-rose)' }}>
          <div style={{ textAlign: 'center', marginBottom: '1.5rem' }}>
            <AlertTriangle size={48} className="color-red" style={{ filter: 'drop-shadow(0 0 10px rgba(239,68,68,0.5))' }} />
            <h2 style={{ marginTop: '1rem', fontWeight: 800 }}>License Suspended</h2>
            <p style={{ color: 'var(--text-secondary)', fontSize: '0.9rem', marginTop: '0.2rem' }}>
              Your 7-day free trial has expired, and your wallet balance is empty ($0.00).
            </p>
          </div>

          <div className="form-group">
            <label>Current Wallet Balance</label>
            <div style={{ fontSize: '1.8rem', fontWeight: 800, color: 'var(--neon-rose)' }}>$0.00</div>
          </div>

          <div style={{ background: 'rgba(255,255,255,0.02)', border: '1px solid var(--border-color)', borderRadius: '8px', padding: '1rem', marginBottom: '1.5rem' }}>
            <label style={{ fontSize: '0.85rem', color: 'var(--text-secondary)' }}>Top Up Wallet (Test Portal)</label>
            <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
              <input 
                type="number" 
                className="form-control" 
                value={topupAmount} 
                onChange={e => setTopupAmount(e.target.value)} 
                placeholder="50"
              />
              <button className="btn-primary" style={{ width: 'auto' }} onClick={handleTopup} disabled={topupLoading}>
                {topupLoading ? 'Adding...' : 'Recharge'}
              </button>
            </div>
          </div>

          <div style={{ display: 'flex', gap: '1rem' }}>
            <button className="btn-primary btn-secondary" onClick={() => verifyLicense(email)}>
              <RefreshCw size={16} style={{ marginRight: '0.5rem' }} /> Refresh Status
            </button>
            <button className="btn-primary btn-secondary" onClick={handleLogout}>
              <LogOut size={16} style={{ marginRight: '0.5rem' }} /> Log Out
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="app-container">
      {/* Sidebar */}
      <div className="sidebar">
        <div className="sidebar-menu">
          <div className="logo-glow" style={{ fontSize: '1.8rem', marginBottom: '2rem', textAlign: 'center' }}>
            DFT PRO
          </div>
          
          <div className={`menu-item ${activeTab === 'dashboard' ? 'active' : ''}`} onClick={() => setActiveTab('dashboard')}>
            <Layers size={18} /> Dashboard
          </div>
          
          <div className={`menu-item ${activeTab === 'xiaomi' ? 'active' : ''}`} onClick={() => setActiveTab('xiaomi')}>
            <Cpu size={18} /> Xiaomi Tool
          </div>
          
          <div className={`menu-item ${activeTab === 'samsung' ? 'active' : ''}`} onClick={() => setActiveTab('samsung')}>
            <Cpu size={18} /> Samsung Tool
          </div>
          
          <div className={`menu-item ${activeTab === 'mediatek' ? 'active' : ''}`} onClick={() => setActiveTab('mediatek')}>
            <Cpu size={18} /> MediaTek MTK
          </div>
          
          <div className={`menu-item ${activeTab === 'wallet' ? 'active' : ''}`} onClick={() => setActiveTab('wallet')}>
            <Wallet size={18} /> Wallet & Billing
          </div>
        </div>

        <div>
          <div style={{ borderTop: '1px solid var(--border-color)', paddingTop: '1rem', marginBottom: '1rem', fontSize: '0.8rem', color: 'var(--text-secondary)' }}>
            <div>Active Profile:</div>
            <div style={{ color: 'white', fontWeight: 600, textOverflow: 'ellipsis', overflow: 'hidden', whiteSpace: 'nowrap' }}>{email}</div>
          </div>
          <button className="btn-primary btn-secondary" style={{ width: '100%' }} onClick={handleLogout}>
            <LogOut size={16} style={{ marginRight: '0.5rem' }} /> Disconnect
          </button>
        </div>
      </div>

      {/* Main Content Area */}
      <div className="main-content">
        {/* Top Header Panel */}
        <div className="topbar">
          <div>
            <span style={{ color: 'var(--text-secondary)', fontSize: '0.85rem' }}>Active Operation Hub</span>
            <h2 style={{ fontSize: '1.1rem', fontWeight: 600 }}>{activeTab.toUpperCase()} PANEL</h2>
          </div>

          <div className="topbar-right">
            {/* USB Connection State Selector */}
            <div className="status-indicator">
              <span style={{ color: 'var(--text-secondary)' }}>Device Mode:</span>
              <select 
                style={{ background: '#121422', color: 'white', border: '1px solid var(--border-color)', padding: '0.3rem 0.5rem', borderRadius: '4px', fontWeight: 600, outline: 'none' }}
                value={deviceState}
                onChange={e => {
                  const state = e.target.value as any;
                  setDeviceState(state);
                  addLog(`Smart Device Detection: State changed to ${state.toUpperCase()}`, 'info');
                }}
              >
                <option value="disconnected">No Device Detected</option>
                <option value="adb">ADB / Debugging Mode</option>
                <option value="fastboot">Fastboot Mode</option>
                <option value="edl">Emergency Download (EDL)</option>
              </select>
              <div className={`status-dot ${deviceState !== 'disconnected' ? 'color-green' : 'color-red'}`}></div>
            </div>

            {/* Current Wallet Quick Panel */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', background: 'rgba(139, 92, 246, 0.1)', padding: '0.4rem 0.8rem', borderRadius: '6px', border: '1px solid var(--border-color)' }}>
              <Wallet size={16} className="color-purple" />
              <span style={{ fontSize: '0.9rem', fontWeight: 600 }}>
                ${license?.wallet_balance !== undefined ? license.wallet_balance.toFixed(2) : '0.00'}
              </span>
            </div>
          </div>
        </div>

        {/* Content Body */}
        <div className="content-body">
          {activeTab === 'dashboard' && (
            <div>
              {/* Stat Cards */}
              <div className="dashboard-grid">
                <div className="panel card-stat">
                  <div>
                    <span style={{ color: 'var(--text-secondary)', fontSize: '0.85rem' }}>Trial Status</span>
                    <div className="stat-val color-green">
                      {license?.trial_expired ? 'Expired' : `${license?.trial_days_remaining} Days`}
                    </div>
                  </div>
                  <Info size={32} className="color-green" />
                </div>

                <div className="panel card-stat">
                  <div>
                    <span style={{ color: 'var(--text-secondary)', fontSize: '0.85rem' }}>Active Wallet Balance</span>
                    <div className="stat-val color-purple">
                      ${license?.wallet_balance.toFixed(2)}
                    </div>
                  </div>
                  <Wallet size={32} className="color-purple" />
                </div>

                <div className="panel card-stat">
                  <div>
                    <span style={{ color: 'var(--text-secondary)', fontSize: '0.85rem' }}>Connected Device</span>
                    <div className="stat-val color-blue">
                      {deviceState.toUpperCase()}
                    </div>
                  </div>
                  <Smartphone size={32} className="color-blue" />
                </div>
              </div>

              {/* Graphic phone layout */}
              <div className="panel" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '2rem' }}>
                <div style={{ flex: 1 }}>
                  <h3 style={{ fontWeight: 600, marginBottom: '0.5rem' }}>Physical USB Connection Map</h3>
                  <p style={{ color: 'var(--text-secondary)', fontSize: '0.9rem', marginBottom: '1rem' }}>
                    Select the target servicing mode from the top right selector. The interface auto-maps drivers, COM ports, and device interfaces instantly.
                  </p>
                  <div style={{ fontFamily: 'monospace', background: '#090a10', padding: '1rem', borderRadius: '8px', border: '1px solid var(--border-color)', fontSize: '0.85rem' }}>
                    <div><strong>Driver Status:</strong> OK</div>
                    <div><strong>Active Serial COM:</strong> COM5 (115200 Baud)</div>
                    <div><strong>Secure Handshake:</strong> {deviceState !== 'disconnected' ? 'STABLE' : 'WAITING FOR TARGET'}</div>
                  </div>
                </div>

                {/* Simulated Phone UI component */}
                <div style={{ width: '120px', height: '200px', border: '3px solid var(--border-color)', borderRadius: '16px', position: 'relative', display: 'flex', alignItems: 'center', justifyContent: 'center', background: '#090b10', boxShadow: deviceState !== 'disconnected' ? '0 0 20px rgba(59,130,246,0.3)' : 'none' }}>
                  <div style={{ width: '40px', height: '4px', background: 'var(--border-color)', borderRadius: '2px', position: 'absolute', top: '8px' }}></div>
                  <div style={{ textAlign: 'center', padding: '0.5rem' }}>
                    <Smartphone size={36} className={deviceState !== 'disconnected' ? 'color-blue' : 'color-red'} />
                    <div style={{ fontSize: '0.65rem', marginTop: '0.5rem', fontWeight: 600, textTransform: 'uppercase' }}>
                      {deviceState}
                    </div>
                  </div>
                  <div style={{ width: '8px', height: '8px', background: 'var(--border-color)', borderRadius: '50%', position: 'absolute', bottom: '8px' }}></div>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'xiaomi' && (
            <div className="ops-grid">
              <div className="panel ops-card">
                <h3 style={{ fontWeight: 600, display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <Cpu className="color-purple" /> Fastboot Operations
                </h3>
                <div className="ops-button-list">
                  <button 
                    className="op-btn" 
                    disabled={opLoading}
                    onClick={() => runOperation("Xiaomi FRP Bypass", 5.0, async () => {
                      addLog("Booting device into Fastboot Mode...", "info");
                      await new Promise(r => setTimeout(r, 1000));
                      addLog("Searching for partition config raw block...", "info");
                      await new Promise(r => setTimeout(r, 800));
                      addLog("Erasing persist partition secure sector...", "warning");
                      await new Promise(r => setTimeout(r, 600));
                      addLog("FRP bypass completed successfully.", "success");
                    })}
                  >
                    <span>FRP Bypass (Fastboot)</span>
                    <span className="color-amber">$5.00</span>
                  </button>

                  <button 
                    className="op-btn" 
                    disabled={opLoading}
                    onClick={() => runOperation("Xiaomi Bootloader Unlock", 10.0, async () => {
                      addLog("Querying Mi Account lock token...", "info");
                      await new Promise(r => setTimeout(r, 1200));
                      addLog("Uploading secure boot key token...", "info");
                      await new Promise(r => setTimeout(r, 1000));
                      addLog("Writing partition table signatures...", "warning");
                      await new Promise(r => setTimeout(r, 500));
                      addLog("Bootloader unlocked successfully.", "success");
                    })}
                  >
                    <span>Unlock Bootloader</span>
                    <span className="color-amber">$10.00</span>
                  </button>
                </div>
              </div>

              <div className="panel ops-card">
                <h3 style={{ fontWeight: 600, display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <Settings className="color-purple" /> Diagnostics & IMEI
                </h3>
                <div className="ops-button-list">
                  <button 
                    className="op-btn" 
                    disabled={opLoading}
                    onClick={() => runOperation("IMEI Repair (Dual)", 15.0, async () => {
                      addLog("Checking DIAG Port diagnostic mode...", "info");
                      await new Promise(r => setTimeout(r, 1000));
                      addLog("Erasing NVRAM backup partition blocks...", "warning");
                      await new Promise(r => setTimeout(r, 1200));
                      addLog("Writing secure parameters for IMEI 1 & IMEI 2...", "info");
                      await new Promise(r => setTimeout(r, 900));
                      addLog("IMEI calibration repair succeeded.", "success");
                    })}
                  >
                    <span>IMEI Dual Repair</span>
                    <span className="color-amber">$15.00</span>
                  </button>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'samsung' && (
            <div className="ops-grid">
              <div className="panel ops-card">
                <h3 style={{ fontWeight: 600, display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <Cpu className="color-purple" /> Odin & Flashing
                </h3>
                <div className="ops-button-list">
                  <button 
                    className="op-btn" 
                    disabled={opLoading}
                    onClick={() => runOperation("Samsung PIT Read", 0.0, async () => {
                      addLog("Sending handshake down Odin PIT channel...", "info");
                      await new Promise(r => setTimeout(r, 500));
                      addLog("PIT map retrieved successfully: 24 active partitions.", "success");
                    })}
                  >
                    <span>Read PIT Map</span>
                    <span className="color-green">FREE</span>
                  </button>

                  <button 
                    className="op-btn" 
                    disabled={opLoading}
                    onClick={() => runOperation("Samsung CSC Change", 4.0, async () => {
                      addLog("Opening Odin secure region port...", "info");
                      await new Promise(r => setTimeout(r, 800));
                      addLog("Writing new CSC system metadata profile...", "info");
                      await new Promise(r => setTimeout(r, 800));
                      addLog("Rebooting device...", "warning");
                    })}
                  >
                    <span>CSC Region Change</span>
                    <span className="color-amber">$4.00</span>
                  </button>
                </div>
              </div>

              <div className="panel ops-card">
                <h3 style={{ fontWeight: 600, display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <Lock className="color-purple" /> Security Bypass
                </h3>
                <div className="ops-button-list">
                  <button 
                    className="op-btn" 
                    disabled={opLoading}
                    onClick={() => runOperation("Samsung FRP Bypass", 8.0, async () => {
                      addLog("Checking Knox security version...", "info");
                      await new Promise(r => setTimeout(r, 1100));
                      addLog("Executing Knox security token exploit...", "warning");
                      await new Promise(r => setTimeout(r, 1300));
                      addLog("Sending factory reset command...", "info");
                      await new Promise(r => setTimeout(r, 500));
                    })}
                  >
                    <span>FRP Bypass (Knox Direct)</span>
                    <span className="color-amber">$8.00</span>
                  </button>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'mediatek' && (
            <div className="ops-grid">
              <div className="panel ops-card">
                <h3 style={{ fontWeight: 600, display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <Cpu className="color-purple" /> MTK Brom Bypasses
                </h3>
                <div className="ops-button-list">
                  <button 
                    className="op-btn" 
                    disabled={opLoading}
                    onClick={() => runOperation("MTK Auth Bypass", 3.0, async () => {
                      addLog("Waiting for MTK USB port device insertion...", "info");
                      await new Promise(r => setTimeout(r, 1000));
                      addLog("Brom detected. Triggering handshake payload...", "info");
                      await new Promise(r => setTimeout(r, 700));
                      addLog("SLA/DAA security authentication disabled successfully.", "success");
                    })}
                  >
                    <span>Disable SLA/DAA Auth</span>
                    <span className="color-amber">$3.00</span>
                  </button>

                  <button 
                    className="op-btn" 
                    disabled={opLoading}
                    onClick={() => runOperation("Safe Format", 6.0, async () => {
                      addLog("Connecting to DA agent...", "info");
                      await new Promise(r => setTimeout(r, 900));
                      addLog("Finding user data pattern lock registers...", "info");
                      await new Promise(r => setTimeout(r, 1000));
                      addLog("Formatting secure lock registry while keeping media...", "warning");
                      await new Promise(r => setTimeout(r, 1200));
                    })}
                  >
                    <span>Safe Format (Keep Data)</span>
                    <span className="color-amber">$6.00</span>
                  </button>
                </div>
              </div>

              <div className="panel ops-card">
                <h3 style={{ fontWeight: 600, display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <Layers className="color-purple" /> Flash Firmware
                </h3>
                <div className="ops-button-list">
                  <button 
                    className="op-btn" 
                    disabled={opLoading}
                    onClick={() => runOperation("MTK Firmware Flash", 5.0, async () => {
                      addLog("Reading scatter map file...", "info");
                      await new Promise(r => setTimeout(r, 600));
                      addLog("Sending boot partition (5MB)...", "info");
                      await new Promise(r => setTimeout(r, 400));
                      addLog("Sending system partition (2.4GB)...", "info");
                      await new Promise(r => setTimeout(r, 2000));
                      addLog("Sending userdata partition (1.2GB)...", "info");
                      await new Promise(r => setTimeout(r, 1000));
                    })}
                  >
                    <span>Flash Firmware Scatter</span>
                    <span className="color-amber">$5.00</span>
                  </button>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'wallet' && (
            <div className="ops-grid">
              {/* Account Information */}
              <div className="panel ops-card">
                <h3 style={{ fontWeight: 600, display: 'flex', alignItems: 'center', gap: '0.5rem' }}><Info className="color-purple" /> Subscription Details</h3>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem', marginTop: '0.5rem' }}>
                  <div>
                    <span style={{ color: 'var(--text-secondary)', fontSize: '0.85rem' }}>Licensed Email</span>
                    <div style={{ fontWeight: 600, fontSize: '1.1rem' }}>{license?.email}</div>
                  </div>

                  <div>
                    <span style={{ color: 'var(--text-secondary)', fontSize: '0.85rem' }}>Hardware Fingerprint (HWID)</span>
                    <div style={{ fontSize: '0.75rem', fontFamily: 'monospace', background: '#090a10', padding: '0.5rem', borderRadius: '4px', border: '1px solid var(--border-color)', wordBreak: 'break-all' }}>
                      {license?.hwid}
                    </div>
                  </div>

                  <div>
                    <span style={{ color: 'var(--text-secondary)', fontSize: '0.85rem' }}>Trial Expires At</span>
                    <div style={{ fontWeight: 600 }}>{license ? new Date(license.trial_expires_at).toLocaleString() : ''}</div>
                  </div>
                </div>
              </div>

              {/* Recharge Terminal */}
              <div className="panel ops-card">
                <h3 style={{ fontWeight: 600, display: 'flex', alignItems: 'center', gap: '0.5rem' }}><CreditCard className="color-purple" /> Recharge Terminal</h3>
                <p style={{ color: 'var(--text-secondary)', fontSize: '0.85rem' }}>
                  Top up credits securely. In development, credits can be injected immediately via this mock admin controller.
                </p>

                <div className="form-group" style={{ marginTop: '0.5rem' }}>
                  <label>Amount (USD)</label>
                  <div style={{ display: 'flex', gap: '0.5rem' }}>
                    <input 
                      type="number" 
                      className="form-control" 
                      value={topupAmount} 
                      onChange={e => setTopupAmount(e.target.value)} 
                      placeholder="50.00" 
                    />
                    <button className="btn-primary" style={{ width: 'auto', padding: '0.75rem 1.5rem' }} onClick={handleTopup} disabled={topupLoading}>
                      {topupLoading ? 'Adding...' : 'Add Balance'}
                    </button>
                  </div>
                </div>

                <div style={{ display: 'flex', gap: '1rem', marginTop: '1rem' }}>
                  <button className="btn-primary btn-secondary" style={{ flex: 1 }} onClick={() => verifyLicense(email)}>
                    <RefreshCw size={14} style={{ marginRight: '0.5rem' }} /> Verify License
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>

        {/* Real-time Logger Console Panel */}
        <div className="console-panel">
          <div className="console-header">
            <span style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}><TermIcon size={14} /> LIVE SERVICING PORT LOGS</span>
            <button 
              style={{ background: 'transparent', border: 'none', color: 'var(--text-secondary)', cursor: 'pointer', fontSize: '0.75rem' }}
              onClick={() => setLogs([])}
            >
              Clear Console
            </button>
          </div>
          <div className="console-body">
            {logs.length === 0 ? (
              <div style={{ color: 'var(--text-secondary)', fontStyle: 'italic' }}>Console idle. Awaiting device operation...</div>
            ) : (
              logs.map((log, i) => (
                <div key={i} className="log-entry">
                  <span className="log-timestamp">[{log.timestamp}]</span>
                  <span className={`log-${log.type}`}>
                    {log.type === 'error' && '[ERROR] '}
                    {log.type === 'success' && '[SUCCESS] '}
                    {log.type === 'warning' && '[WARN] '}
                    {log.message}
                  </span>
                </div>
              ))
            )}
            <div ref={consoleEndRef}></div>
          </div>
        </div>
      </div>
    </div>
  );
}
