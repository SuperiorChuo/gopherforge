declare module 'tvision-color' {
  type ColorFormat = 'hex' | 'hsl' | 'rgb';

  interface ColorGradationOptions {
    colors: string[];
    step: number;
    remainInput?: boolean;
  }

  interface ColorGradation {
    colors: string[];
    primary: number;
  }

  interface RandomPaletteOptions {
    color?: string;
    count?: number;
    colorGamut?: 'bright' | 'dark' | 'light' | string;
    number?: number;
  }

  export const Color: {
    colorTransform: (value: string | Array<number | string>, from: ColorFormat, to: ColorFormat) => string | string[];
    getColorGradations?: (options: ColorGradationOptions) => ColorGradation[];
    getPaletteByGradation?: (options: ColorGradationOptions) => string[][];
    getRandomPalette: (options?: RandomPaletteOptions) => string[];
  };
}
