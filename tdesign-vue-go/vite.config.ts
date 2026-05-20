import path from 'node:path';

import vue from '@vitejs/plugin-vue';
import vueJsx from '@vitejs/plugin-vue-jsx';
import type { ConfigEnv, UserConfig } from 'vite';
import { loadEnv } from 'vite';
import svgLoader from 'vite-svg-loader';

const CWD = process.cwd();

const normalizeModuleId = (id: string) => id.replace(/\\/g, '/');

const packageChunks: Array<[string, string[]]> = [
  ['vendor-vue', ['vue', 'vue-router', 'pinia', 'pinia-plugin-persistedstate', 'vue-i18n', '@vueuse/core']],
  ['vendor-tdesign-icons', ['tdesign-icons-vue-next']],
  ['vendor-utils', ['axios', 'dayjs', 'lodash', 'nprogress', 'qs', 'qrcode.vue', 'tvision-color']],
];

const isPackageModule = (id: string, packageName: string) => {
  const packageRoot = `/node_modules/${packageName}`;

  return id.includes(`${packageRoot}/`) || id.includes(`${packageRoot}.`);
};

const tdesignChunkMap: Record<string, string> = {
  alert: 'vendor-tdesign-feedback',
  badge: 'vendor-tdesign-data',
  breadcrumb: 'vendor-tdesign-navigation',
  button: 'vendor-tdesign-basic',
  card: 'vendor-tdesign-data',
  checkbox: 'vendor-tdesign-form',
  col: 'vendor-tdesign-basic',
  'color-picker': 'vendor-tdesign-form',
  'config-provider': 'vendor-tdesign-basic',
  'date-picker': 'vendor-tdesign-form',
  dialog: 'vendor-tdesign-feedback',
  divider: 'vendor-tdesign-basic',
  drawer: 'vendor-tdesign-feedback',
  dropdown: 'vendor-tdesign-navigation',
  empty: 'vendor-tdesign-feedback',
  form: 'vendor-tdesign-form',
  image: 'vendor-tdesign-data',
  input: 'vendor-tdesign-form',
  'input-adornment': 'vendor-tdesign-form',
  'input-number': 'vendor-tdesign-form',
  layout: 'vendor-tdesign-basic',
  link: 'vendor-tdesign-basic',
  list: 'vendor-tdesign-data',
  loading: 'vendor-tdesign-feedback',
  menu: 'vendor-tdesign-navigation',
  message: 'vendor-tdesign-feedback',
  notification: 'vendor-tdesign-feedback',
  pagination: 'vendor-tdesign-data',
  popconfirm: 'vendor-tdesign-feedback',
  popup: 'vendor-tdesign-feedback',
  progress: 'vendor-tdesign-data',
  radio: 'vendor-tdesign-form',
  row: 'vendor-tdesign-basic',
  select: 'vendor-tdesign-form',
  skeleton: 'vendor-tdesign-feedback',
  slider: 'vendor-tdesign-form',
  space: 'vendor-tdesign-basic',
  statistic: 'vendor-tdesign-data',
  steps: 'vendor-tdesign-navigation',
  switch: 'vendor-tdesign-form',
  table: 'vendor-tdesign-data',
  tabs: 'vendor-tdesign-navigation',
  tag: 'vendor-tdesign-data',
  textarea: 'vendor-tdesign-form',
  'time-picker': 'vendor-tdesign-form',
  tooltip: 'vendor-tdesign-feedback',
  transfer: 'vendor-tdesign-form',
  tree: 'vendor-tdesign-data',
  'tree-select': 'vendor-tdesign-form',
  upload: 'vendor-tdesign-form',
};

function resolvePackageChunk(id: string) {
  const normalizedId = normalizeModuleId(id);

  if (!normalizedId.includes('/node_modules/')) {
    return undefined;
  }

  if (normalizedId.includes('/node_modules/tdesign-vue-next/')) {
    const [, packagePath = ''] = normalizedId.split('/node_modules/tdesign-vue-next/');
    const [, firstSegment = ''] = packagePath.split('/');

    if (!firstSegment || firstSegment.startsWith('_') || firstSegment === 'common' || firstSegment === 'hooks') {
      return 'vendor-tdesign-shared';
    }

    return tdesignChunkMap[firstSegment] ?? 'vendor-tdesign-misc';
  }

  for (const [chunkName, packages] of packageChunks) {
    if (packages.some((packageName) => isPackageModule(normalizedId, packageName))) {
      return chunkName;
    }
  }

  return 'vendor';
}

// https://vitejs.dev/config/
export default ({ mode }: ConfigEnv): UserConfig => {
  const {
    VITE_BASE_URL = '/',
    VITE_API_URL = 'http://localhost:8081',
    VITE_API_URL_PREFIX = '/api/v1',
  } = loadEnv(mode, CWD);
  return {
    base: VITE_BASE_URL,
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },

    css: {
      preprocessorOptions: {
        less: {
          modifyVars: {
            hack: `true; @import (reference) "${path.resolve('src/style/variables.less')}";`,
          },
          math: 'strict',
          javascriptEnabled: true,
        },
      },
    },

    plugins: [
      vue(),
      vueJsx(),
      svgLoader(),
    ],

    build: {
      rollupOptions: {
        output: {
          manualChunks: resolvePackageChunk,
        },
      },
    },

    server: {
      port: 3002,
      host: '0.0.0.0',
      proxy: {
        [VITE_API_URL_PREFIX]: {
          target: VITE_API_URL,
          changeOrigin: true,
          rewrite: (path) => path.replace(new RegExp(`^${VITE_API_URL_PREFIX}`), VITE_API_URL_PREFIX),
        },
        '/uploads': {
          target: VITE_API_URL,
          changeOrigin: true,
        },
      },
    },
  };
};
