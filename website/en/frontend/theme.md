# Theme & Styling

The frontend supports **dark / light** themes. The user's choice is stored in `localStorage` (key `app_theme`, dark by default) and toggled from the header.

## How Theming Works

1. **Root data attribute**: `App.tsx` writes the current mode to `<html data-theme="dark|light">`; global CSS switches the variable sets via `[data-theme]`.
2. **antd theme**: `ConfigProvider theme={mode === 'dark' ? darkTheme : lightTheme}` themes the antd components; `locale={zhCN}` and a unified empty state via `renderEmpty`.
3. **Toggle animation**: `toggle(point)` uses the [View Transitions API](https://developer.mozilla.org/docs/Web/API/View_Transitions_API) for a circular "liquid" reveal from the click point, falling back to an instant switch under `prefers-reduced-motion: reduce` or in unsupported browsers.
4. **Hook**: `useThemeMode()` (`src/theme/ThemeContext.ts`) exposes the current `mode` and `toggle`.

## Styling Conventions (read before writing a page)

- **Never hardcode colors.** Page styles must use the **CSS variables** defined in `index.css` / `list-pages.css`, so both themes apply automatically.
- Give list pages a root `className="list-page"` to inherit the unified layout from `list-pages.css` (filter area + table area + bottom spacing).
- Use the brand/accent variables (e.g. `--brand`, `--text-secondary` — see the variable list in `index.css`) instead of hardcoding values like `#3b82f6`.
- Style antd via `ConfigProvider` tokens, not `!important` overrides.

## Checklist

- [ ] Page is readable in both dark and light themes (screenshot compare)
- [ ] No hardcoded colors
- [ ] `message` / `modal` come from `@/utils/feedback` (theme-aware), not antd static methods
