<script lang="ts">
  import { onMount } from 'svelte';
  import type { Chart, ChartConfiguration } from 'chart.js';

  import { cn } from '$lib/utils';

  export let config: ChartConfiguration;
  let className = '';
  export { className as class };

  let canvasElement: HTMLCanvasElement | null = null;
  let chartInstance: Chart | null = null;
  let mounted = false;

  async function renderChart() {
    if (!mounted || !canvasElement) {
      return;
    }

    const { Chart, registerables } = await import('chart.js');
    Chart.register(...registerables);

    chartInstance?.destroy();
    chartInstance = new Chart(canvasElement, config);
  }

  onMount(() => {
    mounted = true;
    void renderChart();

    return () => {
      chartInstance?.destroy();
    };
  });

  $: if (mounted && config) {
    void renderChart();
  }
</script>

<div class={cn('relative min-h-[240px]', className)}>
  <canvas bind:this={canvasElement}></canvas>
</div>
