// Auth API Service
const API_BASE = '/api/v1';

export interface Role {
  id: number;
  name: string;
  display_name?: string;
  description?: string;
}

export interface User {
  id: number;
  username: string;
  email: string;
  first_name: string;
  last_name: string;
  is_active: boolean;
  is_admin?: boolean;
  roles?: Role[];
  force_password_change?: boolean;
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  success: boolean;
  message: string;
  user?: User;
  force_password_change?: boolean;
}

export interface ChangePasswordRequest {
  current_password: string;
  new_password: string;
}

export interface ChangePasswordResponse {
  success: boolean;
  message: string;
}

class AuthService {
  async login(username: string, password: string): Promise<{ user: User; forcePasswordChange: boolean }> {
    const response = await fetch(`${API_BASE}/auth/login`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      credentials: 'include', // Important for cookies
      body: JSON.stringify({ username, password }),
    });

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.message || 'Login failed');
    }

    const data: LoginResponse = await response.json();
    if (!data.success || !data.user) {
      throw new Error(data.message || 'Login failed');
    }

    return {
      user: data.user,
      forcePasswordChange: data.force_password_change || false,
    };
  }

  async changePassword(currentPassword: string, newPassword: string): Promise<void> {
    const response = await fetch(`${API_BASE}/auth/change-password`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      credentials: 'include',
      body: JSON.stringify({
        current_password: currentPassword,
        new_password: newPassword,
      }),
    });

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.message || 'Password change failed');
    }

    const data: ChangePasswordResponse = await response.json();
    if (!data.success) {
      throw new Error(data.message || 'Password change failed');
    }
  }

  async logout(): Promise<void> {
    const response = await fetch(`${API_BASE}/auth/logout`, {
      method: 'POST',
      credentials: 'include',
    });

    if (!response.ok) {
      throw new Error('Logout failed');
    }
  }

  async getCurrentUser(): Promise<User | null> {
    try {
      const response = await fetch(`${API_BASE}/auth/me`, {
        credentials: 'include',
      });

      if (response.status === 401) {
        // Not authenticated
        return null;
      }

      if (!response.ok) {
        throw new Error('Failed to get current user');
      }

      const user: User = await response.json();
      return user;
    } catch (error) {
      console.error('Failed to get current user:', error);
      return null;
    }
  }
}

export const authService = new AuthService();
