<script lang="ts">
  export let eyebrow = 'Empty State';
  export let title: string;
  export let body: string;
  export let actionHref = '';
  export let actionLabel = '';
</script>

<section class="glass-panel relative overflow-hidden rounded-[2rem] px-5 py-6 text-sm text-ink-700">
  <div class="absolute inset-x-6 top-0 h-px bg-linear-to-r from-transparent via-brand-300/70 to-transparent"></div>
  <p class="section-kicker">{eyebrow}</p>
  <h3 class="mt-3 font-display text-2xl font-bold tracking-tight text-ink-900">{title}</h3>
  <p class="mt-3 max-w-2xl leading-6">{body}</p>

  {#if actionHref !== '' && actionLabel !== ''}
    <a
      class="mt-5 inline-flex rounded-full border border-white/70 bg-white px-4 py-2 font-semibold text-ink-900 transition hover:-translate-y-0.5 hover:border-accent-300 hover:text-accent-700"
      href={actionHref}
    >
      {actionLabel}
    </a>
  {/if}
</section>
