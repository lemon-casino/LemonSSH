import type { Monaco } from '@monaco-editor/react';
import { useEffect, useState } from 'react';
import {
  buildLemonSSHMonacoThemeColors,
  getLemonSSHEditorColors,
  getLemonSSHMonacoThemeName,
  getLemonSSHThemeSignal,
  LEMONSSH_MONACO_THEME_DARK,
  LEMONSSH_MONACO_THEME_LIGHT,
} from './lemonsshMonacoTheme';

export const useLemonsshMonacoTheme = (
  monaco: Monaco | null | undefined,
): string => {
  const [isDarkTheme, setIsDarkTheme] = useState(() =>
    typeof document !== 'undefined' && document.documentElement.classList.contains('dark'),
  );
  const [themeSignal, setThemeSignal] = useState(() => getLemonSSHThemeSignal());
  const themeName = getLemonSSHMonacoThemeName(isDarkTheme);

  useEffect(() => {
    if (!monaco) return;

    const colors = getLemonSSHEditorColors(isDarkTheme);
    const themeColors = buildLemonSSHMonacoThemeColors(colors);

    monaco.editor.defineTheme(LEMONSSH_MONACO_THEME_DARK, {
      base: 'vs-dark',
      inherit: true,
      rules: [],
      colors: themeColors,
    });

    monaco.editor.defineTheme(LEMONSSH_MONACO_THEME_LIGHT, {
      base: 'vs',
      inherit: true,
      rules: [],
      colors: themeColors,
    });

    monaco.editor.setTheme(themeName);
  }, [monaco, isDarkTheme, themeSignal, themeName]);

  useEffect(() => {
    if (typeof document === 'undefined' || typeof MutationObserver === 'undefined') return;
    const root = document.documentElement;
    const updateTheme = () => {
      setIsDarkTheme(root.classList.contains('dark'));
      setThemeSignal(getLemonSSHThemeSignal());
    };
    const observer = new MutationObserver(updateTheme);
    observer.observe(root, {
      attributes: true,
      attributeFilter: ['class', 'style', 'data-active-chrome-theme'],
    });
    return () => observer.disconnect();
  }, []);

  return themeName;
};
