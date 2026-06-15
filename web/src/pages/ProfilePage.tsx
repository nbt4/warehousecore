import { useState, useEffect } from 'react';
import { User, Mail, Shield, Save, Lock, Eye, EyeOff, Check } from 'lucide-react';
import { api } from '../lib/api';
import { useAuth } from '../contexts/AuthContext';
import { toast } from '../lib/toast';

interface UserProfile {
  profile: {
    id: number;
    user_id: number;
    display_name: string;
    avatar_url: string;
    prefs: unknown;
    user?: {
      id: number;
      username: string;
      email: string;
      first_name: string;
      last_name: string;
    };
  };
  roles: Array<{ id: number; name: string; description: string }>;
}

export function ProfilePage() {
  const { changePassword } = useAuth();
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [displayName, setDisplayName] = useState('');
  const [avatarURL, setAvatarURL] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [profileMsg, setProfileMsg] = useState('');

  // Password change state
  const [currentPw, setCurrentPw] = useState('');
  const [newPw, setNewPw] = useState('');
  const [confirmPw, setConfirmPw] = useState('');
  const [showPw, setShowPw] = useState(false);
  const [pwSaving, setPwSaving] = useState(false);
  const [pwMsg, setPwMsg] = useState('');

  useEffect(() => { loadProfile(); }, []);

  const loadProfile = async () => {
    try {
      const r = await api.get('/profile/me');
      setProfile(r.data);
      setDisplayName(r.data.profile.display_name || '');
      setAvatarURL(r.data.profile.avatar_url || '');
    } catch (e) {
      toast.error(String(e));
    } finally {
      setLoading(false);
    }
  };

  const handleSaveProfile = async () => {
    setSaving(true);
    setProfileMsg('');
    try {
      await api.put('/profile/me', {
        display_name: displayName,
        avatar_url: avatarURL,
        prefs: profile?.profile.prefs || {},
      });
      setProfileMsg('✓ Profil gespeichert');
      setTimeout(() => setProfileMsg(''), 3000);
      loadProfile();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Fehler';
      setProfileMsg('Fehler: ' + msg);
    } finally {
      setSaving(false);
    }
  };

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    if (newPw !== confirmPw) { setPwMsg('Passwörter stimmen nicht überein'); return; }
    if (newPw.length < 8) { setPwMsg('Passwort muss mindestens 8 Zeichen haben'); return; }
    setPwMsg('');
    setPwSaving(true);
    try {
      await changePassword(currentPw, newPw);
      setPwMsg('✓ Passwort geändert');
      setCurrentPw(''); setNewPw(''); setConfirmPw('');
      setTimeout(() => setPwMsg(''), 3000);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Fehler beim Ändern';
      setPwMsg(msg);
    } finally {
      setPwSaving(false);
    }
  };

  if (loading) return <div className="text-white">Lädt...</div>;

  const u = profile?.profile.user;

  return (
    <div className="space-y-6 max-w-3xl">
      <div className="flex items-center gap-3">
        <User className="w-8 h-8 text-accent-red" />
        <div>
          <h1 className="text-3xl font-bold text-white">Mein Profil</h1>
          <p className="text-gray-400">Persönliche Einstellungen</p>
        </div>
      </div>

      {/* User info & profile settings */}
      <div className="glass-dark rounded-2xl p-6 space-y-6">
        <div className="flex items-center gap-4 pb-6 border-b border-white/10">
          <div className="w-16 h-16 rounded-full bg-accent-red/20 flex items-center justify-center">
            {avatarURL ? (
              <img src={avatarURL} alt="Avatar" className="w-16 h-16 rounded-full object-cover" />
            ) : (
              <User className="w-8 h-8 text-accent-red" />
            )}
          </div>
          <div>
            <h2 className="text-xl font-bold text-white">
              {u?.first_name || u?.last_name ? `${u?.first_name} ${u?.last_name}`.trim() : u?.username}
            </h2>
            <p className="text-gray-400 flex items-center gap-2 text-sm">
              <Mail className="w-4 h-4" />{u?.email}
            </p>
            <p className="text-gray-500 text-xs mt-0.5">@{u?.username}</p>
          </div>
        </div>

        <div>
          <label className="block text-sm font-semibold text-gray-400 mb-2 flex items-center gap-2">
            <Shield className="w-4 h-4" /> Rollen
          </label>
          <div className="flex flex-wrap gap-2">
            {profile?.roles.map((role) => (
              <span key={role.id} className="px-3 py-1 rounded-full bg-accent-red/20 text-accent-red text-sm font-semibold">
                {role.name}
              </span>
            ))}
          </div>
        </div>

        <div>
          <label className="block text-sm font-semibold text-gray-400 mb-2">Anzeigename (optional)</label>
          <input type="text" value={displayName} onChange={(e) => setDisplayName(e.target.value)}
            placeholder="z.B. Max M."
            className="w-full px-4 py-3 rounded-xl glass text-white placeholder-gray-500 focus:ring-2 focus:ring-accent-red outline-none" />
        </div>

        <div>
          <label className="block text-sm font-semibold text-gray-400 mb-2">Avatar URL (optional)</label>
          <input type="url" value={avatarURL} onChange={(e) => setAvatarURL(e.target.value)}
            placeholder="https://example.com/avatar.jpg"
            className="w-full px-4 py-3 rounded-xl glass text-white placeholder-gray-500 focus:ring-2 focus:ring-accent-red outline-none" />
        </div>

        <div className="pt-4 border-t border-white/10">
          <button onClick={handleSaveProfile} disabled={saving}
            className="py-3 px-6 rounded-xl font-semibold text-white transition-all flex items-center gap-2 bg-gradient-to-r from-accent-red to-red-700 hover:shadow-lg hover:shadow-red-500/30 disabled:opacity-50">
            <Save className="w-5 h-5" />{saving ? 'Speichert...' : 'Profil speichern'}
          </button>
          {profileMsg && (
            <div className={`mt-3 p-3 rounded-lg text-sm font-semibold ${profileMsg.startsWith('✓') ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400'}`}>
              {profileMsg}
            </div>
          )}
        </div>
      </div>

      {/* Password change */}
      <div className="glass-dark rounded-2xl p-6">
        <h3 className="text-lg font-semibold text-white mb-5 flex items-center gap-2">
          <Lock className="w-5 h-5 text-accent-red" /> Passwort ändern
        </h3>
        <form onSubmit={handleChangePassword} className="space-y-4">
          {[
            { label: 'Aktuelles Passwort', value: currentPw, set: setCurrentPw, autoComplete: 'current-password' },
            { label: 'Neues Passwort', value: newPw, set: setNewPw, autoComplete: 'new-password' },
            { label: 'Neues Passwort bestätigen', value: confirmPw, set: setConfirmPw, autoComplete: 'new-password' },
          ].map(({ label, value, set, autoComplete }) => (
            <div key={label}>
              <label className="block text-sm font-medium text-gray-300 mb-1.5">{label}</label>
              <div className="relative">
                <input
                  type={showPw ? 'text' : 'password'}
                  value={value}
                  onChange={(e) => set(e.target.value)}
                  required
                  autoComplete={autoComplete}
                  className="w-full px-4 py-3 pr-10 rounded-xl glass text-white placeholder-gray-500 focus:ring-2 focus:ring-accent-red outline-none"
                />
                <button type="button" onClick={() => setShowPw(!showPw)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-white">
                  {showPw ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
            </div>
          ))}
          <button type="submit" disabled={pwSaving}
            className="flex items-center gap-2 py-2.5 px-5 rounded-xl font-semibold text-white bg-accent-red hover:bg-accent-red/80 disabled:opacity-50 transition-colors">
            <Check className="w-4 h-4" />{pwSaving ? 'Wird geändert...' : 'Passwort ändern'}
          </button>
          {pwMsg && (
            <div className={`p-3 rounded-lg text-sm font-semibold ${pwMsg.startsWith('✓') ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400'}`}>
              {pwMsg}
            </div>
          )}
        </form>
      </div>
    </div>
  );
}
