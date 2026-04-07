import { useState, useEffect, useMemo } from 'react';
import { Users, Shield, Save } from 'lucide-react';
import { api } from '../../lib/api';
import { useAuth } from '../../contexts/AuthContext';

interface Role {
  id: number;
  name: string;
  description: string;
}

interface UserWithRoles {
  userID: number;
  username: string;
  email: string;
  first_name: string;
  last_name: string;
  roles: Role[];
}

export function RolesTab() {
  const { user } = useAuth();
  const [users, setUsers] = useState<UserWithRoles[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [selectedUser, setSelectedUser] = useState<UserWithRoles | null>(null);
  const [selectedRoles, setSelectedRoles] = useState<number[]>([]);
  const [saving, setSaving] = useState(false);

  const currentRoles = useMemo(
    () => (user?.roles ?? []) as Role[],
    [user]
  );
  const restrictedRoleNames = useMemo(() => ['admin', 'manager', 'super_admin'], []);
  const canManageRestricted = useMemo(
    () =>
      currentRoles.some(role => {
        const name = (role.name ?? '').toLowerCase();
        return name === 'admin' || name === 'super_admin';
      }),
    [currentRoles]
  );

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    try {
      const [usersRes, rolesRes] = await Promise.all([
        api.get('/admin/users'),
        api.get('/admin/roles'),
      ]);
      setUsers(usersRes.data);
      setRoles(rolesRes.data);
    } catch (error) {
      console.error('Failed to load data:', error);
    }
  };

  const selectUser = (user: UserWithRoles) => {
    setSelectedUser(user);
    setSelectedRoles(user.roles.map(r => r.id));
  };

  const toggleRole = (roleId: number) => {
    const role = roles.find(r => r.id === roleId);
    if (!role) return;

    const isRestricted = restrictedRoleNames.includes((role.name ?? '').toLowerCase());
    if (isRestricted && !canManageRestricted) {
      return;
    }

    setSelectedRoles(prev =>
      prev.includes(roleId)
        ? prev.filter(id => id !== roleId)
        : [...prev, roleId]
    );
  };

  const handleSave = async () => {
    if (!selectedUser) return;

    setSaving(true);
    try {
      await api.put(`/admin/users/${selectedUser.userID}/roles`, {
        role_ids: selectedRoles,
      });
      await loadData();
      setSelectedUser(null);
      alert('Rollen erfolgreich aktualisiert');
    } catch (error: any) {
      alert('Fehler: ' + (error.response?.data?.error || error.message));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Users className="w-6 h-6 text-blue-400" />
        <div>
          <h2 className="text-xl font-bold text-white">Rollen & Benutzer</h2>
          <p className="text-gray-400 text-sm">Benutzerrollen verwalten</p>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Users List */}
        <div className="space-y-2">
          <h3 className="text-white font-semibold mb-3">Benutzer</h3>
          {users.map(user => (
            <div
              key={user.userID}
              onClick={() => selectUser(user)}
              className={`glass rounded-xl p-4 cursor-pointer transition-all ${
                selectedUser?.userID === user.userID
                  ? 'ring-2 ring-accent-red'
                  : 'hover:bg-white/5'
              }`}
            >
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-full bg-accent-red/20 flex items-center justify-center flex-shrink-0">
                  <Users className="w-5 h-5 text-accent-red" />
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-white font-semibold truncate">
                    {user.first_name} {user.last_name}
                  </p>
                  <p className="text-gray-400 text-sm truncate">{user.email}</p>
                  <div className="flex flex-wrap gap-1 mt-1">
                    {user.roles.map(role => (
                      <span
                        key={role.id}
                        className="px-2 py-0.5 rounded-full bg-blue-500/20 text-blue-400 text-xs"
                      >
                        {role.name}
                      </span>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>

        {/* Roles Editor */}
        <div className="glass rounded-xl p-6">
          {selectedUser ? (
            <div className="space-y-4">
              <div>
                <h3 className="text-white font-semibold mb-2">
                  Rollen für {selectedUser.first_name} {selectedUser.last_name}
                </h3>
                <p className="text-gray-400 text-sm">{selectedUser.email}</p>
              </div>

              <div className="space-y-2">
                {roles.map(role => {
                  const isRestricted = restrictedRoleNames.includes((role.name ?? '').toLowerCase());
                  const disabled = isRestricted && !canManageRestricted;
                  return (
                    <label
                      key={role.id}
                      className={`flex items-center gap-3 p-3 rounded-lg glass transition-colors ${
                        disabled ? 'cursor-not-allowed opacity-70' : 'cursor-pointer hover:bg-white/5'
                      }`}
                      title={disabled ? 'Nur System-Admins dürfen diese Rolle ändern' : undefined}
                    >
                      <input
                        type="checkbox"
                        checked={selectedRoles.includes(role.id)}
                        onChange={() => toggleRole(role.id)}
                        className="w-5 h-5 rounded accent-accent-red"
                        disabled={disabled}
                      />
                      <div className="flex-1">
                        <div className="flex items-center gap-2">
                          <Shield className="w-4 h-4 text-blue-400" />
                          <span className="text-white font-semibold">{role.name}</span>
                        </div>
                        <p className="text-gray-400 text-sm">{role.description}</p>
                        {disabled && (
                          <p className="text-xs text-gray-500 mt-1">
                            Diese Rolle kann nur durch System-Admins angepasst werden.
                          </p>
                        )}
                      </div>
                    </label>
                  );
                })}
              </div>

              <button
                onClick={handleSave}
                disabled={saving}
                className={`w-full py-3 px-6 rounded-xl font-semibold text-white transition-all flex items-center justify-center gap-2 ${
                  saving
                    ? 'bg-gray-600 cursor-not-allowed'
                    : 'bg-gradient-to-r from-accent-red to-red-700 hover:shadow-lg'
                }`}
              >
                <Save className="w-5 h-5" />
                <span>{saving ? 'Speichert...' : 'Speichern'}</span>
              </button>
            </div>
          ) : (
            <div className="text-center text-gray-400 py-12">
              <Users className="w-12 h-12 mx-auto mb-3 opacity-50" />
              <p>Wähle einen Benutzer aus, um Rollen zu verwalten</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
