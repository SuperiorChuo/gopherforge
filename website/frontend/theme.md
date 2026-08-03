# 主题与样式

前端支持**深空暗色 / 白蓝亮色**双主题，用户选择存 `localStorage`（键 `app_theme`，默认暗色），顶栏一键切换。

## 主题怎么工作的

1. **根元素数据属性**：`App.tsx` 把当前模式写到 `<html data-theme="dark|light">`（`document.documentElement.dataset.theme`），全局 CSS 用 `[data-theme]` 选择器区分两套变量。
2. **antd 主题**：`ConfigProvider theme={mode === 'dark' ? darkTheme : lightTheme}` 给 antd 组件套对应 token；`locale={zhCN}`、`renderEmpty` 统一空态。
3. **切换动画**：`toggle(point)` 用 [View Transitions API](https://developer.mozilla.org/docs/Web/API/View_Transitions_API) 从点击处做圆形「液面」漫开；`prefers-reduced-motion: reduce` 或浏览器不支持时直接切换。
4. **Hook**：`useThemeMode()`（`src/theme/ThemeContext.ts`）读当前 `mode` 与 `toggle`。

## 样式约定（写页面必读）

- **不要写死颜色**。页面样式一律用全局样式文件里的 **CSS 变量**（`index.css` / `list-pages.css`），双主题自动适配。
- 列表页根节点加 `className="list-page"`，可继承 `list-pages.css` 里的统一布局（筛选区 + 表格区 + 底部留白）。
- 需要品牌色/强调色时取变量（如 `--brand` / `--text-secondary` 等，具体以 `index.css` 变量清单为准），不要硬编码 `#3b82f6` 这类值。
- antd 组件样式走 `ConfigProvider` token，不要靠 `!important` 覆盖。

## 检查清单

- [ ] 页面在暗色/亮色下都可读（截图对比）
- [ ] 无硬编码颜色
- [ ] 用 `message`/`modal` 走 `@/utils/feedback`（跟随主题），而非 antd 静态方法
