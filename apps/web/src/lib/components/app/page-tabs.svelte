<script lang="ts">
  type PageTabItem = {
    id: string;
    label: string;
    description?: string;
    badge?: string;
  };

  export let items: PageTabItem[] = [];
  export let value = '';
</script>

<div class="page-tabs" role="tablist" aria-label="Page workspace tabs">
  {#each items as item}
    <button
      class="page-tabs__item"
      data-active={item.id === value}
      role="tab"
      aria-selected={item.id === value}
      type="button"
      onclick={() => {
        value = item.id;
      }}
    >
      <span class="page-tabs__label-row">
        <span class="page-tabs__label">{item.label}</span>
        {#if item.badge}
          <span class="page-tabs__badge">{item.badge}</span>
        {/if}
      </span>
      {#if item.description}
        <span class="page-tabs__description">{item.description}</span>
      {/if}
    </button>
  {/each}
</div>
