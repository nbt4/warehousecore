export const warehouseMountPath = '/warehousecore';

export const appBasePath = window.location.pathname === warehouseMountPath
  || window.location.pathname.startsWith(`${warehouseMountPath}/`)
  ? warehouseMountPath
  : '';

export function appPath(path: string): string {
  const normalized = path.startsWith('/') ? path : `/${path}`;
  return `${appBasePath}${normalized}`;
}

export function appAssetPath(value: string): string {
  if (!appBasePath || !value || !value.startsWith('/') || value.startsWith(`${appBasePath}/`)) return value;
  return appPath(value);
}
