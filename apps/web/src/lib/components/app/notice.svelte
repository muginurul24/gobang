<script lang="ts">
  type Tone = 'error' | 'success' | 'info' | 'warning';

  export let tone: Tone = 'info';
  export let title = '';
  export let message: string;

  const toneClass: Record<Tone, string> = {
    error: 'border-rose-200 bg-rose-50 text-rose-700',
    success: 'border-brand-200 bg-brand-100/60 text-brand-700',
    info: 'border-accent-200 bg-accent-50 text-accent-700',
    warning: 'border-amber-200 bg-amber-50 text-amber-800'
  };
</script>

<article class={`rounded-3xl border px-4 py-3 text-sm ${toneClass[tone]}`}>
  {#if title !== ''}
    <p class="font-semibold">{title}</p>
  {/if}
  <p class={title !== '' ? 'mt-1 leading-6' : 'leading-6'}>{message}</p>
</article>
