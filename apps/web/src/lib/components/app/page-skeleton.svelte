<script lang="ts">
  export let blocks = 3;

  $: items = Array.from({ length: blocks });
</script>

<div class="space-y-6" aria-hidden="true">
  <section class="surface-dark surface-grid animate-pulse rounded-[2rem] px-6 py-7 text-white">
    <div class="h-3 w-28 rounded-full bg-white/15"></div>
    <div class="mt-4 h-10 w-2/3 rounded-full bg-white/12"></div>
    <div class="mt-4 h-3 w-full rounded-full bg-white/10"></div>
    <div class="mt-2 h-3 w-5/6 rounded-full bg-white/10"></div>
  </section>

  <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
    {#each items as _, index}
      <article class="glass-panel animate-pulse rounded-[1.8rem] px-5 py-5">
        <div class="h-3 w-24 rounded-full bg-canvas-100/80"></div>
        <div class="mt-4 h-8 w-32 rounded-full bg-canvas-100/80"></div>
        <div class="mt-4 h-3 w-full rounded-full bg-canvas-100/80"></div>
        <div class="mt-2 h-3 w-2/3 rounded-full bg-canvas-100/80"></div>
      </article>
    {/each}
  </div>
</div>
