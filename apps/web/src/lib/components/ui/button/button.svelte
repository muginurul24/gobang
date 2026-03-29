<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';

  import { cn } from '$lib/utils';

  import { buttonVariants, type ButtonSize, type ButtonVariant } from './index';

  type Props = HTMLButtonAttributes & {
    children?: Snippet;
    variant?: ButtonVariant;
    size?: ButtonSize;
    class?: string;
  };

  let {
    children,
    variant = 'default',
    size = 'default',
    class: className = '',
    type = 'button',
    ...restProps
  }: Props = $props();
</script>

<button class={cn(buttonVariants({ variant, size }), className)} {type} {...restProps}>
  {@render children?.()}
</button>
