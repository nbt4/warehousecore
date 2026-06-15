// Global toast system via custom DOM events.
// Import { toast } from '../lib/toast' and use toast.error('message') or toast.success('message').

type ToastType = 'success' | 'error' | 'info';

interface ToastEvent {
  type: ToastType;
  message: string;
}

function emit(type: ToastType, message: string) {
  window.dispatchEvent(new CustomEvent('toast', { detail: { type, message } }));
}

export const toast = {
  error: (message: string) => emit('error', message),
  success: (message: string) => emit('success', message),
  info: (message: string) => emit('info', message),
};
