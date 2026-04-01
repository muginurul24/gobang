<script lang="ts">
  import { clamp } from '$lib/formatters';
  import { cn } from '$lib/utils';

  type Tone = 'brand' | 'accent' | 'slate';

  export let label: string;
  export let value = 0;
  export let max = 100;
  export let detail = '';
  export let suffix = '%';
  export let tone: Tone = 'brand';
  let className = '';
  export { className as class };

  $: ratio = max <= 0 ? 0 : clamp(value / max, 0, 1);
  $: degrees = Math.round(ratio * 360);
  $: gaugeStyle = `background: conic-gradient(${toneColor(tone)} ${degrees}deg, rgba(17, 23, 19, 0.08) ${degrees}deg 360deg);`;

  function toneColor(selected: Tone) {
    switch (selected) {
      case 'accent':
        return '#e7b34b';
      case 'slate':
        return '#4f645b';
      default:
        return '#21c977';
    }
  }
</script>

<article class={cn('gauge-card', className)}>
  <div class="gauge-card__ring" style={gaugeStyle}>
    <div class="gauge-card__core">
      <p class="gauge-card__value">{value.toFixed(1)}{suffix}</p>
    </div>
  </div>

  <div class="space-y-2">
    <p class="text-[0.72rem] font-semibold uppercase tracking-[0.32em] text-ink-300">{label}</p>
    {#if detail !== ''}
      <p class="text-sm leading-6 text-ink-700">{detail}</p>
    {/if}
  </div>
</article>
