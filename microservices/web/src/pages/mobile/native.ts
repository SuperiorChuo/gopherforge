// 下游脚手架不含 Capacitor 壳工程（npm 依赖与 ios/android 目录仅在主仓）。
// 网页形态恒为 false；保留 data-native-app="1" DOM 标记作为原生壳注入时的
// 兼容通道，行为与主仓一致——原生壳里禁 PWA 注册、显示退出按钮。
export function isNativeApp(): boolean {
  return document.documentElement.dataset.nativeApp === '1'
}
