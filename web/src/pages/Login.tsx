import { useState } from 'react';
import type { FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { LogIn, User, Lock } from 'lucide-react';

export function Login() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const { login } = useAuth();
  const navigate = useNavigate();
  const companyName = (window as any).__APP_CONFIG__?.companyName || '';

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      await login(username, password);
      navigate('/');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login fehlgeschlagen');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div
      className="min-h-screen flex items-center justify-center p-4"
      style={{
        background:
          'radial-gradient(ellipse at 20% 80%, rgba(208,2,27,0.04) 0%, transparent 50%), radial-gradient(ellipse at 80% 20%, rgba(208,2,27,0.06) 0%, transparent 50%), #0B0B0B',
      }}
    >
      <div className="w-full" style={{ maxWidth: '400px' }}>

        {/* Brand */}
        <div className="text-center mb-8">
          <h1 className="font-bold tracking-tight mb-2" style={{ fontSize: '2.25rem' }}>
            <span style={{ color: '#D0021B' }}>Warehouse</span>
            <span style={{ color: '#ffffff' }}>Core</span>
          </h1>
          {companyName && (
            <p className="text-sm" style={{ color: '#A0A0A0' }}>{companyName}</p>
          )}
        </div>

        {/* Card */}
        <div
          className="card"
          style={{ padding: '2rem', position: 'relative', overflow: 'hidden' }}
        >
          {/* Top accent line */}
          <div
            style={{
              position: 'absolute',
              top: 0,
              left: 0,
              right: 0,
              height: '3px',
              background: '#D0021B',
              borderRadius: '0.75rem 0.75rem 0 0',
            }}
          />

          <h2 className="font-semibold text-white mb-6" style={{ fontSize: '1.125rem' }}>
            Anmelden
          </h2>

          {error && (
            <div
              className="mb-5 px-4 py-3 rounded-lg text-sm"
              style={{
                background: 'rgba(208,2,27,0.08)',
                border: '1px solid rgba(208,2,27,0.2)',
                color: '#f87171',
              }}
            >
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-4">
            {/* Username */}
            <div>
              <label
                htmlFor="username"
                className="block text-sm font-medium mb-2"
                style={{ color: '#A0A0A0' }}
              >
                Benutzername
              </label>
              <div className="relative">
                <User
                  className="absolute top-1/2 -translate-y-1/2 w-4 h-4"
                  style={{ left: '0.75rem', color: '#A0A0A0' }}
                />
                <input
                  id="username"
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  required
                  disabled={loading}
                  className="w-full"
                  style={{ paddingTop: '0.75rem', paddingBottom: '0.75rem', paddingLeft: '2.5rem', paddingRight: '1rem', borderRadius: '0.5rem' }}
                  placeholder="admin"
                  autoComplete="username"
                  autoFocus
                />
              </div>
            </div>

            {/* Password */}
            <div>
              <label
                htmlFor="password"
                className="block text-sm font-medium mb-2"
                style={{ color: '#A0A0A0' }}
              >
                Passwort
              </label>
              <div className="relative">
                <Lock
                  className="absolute top-1/2 -translate-y-1/2 w-4 h-4"
                  style={{ left: '0.75rem', color: '#A0A0A0' }}
                />
                <input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  disabled={loading}
                  className="w-full"
                  style={{ paddingTop: '0.75rem', paddingBottom: '0.75rem', paddingLeft: '2.5rem', paddingRight: '1rem', borderRadius: '0.5rem' }}
                  placeholder="••••••••"
                  autoComplete="current-password"
                />
              </div>
            </div>

            {/* Submit */}
            <button
              type="submit"
              disabled={loading}
              className="w-full flex items-center justify-center gap-2 font-semibold rounded-lg transition-all cursor-pointer"
              style={{
                paddingTop: '0.75rem',
                paddingBottom: '0.75rem',
                marginTop: '0.5rem',
                background: loading ? 'rgba(208,2,27,0.5)' : '#D0021B',
                color: 'white',
                border: 'none',
                fontSize: '1rem',
                boxShadow: loading ? 'none' : '0 4px 14px rgba(208,2,27,0.25)',
              }}
            >
              {loading ? (
                <>
                  <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                  Anmelden...
                </>
              ) : (
                <>
                  <LogIn className="w-4 h-4" />
                  Anmelden
                </>
              )}
            </button>
          </form>

          <div
            className="mt-6 pt-5 text-center text-xs"
            style={{
              borderTop: '1px solid rgba(255,255,255,0.08)',
              color: '#606060',
            }}
          >
            Sicher verschlüsselt · WarehouseCore
          </div>
        </div>

      </div>
    </div>
  );
}
