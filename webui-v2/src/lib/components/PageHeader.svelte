<script lang="ts">
  import type { Snippet } from "svelte";

  let {
    id,
    eyebrow,
    title,
    description = "",
    actions,
    className = ""
  }: {
    id?: string;
    eyebrow?: string;
    title: string;
    description?: string;
    actions?: Snippet;
    className?: string;
  } = $props();
</script>

<header class={`page-heading ${className}`.trim()}>
  <div class="heading-copy">
    {#if eyebrow}<p class="eyebrow">{eyebrow}</p>{/if}
    <h1 id={id}>{title}</h1>
    {#if description}<p class="description">{description}</p>{/if}
  </div>
  {#if actions}<div class="heading-actions">{@render actions()}</div>{/if}
</header>

<style>
  .page-heading { align-items: flex-start; border-bottom: 1px solid var(--tw-border); display: flex; gap: 1rem; justify-content: space-between; min-width: 0; padding-bottom: 1.15rem; }
  .heading-copy { min-width: 0; }
  .eyebrow { color: var(--tw-text-muted); font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, .description { overflow-wrap: anywhere; }
  h1 { color: var(--tw-text); font-size: 2rem; line-height: 1.2; margin: 0; }
  .description { color: var(--tw-text-muted); margin: .4rem 0 0; }
  .heading-actions { align-items: center; display: flex; flex-wrap: wrap; gap: .65rem; justify-content: flex-end; min-width: 0; }
  .heading-actions :global(form) { display: flex; min-width: 0; }
  .heading-actions :global(a), .heading-actions :global(button) { max-width: 100%; min-height: var(--tw-control-height); overflow-wrap: anywhere; }
  @media (max-width: 600px) {
    .page-heading { align-items: stretch; flex-direction: column; }
    .heading-actions, .heading-actions :global(form), .heading-actions :global(a), .heading-actions :global(button) { width: 100%; }
    .heading-actions { align-items: stretch; justify-content: stretch; }
  }
</style>
