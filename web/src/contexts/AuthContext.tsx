import { createContext, useContext, useState, useEffect } from 'react';
import type { ReactNode } from 'react';
import { authService } from '../services/auth';
import type { User } from '../services/auth';
import { toast } from '../lib/toast';

interface AuthContextType {
  user: User | null;
  loading: boolean;
  forcePasswordChange: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  changePassword: (currentPassword: string, newPassword: string) => Promise<void>;
  clearForcePasswordChange: () => void;
  isAuthenticated: boolean;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [forcePasswordChange, setForcePasswordChange] = useState(false);

  // Check if user is already logged in on mount
  useEffect(() => {
    checkAuth();
  }, []);

  const checkAuth = async () => {
    try {
      const currentUser = await authService.getCurrentUser();
      setUser(currentUser);
      // Check if user needs to change password
      if (currentUser?.force_password_change) {
        setForcePasswordChange(true);
      }
    } catch (error) {
      toast.error('Auth check failed:' + " " + String(error));
      setUser(null);
    } finally {
      setLoading(false);
    }
  };

  const login = async (username: string, password: string) => {
    const result = await authService.login(username, password);
    setUser(result.user);
    setForcePasswordChange(result.forcePasswordChange);
  };

  const logout = async () => {
    await authService.logout();
    setUser(null);
    setForcePasswordChange(false);
  };

  const changePassword = async (currentPassword: string, newPassword: string) => {
    await authService.changePassword(currentPassword, newPassword);
    setForcePasswordChange(false);
  };

  const clearForcePasswordChange = () => {
    setForcePasswordChange(false);
  };

  const value: AuthContextType = {
    user,
    loading,
    forcePasswordChange,
    login,
    logout,
    changePassword,
    clearForcePasswordChange,
    isAuthenticated: user !== null,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
