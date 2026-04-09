import type { ReactNode } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { Home, Package, MapPin, ScanLine, Wrench, Menu, Briefcase, X, LogOut, User, ChevronLeft, ChevronRight, Settings, Boxes, Tag, Cable, ChevronDown } from 'lucide-react';
import { useState, useEffect, useMemo } from 'react';
import { useAuth } from '../contexts/AuthContext';
import { LanguageSwitcher } from './LanguageSwitcher';
import { useTranslation } from 'react-i18next';

interface LayoutProps {
  children: ReactNode;
}

export function Layout({ children }: LayoutProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const { user, logout } = useAuth();
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [isMobile, setIsMobile] = useState(false);
  const { t } = useTranslation();
  const companyName = (window as any).__APP_CONFIG__?.companyName || 'RentalCore';

  useEffect(() => {
    const checkMobile = () => {
      const mobile = window.innerWidth < 768;
      setIsMobile(mobile);
      if (!mobile) {
        setSidebarOpen(true); // Desktop: sidebar expanded by default
      } else {
        setSidebarOpen(false); // Mobile: sidebar hidden by default
      }
    };

    checkMobile();
    window.addEventListener('resize', checkMobile);
    return () => window.removeEventListener('resize', checkMobile);
  }, []);

  const closeSidebar = () => {
    if (isMobile) {
      setSidebarOpen(false);
    }
  };

  const handleLogout = async () => {
    await logout();
    navigate('/login');
  };

  // Get cross-navigation domain from environment variable injected by backend
  // If not set, auto-detect based on current hostname
  const getRentalCoreURL = () => {
    const rentalDomain = (window as any).__APP_CONFIG__?.rentalCoreDomain;
    const protocol = window.location.protocol; // Use same protocol as current page (http/https)

    if (rentalDomain && rentalDomain !== '') {
      // Use configured domain from environment (no port, handled by reverse proxy)
      return `${protocol}//${rentalDomain}`;
    }

    // Auto-detect based on current hostname
    const hostname = window.location.hostname;
    const port = window.location.port;

    // Check if we're on a subdomain setup (e.g., warehouse.server-nt.de)
    if (hostname.startsWith('warehouse.')) {
      // Replace 'warehouse' with 'rent'
      const rentalHost = hostname.replace(/^warehouse\./, 'rent.');
      return `${protocol}//${rentalHost}`;
    } else if (port === '8082') {
      // Running on port 8082 -> go to 8081 on same host
      return `${protocol}//${hostname}:8081`;
    } else if (port === '') {
      // No port specified (reverse proxy) - assume different subdomain
      // This handles cases like warehouse.example.com -> rent.example.com
      if (hostname.includes('.')) {
        // Try to replace warehouse with rent in hostname
        const rentalHost = hostname.replace(/warehouse/i, 'rent');
        return `${protocol}//${rentalHost}`;
      }
      // Fallback if no subdomain detected
      return `${protocol}//${hostname}:8081`;
    } else {
      // Default fallback - try :8081 on same host
      return `${protocol}//${hostname}:8081`;
    }
  };

  const rentalCoreURL = getRentalCoreURL();

  // Debug log
  console.log('RentalCore URL:', rentalCoreURL);

  const userRoles = (user?.roles ?? []) as any[];
  const hasAdminAccess = useMemo(() => {
    return userRoles.some((role) => {
      const name = (role?.name || role?.Name || '').toString().toLowerCase();
      return name === 'admin' || name === 'manager' || name === 'warehouse_admin';
    });
  }, [userRoles]);

  const mainNavItems = useMemo(() => ([
    { key: 'dashboard', path: '/', icon: Home, label: t('nav.dashboard') },
    { key: 'scan', path: '/scan', icon: ScanLine, label: t('nav.scan') },
    { key: 'labels', path: '/labels', icon: Tag, label: t('nav.labels') },
    { key: 'cases', path: '/cases', icon: Boxes, label: t('nav.cases') },
    { key: 'zones', path: '/zones', icon: MapPin, label: t('nav.zones') },
    { key: 'jobs', path: '/jobs', icon: Briefcase, label: t('nav.jobs') },
    { key: 'maintenance', path: '/maintenance', icon: Wrench, label: t('nav.maintenance') },
  ]), [t]);

  const navIndex = useMemo(() => {
    const map = new Map<string, typeof mainNavItems[number]>();
    mainNavItems.forEach((item) => map.set(item.key, item));
    return map;
  }, [mainNavItems]);

  const productNavItems = useMemo(() => ([
    { path: '/products', icon: Package, label: t('nav.products') },
    { path: '/cables', icon: Cable, label: t('nav.cables') },
  ]), [t]);

  const adminNavItem = useMemo(() => (
    { path: '/admin', icon: Settings, label: t('nav.admin') }
  ), [t]);

  const productNavActive = useMemo(
    () => productNavItems.some(item => location.pathname.startsWith(item.path)),
    [productNavItems, location.pathname]
  );
  const [productMenuOpen, setProductMenuOpen] = useState(productNavActive);

  useEffect(() => {
    if (productNavActive) {
      setProductMenuOpen(true);
    }
  }, [productNavActive]);

  return (
    <div className="min-h-screen bg-dark">
      {/* Header */}
      <header
        className={`fixed top-0 right-0 z-50 glass-dark transition-all duration-300 ${
          !isMobile && sidebarOpen ? 'left-64' : !isMobile ? 'left-20' : 'left-0'
        }`}
        style={{ height: '60px', borderBottom: '1px solid rgba(255,255,255,0.08)' }}
      >
        <div className="flex items-center justify-between px-4 sm:px-6 h-full">
          <div className="flex items-center gap-3">
            <button
              onClick={() => setSidebarOpen(!sidebarOpen)}
              className="p-2 rounded-lg transition-colors cursor-pointer"
              style={{ background: 'none', border: 'none', color: '#A0A0A0' }}
              aria-label="Toggle sidebar"
            >
              {isMobile
                ? <Menu className="w-5 h-5" />
                : sidebarOpen
                  ? <ChevronLeft className="w-5 h-5" />
                  : <ChevronRight className="w-5 h-5" />
              }
            </button>
            <h1 className="font-bold" style={{ fontSize: '1.125rem' }}>
              <span style={{ color: '#D0021B' }}>Warehouse</span>
              <span style={{ color: '#ffffff' }}>Core</span>
            </h1>
          </div>
          <div className="flex items-center gap-3">
            <LanguageSwitcher />
            {companyName && (
              <span className="text-sm hidden sm:block" style={{ color: '#606060' }}>
                {companyName}
              </span>
            )}
          </div>
        </div>
      </header>

      {/* Mobile Backdrop */}
      {isMobile && sidebarOpen && (
        <div
          className="fixed inset-0 z-40"
          style={{ background: 'rgba(0,0,0,0.7)' }}
          onClick={closeSidebar}
        />
      )}

      {/* Sidebar */}
      <aside
        className={`fixed left-0 top-0 bottom-0 z-50 glass-dark flex flex-col transition-all duration-300 ease-in-out ${
          isMobile && !sidebarOpen ? '-translate-x-full' : 'translate-x-0'
        } ${isMobile ? 'w-64' : sidebarOpen ? 'w-64' : 'w-20'}`}
        style={{ borderRight: '1px solid rgba(255,255,255,0.08)' }}
      >
        {/* Mobile sidebar header */}
        <div
          className="flex items-center justify-between px-4 py-4 md:hidden"
          style={{ borderBottom: '1px solid rgba(255,255,255,0.08)' }}
        >
          <h2 className="font-bold" style={{ fontSize: '1.125rem' }}>
            <span style={{ color: '#D0021B' }}>Warehouse</span>
            <span style={{ color: '#ffffff' }}>Core</span>
          </h2>
          <button
            onClick={closeSidebar}
            className="p-2 rounded-lg cursor-pointer"
            style={{ background: 'none', border: 'none', color: '#A0A0A0' }}
            aria-label="Close menu"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <nav
          className={`flex-1 overflow-y-auto p-3 space-y-1 ${isMobile ? 'mt-12' : 'mt-[60px]'}`}
          style={{ scrollbarWidth: 'none' }}
        >
          {/* Cross-nav to RentalCore */}
          <a
            href={rentalCoreURL}
            className={`flex items-center rounded-lg transition-all ${
              sidebarOpen || isMobile ? 'gap-3 px-3 py-2.5' : 'justify-center p-3'
            }`}
            style={{
              background: 'rgba(208,2,27,0.08)',
              color: '#D0021B',
              border: '1px solid rgba(208,2,27,0.15)',
              textDecoration: 'none',
              fontSize: '0.875rem',
              fontWeight: 600,
            }}
            title="Switch to RentalCore"
          >
            <Briefcase className="w-4 h-4 flex-shrink-0" />
            {(sidebarOpen || isMobile) && <span>RentalCore</span>}
          </a>

          <div style={{ height: '4px' }} />

          {/* Main nav items */}
          {['dashboard', 'scan'].map((key) => {
            const item = navIndex.get(key);
            if (!item) return null;
            const Icon = item.icon;
            const isActive = location.pathname === item.path;

            return (
              <Link
                key={item.path}
                to={item.path}
                onClick={closeSidebar}
                className={`flex items-center rounded-lg transition-all ${
                  sidebarOpen || isMobile ? 'gap-3 px-3 py-2.5' : 'justify-center p-3'
                }`}
                style={{
                  background: isActive ? '#D0021B' : 'transparent',
                  color: isActive ? '#ffffff' : '#A0A0A0',
                  textDecoration: 'none',
                  fontSize: '0.875rem',
                  fontWeight: 500,
                  boxShadow: isActive ? '0 2px 8px rgba(208,2,27,0.3)' : 'none',
                }}
                title={!sidebarOpen && !isMobile ? item.label : ''}
              >
                <Icon className="w-4 h-4 flex-shrink-0" />
                {(sidebarOpen || isMobile) && <span>{item.label}</span>}
              </Link>
            );
          })}

          {/* Product management submenu */}
          <div>
            <button
              type="button"
              onClick={() => setProductMenuOpen(prev => !prev)}
              className={`flex items-center w-full rounded-lg transition-all cursor-pointer ${
                sidebarOpen || isMobile ? 'justify-between gap-3 px-3 py-2.5' : 'justify-center p-3'
              }`}
              style={{
                background: productNavActive ? '#D0021B' : 'transparent',
                color: productNavActive ? '#ffffff' : '#A0A0A0',
                border: 'none',
                fontSize: '0.875rem',
                fontWeight: 500,
                boxShadow: productNavActive ? '0 2px 8px rgba(208,2,27,0.3)' : 'none',
              }}
              aria-expanded={productMenuOpen}
              title={!sidebarOpen && !isMobile ? t('nav.productManagement') : ''}
            >
              <div className="flex items-center gap-3">
                <Package className="w-4 h-4 flex-shrink-0" />
                {(sidebarOpen || isMobile) && <span>{t('nav.productManagement')}</span>}
              </div>
              {(sidebarOpen || isMobile) && (
                <ChevronDown className={`w-3.5 h-3.5 transition-transform ${productMenuOpen ? 'rotate-180' : ''}`} />
              )}
            </button>

            {(sidebarOpen || isMobile) && (
              <div
                className={`flex flex-col space-y-0.5 overflow-hidden transition-all duration-200 ${
                  productMenuOpen ? 'max-h-40 opacity-100 mt-1' : 'max-h-0 opacity-0'
                }`}
              >
                {productNavItems.map((item) => {
                  const Icon = item.icon;
                  const isActive = location.pathname.startsWith(item.path);
                  return (
                    <Link
                      key={item.path}
                      to={item.path}
                      onClick={closeSidebar}
                      className="flex items-center gap-3 rounded-lg transition-all"
                      style={{
                        padding: '0.5rem 0.75rem',
                        marginLeft: '1.25rem',
                        background: isActive ? '#D0021B' : 'transparent',
                        color: isActive ? '#ffffff' : '#A0A0A0',
                        textDecoration: 'none',
                        fontSize: '0.875rem',
                        fontWeight: 500,
                      }}
                    >
                      <Icon className="w-4 h-4 flex-shrink-0" />
                      <span>{item.label}</span>
                    </Link>
                  );
                })}
              </div>
            )}
          </div>

          {/* Remaining nav items */}
          {['labels', 'cases', 'zones', 'jobs', 'maintenance'].map((key) => {
            const item = navIndex.get(key);
            if (!item) return null;
            const Icon = item.icon;
            const isActive = location.pathname === item.path;

            return (
              <Link
                key={item.path}
                to={item.path}
                onClick={closeSidebar}
                className={`flex items-center rounded-lg transition-all ${
                  sidebarOpen || isMobile ? 'gap-3 px-3 py-2.5' : 'justify-center p-3'
                }`}
                style={{
                  background: isActive ? '#D0021B' : 'transparent',
                  color: isActive ? '#ffffff' : '#A0A0A0',
                  textDecoration: 'none',
                  fontSize: '0.875rem',
                  fontWeight: 500,
                  boxShadow: isActive ? '0 2px 8px rgba(208,2,27,0.3)' : 'none',
                }}
                title={!sidebarOpen && !isMobile ? item.label : ''}
              >
                <Icon className="w-4 h-4 flex-shrink-0" />
                {(sidebarOpen || isMobile) && <span>{item.label}</span>}
              </Link>
            );
          })}

          {hasAdminAccess && (
            <Link
              to={adminNavItem.path}
              onClick={closeSidebar}
              className={`flex items-center rounded-lg transition-all ${
                sidebarOpen || isMobile ? 'gap-3 px-3 py-2.5' : 'justify-center p-3'
              }`}
              style={{
                background: location.pathname === adminNavItem.path ? '#D0021B' : 'transparent',
                color: location.pathname === adminNavItem.path ? '#ffffff' : '#A0A0A0',
                textDecoration: 'none',
                fontSize: '0.875rem',
                fontWeight: 500,
              }}
              title={!sidebarOpen && !isMobile ? adminNavItem.label : ''}
            >
              <Settings className="w-4 h-4 flex-shrink-0" />
              {(sidebarOpen || isMobile) && <span>{adminNavItem.label}</span>}
            </Link>
          )}
        </nav>

        {/* Sidebar footer — user + logout */}
        <div
          className={`p-3 flex flex-col gap-1 ${!sidebarOpen && !isMobile ? 'items-center' : ''}`}
          style={{ borderTop: '1px solid rgba(255,255,255,0.08)' }}
        >
          {user && (sidebarOpen || isMobile) && (
            <button
              onClick={() => { closeSidebar(); navigate('/profile'); }}
              className="flex items-center gap-2.5 px-3 py-2.5 rounded-lg w-full text-left cursor-pointer transition-all"
              style={{ background: 'rgba(255,255,255,0.04)', border: '1px solid rgba(255,255,255,0.06)', color: '#ffffff' }}
              title="Profil öffnen"
            >
              <User className="w-4 h-4 flex-shrink-0" style={{ color: '#D0021B' }} />
              <span className="text-sm font-medium truncate">{user.username}</span>
            </button>
          )}
          {user && !sidebarOpen && !isMobile && (
            <div
              className="p-2.5 rounded-lg flex justify-center cursor-pointer"
              style={{ background: 'rgba(255,255,255,0.04)' }}
              onClick={() => navigate('/profile')}
              title="Profil"
            >
              <User className="w-4 h-4" style={{ color: '#D0021B' }} />
            </div>
          )}
          <button
            onClick={handleLogout}
            className={`flex items-center rounded-lg transition-all cursor-pointer ${
              sidebarOpen || isMobile ? 'gap-3 px-3 py-2.5 w-full' : 'justify-center p-3'
            }`}
            style={{ background: 'none', border: 'none', color: '#A0A0A0', fontSize: '0.875rem', fontWeight: 500 }}
            title={!sidebarOpen && !isMobile ? 'Abmelden' : ''}
          >
            <LogOut className="w-4 h-4 flex-shrink-0" />
            {(sidebarOpen || isMobile) && <span>Abmelden</span>}
          </button>
        </div>
      </aside>

      {/* Main Content */}
      <main
        className={`transition-all duration-300 ${isMobile ? 'ml-0' : sidebarOpen ? 'ml-64' : 'ml-20'}`}
        style={{ paddingTop: '60px' }}
      >
        <div className="p-4 sm:p-6">
          {children}
        </div>
      </main>
    </div>
  );
}
