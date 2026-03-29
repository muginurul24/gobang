<script lang="ts">
  import { appName } from '$lib';

  const services = [
    { name: 'API', path: 'apps/api', note: 'HTTP API bootstrap and health endpoints' },
    { name: 'Worker', path: 'apps/worker', note: 'Background reconcile and callback jobs' },
    { name: 'Scheduler', path: 'apps/scheduler', note: 'Periodic operational tasks' },
    { name: 'Docs', path: 'docs/', note: 'Architecture, schema, and upstream references' },
  ];
</script>

<svelte:head>
  <title>{appName} Dashboard</title>
  <meta
    name="description"
    content="Initial workspace for the onixggr multi-tenant API bridge platform."
  />
</svelte:head>

<main class="shell">
  <section class="hero">
    <p class="eyebrow">Repository Bootstrap</p>
    <h1>{appName}</h1>
    <p class="summary">
      Modular monolith scaffold for the multi-tenant API bridge. Backend services live in Go,
      while the dashboard starts in SvelteKit.
    </p>
    <div class="actions">
      <code>go run ./apps/api</code>
      <code>npm run dev:web</code>
    </div>
  </section>

  <section class="grid" aria-label="Project services">
    {#each services as service}
      <article class="card">
        <p class="card-label">{service.name}</p>
        <h2>{service.path}</h2>
        <p>{service.note}</p>
      </article>
    {/each}
  </section>
</main>

<style>
  .shell {
    width: min(1100px, calc(100% - 2rem));
    margin: 0 auto;
    padding: 3rem 0 5rem;
  }

  .hero {
    padding: 2rem;
    border: 1px solid rgba(16, 34, 26, 0.08);
    border-radius: 28px;
    background: rgba(255, 255, 255, 0.86);
    box-shadow: 0 20px 60px rgba(16, 34, 26, 0.08);
  }

  .eyebrow,
  .card-label {
    margin: 0 0 0.5rem;
    text-transform: uppercase;
    letter-spacing: 0.14em;
    font-size: 0.72rem;
    color: #0f766e;
    font-weight: 700;
  }

  h1 {
    margin: 0;
    font-size: clamp(2.5rem, 7vw, 5rem);
    line-height: 0.95;
  }

  .summary {
    max-width: 42rem;
    margin: 1rem 0 0;
    font-size: 1.05rem;
    line-height: 1.6;
    color: #294239;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    margin-top: 1.5rem;
  }

  code {
    padding: 0.75rem 1rem;
    border-radius: 999px;
    background: #10221a;
    color: #f8fdfb;
    font-size: 0.92rem;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 1rem;
    margin-top: 1.5rem;
  }

  .card {
    padding: 1.25rem;
    border-radius: 22px;
    background: rgba(255, 255, 255, 0.82);
    border: 1px solid rgba(16, 34, 26, 0.08);
    box-shadow: 0 16px 40px rgba(16, 34, 26, 0.05);
  }

  .card h2 {
    margin: 0;
    font-size: 1.15rem;
  }

  .card p:last-child {
    margin-bottom: 0;
    color: #355048;
    line-height: 1.55;
  }
</style>
