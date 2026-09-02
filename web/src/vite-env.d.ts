/// <reference types="vite/client" />

interface Window {
  __APP_CONFIG__?: {
    procurementCoreURL?: string;
    [key: string]: unknown;
  };
}
