<script lang="ts">
import ParcelTasks from '../../lib/ParcelTasks.svelte';
import '../../style.css';
</script>
<svelte:head><title>Tasks | Landman</title></svelte:head>
<main class="task-page"><p class="eyebrow">YOUR WORKSPACE</p><h1>Tasks</h1><p>Upcoming and overdue personal tasks across your parcels. Select a parcel to create a task.</p><ParcelTasks /></main>
<style>
.task-page{min-height:100dvh;padding:24px clamp(20px,4vw,64px) 48px;background:var(--color-surface-950);color:var(--color-surface-50)}
.eyebrow{color:var(--color-primary-300);font-size:10px;font-weight:800;letter-spacing:.17em;margin:0 0 8px}.task-page h1{font-size:clamp(28px,3vw,38px);font-weight:720}.task-page>p{margin:9px 0;color:var(--muted)}
@media(max-width:800px){.task-page{padding:64px 16px 24px}}
</style>
