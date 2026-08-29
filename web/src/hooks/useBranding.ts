import { useEffect, useState } from 'react';
import { appAssetPath, appPath } from '../lib/app-paths';

export interface BrandingAssets {
  markOnDark: string; markOnLight: string;
  horizontalOnDark: string; horizontalOnLight: string;
  stackedOnDark: string; stackedOnLight: string;
  favicon: string; appIcon: string; maskableIcon: string; print: string;
}

export interface BrandingConfig {
  productName: string; companyName: string; brandName: string;
  assets: BrandingAssets; companyAssets: Partial<BrandingAssets>;
  sidebarLogo: string; loginLogo: string; faviconPath: string;
}

const defaults: BrandingConfig = {
  productName: 'WarehouseCore', companyName: 'Cores', brandName: '',
  assets: {
    markOnDark: '/logos/warehousecore_white_icon.svg', markOnLight: '/logos/warehousecore_black_icon.svg',
    horizontalOnDark: '/logos/warehousecore_white_side.svg', horizontalOnLight: '/logos/warehousecore_black_side.svg',
    stackedOnDark: '/logos/warehousecore_white_full.svg', stackedOnLight: '/logos/warehousecore_black_full.svg',
    favicon: '/logos/warehousecore_black_icon.svg', appIcon: '/app-icons/icon-512.png',
    maskableIcon: '/app-icons/icon-maskable-512.png', print: '/logos/warehousecore_black_side.svg',
  },
  companyAssets: {}, sidebarLogo: '/logos/warehousecore_white_side.svg',
  loginLogo: '/logos/warehousecore_white_full.svg', faviconPath: '/logos/warehousecore_black_icon.svg',
};

let cached = defaults;
let started = false;
const listeners = new Set<(value: BrandingConfig) => void>();

function applyDocumentBranding(value: BrandingConfig) {
  const setLink = (selector: string, rel: string, href?: string) => {
    if (!href) return;
    let link = document.querySelector<HTMLLinkElement>(selector);
    if (!link) { link = document.createElement('link'); link.rel = rel; document.head.appendChild(link); }
    link.href = href;
    if (rel === 'icon') link.type = href.toLowerCase().includes('.png') ? 'image/png' : 'image/svg+xml';
  };
  setLink("link[rel~='icon']", 'icon', appAssetPath(value.assets.favicon));
  setLink("link[rel='apple-touch-icon']", 'apple-touch-icon', appAssetPath(value.assets.appIcon));
}

async function refresh() {
  try {
    const response = await fetch(appPath('/api/v1/branding'), { cache: 'no-store' });
    if (!response.ok) return;
    const raw = await response.json();
    const assets = Object.fromEntries(Object.entries({ ...defaults.assets, ...(raw.assets || {}) })
      .map(([key, value]) => [key, appAssetPath(String(value))])) as unknown as BrandingAssets;
    cached = {
      productName: raw.productName || defaults.productName, companyName: raw.companyName || defaults.companyName,
      brandName: raw.brandName || '', assets, companyAssets: raw.companyAssets || {},
      sidebarLogo: assets.horizontalOnDark, loginLogo: assets.stackedOnDark || assets.horizontalOnDark,
      faviconPath: assets.favicon,
    };
    applyDocumentBranding(cached);
    listeners.forEach(listener => listener(cached));
  } catch { /* retain defaults */ }
}

function start() {
  if (started) return;
  started = true;
  void refresh();
  window.setInterval(() => void refresh(), 60_000);
}

export function useBranding() {
  const [value, setValue] = useState(cached);
  useEffect(() => { listeners.add(setValue); start(); return () => { listeners.delete(setValue); }; }, []);
  return value;
}
