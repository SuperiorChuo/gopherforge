const STATE_VERSION_KEY = 'black8-console-state-version';
const STATE_VERSION = '2026-05-13-route-recovery-v2';
const TABS_ROUTER_KEY = 'tabsRouter';

function safeReadStorage(key: string) {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function safeWriteStorage(key: string, value: string) {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Storage may be unavailable in restricted browser contexts.
  }
}

function safeRemoveStorage(key: string) {
  try {
    window.localStorage.removeItem(key);
  } catch {
    // Storage may be unavailable in restricted browser contexts.
  }
}

function normalizePathname(pathname: string) {
  let normalized = pathname;

  for (let index = 0; index < 2 && /%2f/i.test(normalized); index += 1) {
    try {
      const decoded = decodeURIComponent(normalized);
      if (decoded === normalized) break;
      normalized = decoded;
    } catch {
      break;
    }
  }

  if (normalized.startsWith('//')) {
    normalized = `/${normalized.replace(/^\/+/, '')}`;
  }

  return normalized.startsWith('/') ? normalized : pathname;
}

function normalizeCurrentUrl() {
  const normalizedPathname = normalizePathname(window.location.pathname);

  if (normalizedPathname !== window.location.pathname) {
    window.history.replaceState(
      window.history.state,
      document.title,
      `${normalizedPathname}${window.location.search}${window.location.hash}`,
    );
  }
}

function hasStaleTabState(rawValue: string | null) {
  if (!rawValue) return false;

  try {
    const parsed = JSON.parse(rawValue);
    const routes = parsed?.tabRouterList;

    if (!Array.isArray(routes)) return true;

    return routes.some((route) => {
      const path = route?.path;
      const name = route?.name;
      const title = route?.title ?? route?.meta?.title;
      const isHiddenFallback = route?.meta?.hidden === true || (typeof name === 'string' && name.endsWith('Fallback'));
      const hasTitle =
        typeof title === 'string' ? title.trim().length > 0 : title && typeof title === 'object' && Object.keys(title).length > 0;

      return (
        typeof path !== 'string' ||
        /%2f/i.test(path) ||
        path.startsWith('//') ||
        (!route?.isHome && isHiddenFallback) ||
        (!route?.isHome && !hasTitle)
      );
    });
  } catch {
    return true;
  }
}

export function bootstrapConsoleState() {
  normalizeCurrentUrl();

  const storedVersion = safeReadStorage(STATE_VERSION_KEY);
  const tabsRouterState = safeReadStorage(TABS_ROUTER_KEY);

  if (storedVersion !== STATE_VERSION || hasStaleTabState(tabsRouterState)) {
    safeRemoveStorage(TABS_ROUTER_KEY);
    safeWriteStorage(STATE_VERSION_KEY, STATE_VERSION);
  }
}

bootstrapConsoleState();
