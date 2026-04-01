import type { ResolvedTheme } from '$lib/theme';

export function chartTextColor(theme: ResolvedTheme) {
  return theme === 'dark' ? 'rgba(248, 241, 223, 0.78)' : '#6b5d45';
}

export function chartMutedTextColor(theme: ResolvedTheme) {
  return theme === 'dark' ? 'rgba(214, 204, 186, 0.58)' : '#8f846f';
}

export function chartGridColor(theme: ResolvedTheme) {
  return theme === 'dark'
    ? 'rgba(255, 255, 255, 0.08)'
    : 'rgba(98, 84, 49, 0.12)';
}
