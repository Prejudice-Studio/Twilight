<script lang="ts">
  import AppShell from "$lib/components/AppShell.svelte";
  import "$lib/app.css";
  import type { LayoutData } from "./$types";

  let { data, children }: { data: LayoutData; children: import("svelte").Snippet } = $props();
</script>

<AppShell user={data.user}>{@render children()}</AppShell>
