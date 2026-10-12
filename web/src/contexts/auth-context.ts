import { createContext } from 'react';
import type { User } from '../services/auth';

export interface AuthContextType {
  user: User | null;
  loading: boolean;
  forcePasswordChange: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  changePassword: (currentPassword: string, newPassword: string) => Promise<void>;
  clearForcePasswordChange: () => void;
  isAuthenticated: boolean;
}

export const AuthContext = createContext<AuthContextType | undefined>(undefined);
