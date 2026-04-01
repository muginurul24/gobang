<script lang="ts">
  type Tone = 'error' | 'success' | 'info' | 'warning';

  export let tone: Tone = 'info';
  export let title = '';
  export let message: string;

  const toneClass: Record<Tone, string> = {
    error: 'notice-panel--error',
    success: 'notice-panel--success',
    info: 'notice-panel--info',
    warning: 'notice-panel--warning'
  };
</script>

<article class={`notice-panel ${toneClass[tone]}`}>
  {#if title !== ''}
    <p class="font-semibold uppercase tracking-[0.16em]">{title}</p>
  {/if}
  <p class={title !== '' ? 'mt-1 leading-6' : 'leading-6'}>{message}</p>
</article>
