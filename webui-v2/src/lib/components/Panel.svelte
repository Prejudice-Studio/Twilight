<script lang="ts">
  import type { Snippet } from "svelte";

  let {
    id,
    title,
    description = "",
    children,
    className = ""
  }: {
    id?: string;
    title?: string;
    description?: string;
    children: Snippet;
    className?: string;
  } = $props();
</script>

<section class={`panel ${className}`.trim()} aria-labelledby={id}>
  {#if title}
    <div class="panel-heading">
      <div>
        <h2 id={id}>{title}</h2>
        {#if description}<p>{description}</p>{/if}
      </div>
    </div>
  {/if}
  {@render children()}
</section>

<style>
  .panel { background: var(--tw-surface); border: 1px solid var(--tw-border); border-radius: .45rem; display: grid; gap: 1rem; min-width: 0; padding: 1rem; }
  .panel-heading { border-bottom: 1px solid var(--tw-border); min-width: 0; padding-bottom: .8rem; }
  h2 { color: var(--tw-text); font-size: 1.2rem; line-height: 1.3; margin: 0; overflow-wrap: anywhere; }
  .panel-heading p { color: var(--tw-text-muted); margin: .35rem 0 0; overflow-wrap: anywhere; }
  @media (max-width: 560px) { .panel { padding: .85rem; } }
</style>
