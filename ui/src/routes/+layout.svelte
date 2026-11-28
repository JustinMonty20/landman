<script lang="ts">
  import { onMount, type Snippet } from 'svelte';
  import { Avatar, Menu } from '@skeletonlabs/skeleton-svelte';
  import Brand from '../lib/Brand.svelte';
  import { page } from '$app/state';
  import '../style.css';

  let { children }: { children: Snippet } = $props();
  type User = { id: string; email: string; name: string; profile_image_url?: string };
  type Access = { kind: 'loading' } | { kind: 'signed-out' } | { kind: 'unavailable' } | { kind: 'signed-in'; user: User };
  let access = $state<Access>({ kind: 'loading' });
  let busy = $state(false);
  let message = $state('');
  let menuOpen = $state(false);
  const initials = (name: string) => name.trim().split(/\s+/).slice(0, 2).map(part => part[0]?.toUpperCase() ?? '').join('') || '?';
  const loginErrors: Record<string, string> = {
    not_allowed: 'This Google account is not approved for the private alpha.',
    invalid_login: 'The login could not be verified. Start again.',
    cancelled: 'Google sign-in was cancelled. You can try again.',
    provider_failed: 'Google sign-in failed. You can try again.'
  };

  async function loadSession() {
    access = { kind: 'loading' };
    try {
      const response = await fetch('/api/me', { credentials: 'same-origin', cache: 'no-store' });
      if (response.status === 401) { access = { kind: 'signed-out' }; return; }
      if (!response.ok) throw new Error('Session unavailable');
      const user = await response.json();
      if (typeof user.id !== 'string' || typeof user.email !== 'string' || typeof user.name !== 'string') throw new Error('Invalid session');
      access = { kind: 'signed-in', user: { ...user, profile_image_url: typeof user.profile_image_url === 'string' ? user.profile_image_url : '' } };
    } catch {
      access = { kind: 'unavailable' };
    }
  }

  async function logout() {
    if (busy) return;
    busy = true;
    message = '';
    try {
      const response = await fetch('/auth/logout', { method: 'POST', credentials: 'same-origin' });
      if (!response.ok) throw new Error('Logout failed');
      menuOpen = false;
      access = { kind: 'signed-out' };
    } catch {
      message = 'Sign-out failed. Try again.';
      menuOpen = true;
    } finally { busy = false; }
  }

  onMount(() => {
    const url = new URL(window.location.href);
    const reason = url.searchParams.get('auth_error');
    if (reason) {
      message = loginErrors[reason] ?? 'Sign-in failed. Start again.';
      url.searchParams.delete('auth_error');
      window.history.replaceState(null, '', url.pathname + url.search + url.hash);
    }
    void loadSession();
  });
</script>

{#if access.kind === 'signed-in'}
  <header class="operator-header">
    <Brand />
    <nav aria-label="Main navigation"><a href="/" aria-current={page.url.pathname === '/' ? 'page' : undefined}>Parcels</a><a href="/leads" aria-current={page.url.pathname === '/leads' ? 'page' : undefined}>Leads</a><a href="/tasks" aria-current={page.url.pathname === '/tasks' ? 'page' : undefined}>Tasks</a></nav>
    <Menu open={menuOpen} onOpenChange={(details) => menuOpen = details.open} onSelect={(details) => { if (details.value === 'signout') void logout(); }}>
      <Menu.Trigger class="account-trigger" aria-label="Account menu" aria-expanded={menuOpen}>
        <Avatar class="account-avatar"><Avatar.Image src={access.user.profile_image_url || undefined} alt="" referrerpolicy="no-referrer" /><Avatar.Fallback>{initials(access.user.name)}</Avatar.Fallback></Avatar>
      </Menu.Trigger>
      <Menu.Positioner><Menu.Content class="account-menu">
        <div class="account-identity"><strong>{access.user.name}</strong><span>{access.user.email}</span></div>
        <Menu.Separator />
        <Menu.Item value="signout" closeOnSelect={false} disabled={busy}>{busy ? 'Signing out…' : 'Sign out'}</Menu.Item>
        {#if message}<p role="alert" class="account-error">{message}</p>{/if}
      </Menu.Content></Menu.Positioner>
    </Menu>
  </header>
  {@render children()}
{:else}
  <main class="login-screen">
    <section class="login-card" aria-label="Application access">
      <Brand />
      <h1>Private alpha</h1>
      {#if access.kind === 'loading'}
        <p role="status">Checking your session…</p>
      {:else if access.kind === 'unavailable'}
        <p role="alert">The sign-in service is unavailable. Start the application API, then try again.</p>
        <button class="btn preset-outlined-surface-300-700 bg-surface-900" onclick={loadSession}>Try again</button>
      {:else}
        <p>Sign in with an approved Google account.</p>
        {#if message}<p role="alert">{message}</p>{/if}
        <a class="google-signin" href="/auth/google/start"><img src="/brand/google-signin-dark.png" alt="Sign in with Google" width="180" height="40" /></a>
        <p class="fine">Access is limited to approved accounts. Calendar access is not requested.</p>
      {/if}
    </section>
  </main>
{/if}

<style>
  .login-screen { min-height: 100dvh; display: grid; place-items: center; padding: 24px; }
  .login-card { width: min(410px, 100%); padding: 32px; background: var(--surface); border: 1px solid var(--border); border-radius: 10px; }
  h1 { font-size: 24px; font-weight: 600; margin: 0; }
  p { line-height: 1.6; }
  .login-card > p { margin: 16px 0; }
  .login-card > :global(.brand) { width: fit-content; margin: 0 0 28px; }
  .google-signin { display: inline-flex; margin: 24px 0; border-radius: 4px; }
  .google-signin img { display: block; width: 180px; height: 40px; }
  .operator-header { gap: 24px; }
  .operator-header nav { display: flex; align-items: center; gap: 16px; margin-right: auto; font-size: 13px; }
  .operator-header nav a { color: var(--color-surface-100); text-decoration: none; }
  :global(.account-trigger) { display: grid; place-items: center; width: 40px; height: 40px; padding: 0; border: 0; border-radius: 50%; background: transparent; cursor: pointer; }
  :global(.account-avatar) { display: grid; place-items: center; width: 36px; height: 36px; overflow: hidden; border-radius: 50%; background: var(--color-primary-700); color: white; font-size: 12px; font-weight: 700; }
  :global(.account-avatar img) { width: 100%; height: 100%; object-fit: cover; }
  :global(.account-menu) { min-width: 210px; max-width: min(300px, calc(100vw - 24px)); padding: 8px; background: var(--surface); border: 1px solid var(--border); border-radius: 8px; box-shadow: 0 8px 24px #0008; z-index: 20; }
  .account-identity { display: flex; flex-direction: column; gap: 2px; padding: 8px; font-size: 13px; overflow-wrap: anywhere; }
  .account-identity span { font-size: 11px; color: var(--muted); }
  .account-error { padding: 8px; color: #fbbf74; font-size: 12px; }
</style>
