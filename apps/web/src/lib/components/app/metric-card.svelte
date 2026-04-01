<script lang="ts">
  import { cn } from '$lib/utils';

  type Tone = 'brand' | 'accent' | 'default' | 'danger';

  export let eyebrow = '';
  export let title: string;
  export let value: string;
  export let detail = '';
  export let badge = '';
  export let tone: Tone = 'default';
  let className = '';
  export { className as class };

  function toneClass(selected: Tone) {
    switch (selected) {
      case 'brand':
        return 'metric-card--brand';
      case 'accent':
        return 'metric-card--accent';
      case 'danger':
        return 'metric-card--danger';
      default:
        return 'metric-card--default';
    }
  }
</script>

<article class={cn('metric-card', toneClass(tone), className)}>
  <div class="flex items-start justify-between gap-4">
    <div class="space-y-2">
      {#if eyebrow !== ''}
        <p class="text-[0.72rem] font-semibold uppercase tracking-[0.32em] text-ink-300">
          {eyebrow}
        </p>
      {/if}
      <h3 class="text-sm font-medium text-ink-700">{title}</h3>
    </div>

    {#if badge !== ''}
      <span class="metric-card__badge">{badge}</span>
    {/if}
  </div>

  <div class="mt-6 space-y-3">
    <p class="font-display text-3xl font-semibold tracking-tight text-ink-900 sm:text-4xl">
      {value}
    </p>

    {#if detail !== ''}
      <p class="max-w-md text-sm leading-6 text-ink-700">{detail}</p>
    {/if}
  </div>
</article>
