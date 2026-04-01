<script lang="ts">
  import Button from '$lib/components/ui/button/button.svelte';
  import { resolvedTheme, setTheme, themePreference, type ThemePreference } from '$lib/theme';

  export let compact = false;

  const options: Array<{ value: ThemePreference; label: string; short: string }> = [
    { value: 'system', label: 'System', short: 'SYS' },
    { value: 'light', label: 'Light', short: 'LGT' },
    { value: 'dark', label: 'Dark', short: 'DRK' }
  ];

  $: stateLabel = $themePreference === 'system' ? `auto · ${$resolvedTheme}` : $resolvedTheme;

  function isActive(option: ThemePreference) {
    return $themePreference === option;
  }
</script>

<div class={`theme-toggle ${compact ? 'theme-toggle--compact' : ''}`}>
  <div class="theme-toggle__header">
    <span class="theme-toggle__kicker">Theme</span>
    <span class="theme-toggle__state">{stateLabel}</span>
  </div>

  <div class="theme-toggle__actions">
    {#each options as option}
      <Button
        variant="ghost"
        size={compact ? 'sm' : 'default'}
        class={isActive(option.value) ? 'theme-toggle__button--active' : 'theme-toggle__button'}
        aria-pressed={isActive(option.value)}
        title={`Gunakan tema ${option.label.toLowerCase()}`}
        onclick={() => setTheme(option.value)}
      >
        {compact ? option.short : option.label}
      </Button>
    {/each}
  </div>
</div>
