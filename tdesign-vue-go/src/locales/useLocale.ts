import { useLocalStorage } from '@vueuse/core';
import type { GlobalConfigProvider } from 'tdesign-vue-next';
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

import { defaultLocale, i18n, langCode, localeConfigKey } from '@/locales/index';

export function useLocale() {
  const { locale } = useI18n({ useScope: 'global' });
  function changeLocale(lang: string) {
    // 如果切换的语言不在对应语言文件里则默认为简体中文
    if (!langCode.includes(lang)) {
      lang = defaultLocale;
    }

    locale.value = lang;
    useLocalStorage(localeConfigKey, defaultLocale).value = lang;
  }

  const getComponentsLocale = computed(() => {
    const message = i18n.global.getLocaleMessage(locale.value) as { componentsLocale?: GlobalConfigProvider };
    return message.componentsLocale || {};
  });

  return {
    changeLocale,
    getComponentsLocale,
    locale,
  };
}
