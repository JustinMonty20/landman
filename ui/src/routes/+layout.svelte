<script lang="ts">
  import { onMount, type Snippet } from 'svelte';
  import '../style.css';

  let { children }: { children: Snippet } = $props();
  type User = { id: string; email: string; name: string };
  type Access = { kind: 'loading' } | { kind: 'signed-out' } | { kind: 'unavailable' } | { kind: 'signed-in'; user: User };
  let access = $state<Access>({ kind: 'loading' });
  let busy = $state(false);
  let message = $state('');
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
      access = { kind: 'signed-in', user };
    } catch {
      access = { kind: 'unavailable' };
    }
  }

  async function logout() {
    busy = true;
    message = '';
    try {
      const response = await fetch('/auth/logout', { method: 'POST', credentials: 'same-origin' });
      if (!response.ok) throw new Error('Logout failed');
      access = { kind: 'signed-out' };
    } catch {
      message = 'Sign-out failed. Try again.';
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
  {@render children()}
  <div class="account-controls" aria-label="Signed-in account"><span>{access.user.email}</span><button class="btn btn-sm preset-outlined-surface-300-700 bg-surface-900" onclick={logout} disabled={busy}>{busy ? 'Signing out…' : 'Sign out'}</button></div>
  {#if message}<p class="account-error" role="alert">{message}</p>{/if}
{:else}
  <main class="login-screen">
    <section class="login-card" aria-label="Application access">
      <p class="login-brand">landman</p>
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
  .login-brand { color: #93c5fd; font-size: 25px; font-weight: 700; margin: 0 0 28px; }
  h1 { font-size: 24px; font-weight: 600; margin: 0; }
  p { line-height: 1.6; }
  .login-card > p { margin: 16px 0; }
  .login-card > .login-brand { margin: 0 0 28px; }
  .google-signin { display: inline-flex; margin: 24px 0; border-radius: 4px; }
  .google-signin img { display: block; width: 180px; height: 40px; }
  .account-controls { position: fixed; top: 6px; right: 12px; height: 40px; z-index: 8; display: flex; align-items: center; gap: 12px; background: var(--app-header); font-size: 12px; }
  .account-controls span { max-width: 220px; overflow: hidden; text-overflow: ellipsis; }
  .account-controls button { min-height: 36px; }
  .account-error { position: fixed; top: 55px; right: 12px; padding: 12px; z-index: 8; background: var(--surface); border: 1px solid #edba71; }
  @media (max-width: 900px) { .account-controls span { max-width: 150px; } }
</style>
