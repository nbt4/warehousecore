import { useEffect, useState } from 'react';

interface ToastItem {
  id: number;
  type: 'success' | 'error' | 'info';
  message: string;
}

let toastId = 0;

export default function ToastContainer() {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  useEffect(() => {
    const handler = (e: Event) => {
      const { type, message } = (e as CustomEvent).detail;
      const id = ++toastId;
      setToasts(prev => [...prev, { id, type, message }]);
      setTimeout(() => {
        setToasts(prev => prev.filter(t => t.id !== id));
      }, 4000);
    };
    window.addEventListener('toast', handler);
    return () => window.removeEventListener('toast', handler);
  }, []);

  if (toasts.length === 0) return null;

  return (
    <div style={{ position: 'fixed', bottom: 20, right: 20, zIndex: 10000, display: 'flex', flexDirection: 'column', gap: 8 }}>
      {toasts.map(t => (
        <div
          key={t.id}
          style={{
            padding: '10px 18px',
            borderRadius: 6,
            color: '#fff',
            background: t.type === 'error' ? '#dc3545' : t.type === 'success' ? '#28a745' : '#17a2b8',
            boxShadow: '0 2px 8px rgba(0,0,0,0.2)',
            fontSize: 14,
            maxWidth: 360,
            wordBreak: 'break-word',
          }}
        >
          {t.message}
        </div>
      ))}
    </div>
  );
}
