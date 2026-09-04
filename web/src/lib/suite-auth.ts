import { appBasePath } from './app-paths';

function configuredDashboardURL(): string | undefined {
  return (window as Window & { __APP_CONFIG__?: { dashboardURL?: string } }).__APP_CONFIG__?.dashboardURL;
}

export function coresDashboardURL(): string {
  if (appBasePath) return `${window.location.origin}/`;
  if (configuredDashboardURL()) return configuredDashboardURL()!;
  const { hostname, port, protocol } = window.location;
  if (port === '8082') return `${protocol}//${hostname}:8080`;
  if (hostname.startsWith('warehouse.')) return `${protocol}//${hostname.replace(/^warehouse\./, 'cores.')}`;
  return `${protocol}//${hostname}${port ? `:${port}` : ''}`;
}

export function centralLoginURL(): string {
  const dashboard = new URL(coresDashboardURL(), window.location.origin);
  const login = new URL('/login', dashboard);
  const localLoginPath = `${appBasePath}/login`;
  const current = window.location.pathname === localLoginPath
    ? `${appBasePath || ''}/`
    : `${window.location.pathname}${window.location.search}${window.location.hash}`;
  login.searchParams.set('redirect', dashboard.origin === window.location.origin ? current : new URL(current, window.location.origin).toString());
  return login.toString();
}
