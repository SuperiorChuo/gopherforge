const DEFAULT_REDIRECT = '/dashboard/index';

export function normalizeRedirectUrl(value?: string | null) {
  if (!value) return DEFAULT_REDIRECT;

  let redirect = value;

  for (let index = 0; index < 2; index += 1) {
    try {
      const decoded = decodeURIComponent(redirect);
      if (decoded === redirect) break;
      redirect = decoded;
    } catch {
      break;
    }
  }

  if (!redirect.startsWith('/') || redirect.startsWith('//')) {
    return DEFAULT_REDIRECT;
  }

  return redirect;
}
