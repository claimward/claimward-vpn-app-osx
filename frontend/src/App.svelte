<script>
  import { onMount } from 'svelte'
  import { api } from './api.js'

  let status = null
  let busy = ''
  let error = ''

  async function refresh() {
    try {
      status = await api('GET', '/api/status')
      error = ''
    } catch (e) {
      error = String(e.message || e)
    }
  }

  async function run(label, method, path) {
    busy = label
    error = ''
    try {
      status = await api(method, path)
    } catch (e) {
      error = String(e.message || e)
    } finally {
      busy = ''
    }
  }

  onMount(() => {
    refresh()
    const id = setInterval(refresh, 4000)
    return () => clearInterval(id)
  })

  $: connected = status && status.connected
  $: loggedIn = status && status.logged_in
  $: configOK = status && status.config_ok
  $: deviceCode = status && status.device_user_code
</script>

<main>
  <header>
    <div class="logo">▲</div>
    <h1>Claimward</h1>
  </header>

  {#if !status}
    <p class="muted">Loading…</p>
  {:else if !configOK}
    <div class="card warn">
      <strong>Not configured</strong>
      <p class="muted">{status.config_error}</p>
      <p class="muted small">
        Set <code>server_url</code>, <code>oidc_issuer</code> and
        <code>oidc_client_id</code> in
        <code>~/Library/Application&nbsp;Support/Claimward/config.json</code>.
      </p>
    </div>
  {:else}
    <div class="card status {connected ? 'on' : 'off'}">
      <div class="dot"></div>
      <div>
        <div class="state">{connected ? 'Connected' : loggedIn ? 'Disconnected' : 'Signed out'}</div>
        {#if connected && status.assigned_ip}
          <div class="muted small">{status.assigned_ip} · {status.interface}</div>
        {:else if status.server_url}
          <div class="muted small">{status.server_url}</div>
        {/if}
      </div>
    </div>

    {#if status.email}
      <p class="muted small center">Signed in as {status.email}</p>
    {/if}

    {#if deviceCode}
      <div class="card device">
        <p class="muted small">To sign in, open</p>
        <a href={status.device_verification_uri} target="_blank" rel="noreferrer">
          {status.device_verification_uri}
        </a>
        <p class="muted small">and enter the code</p>
        <div class="code">{status.device_user_code}</div>
      </div>
    {/if}

    <div class="actions">
      {#if !loggedIn}
        <button class="primary" disabled={!!busy} on:click={() => run('login', 'POST', '/api/login')}>
          {busy === 'login' ? 'Opening browser…' : 'Sign in'}
        </button>
      {:else if connected}
        <button class="danger" disabled={!!busy} on:click={() => run('disconnect', 'POST', '/api/disconnect')}>
          {busy === 'disconnect' ? 'Disconnecting…' : 'Disconnect'}
        </button>
      {:else}
        <button class="primary" disabled={!!busy} on:click={() => run('connect', 'POST', '/api/connect')}>
          {busy === 'connect' ? 'Connecting…' : 'Connect'}
        </button>
      {/if}

      {#if loggedIn}
        <button class="ghost" disabled={!!busy} on:click={() => run('logout', 'POST', '/api/logout')}>
          Sign out
        </button>
      {/if}
    </div>

    {#if !status.helper_installed}
      <p class="muted small center warn-text">
        Privileged helper not detected — install it to connect (see README).
      </p>
    {/if}
  {/if}

  {#if error}
    <p class="error">{error}</p>
  {/if}
</main>

<style>
  :global(body) {
    margin: 0;
  }
  main {
    font-family: -apple-system, BlinkMacSystemFont, system-ui, sans-serif;
    color: #f5f5fa;
    padding: 28px 24px;
    min-height: 100vh;
    box-sizing: border-box;
    background: radial-gradient(120% 120% at 50% 0%, #2a2a52 0%, #14142b 60%, #0d0d1c 100%);
  }
  header {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 22px;
  }
  .logo {
    font-size: 22px;
    color: #7c8cff;
  }
  h1 {
    font-size: 19px;
    font-weight: 600;
    margin: 0;
    letter-spacing: 0.2px;
  }
  .card {
    background: rgba(255, 255, 255, 0.06);
    border: 1px solid rgba(255, 255, 255, 0.09);
    border-radius: 14px;
    padding: 16px;
    margin-bottom: 16px;
  }
  .status {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  .dot {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    flex: none;
  }
  .status.on .dot {
    background: #36d27a;
    box-shadow: 0 0 12px #36d27a99;
  }
  .status.off .dot {
    background: #6b6b85;
  }
  .state {
    font-size: 16px;
    font-weight: 600;
  }
  .muted {
    color: #a9a9c4;
  }
  .small {
    font-size: 12px;
  }
  .center {
    text-align: center;
  }
  .actions {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin-top: 18px;
  }
  button {
    border: none;
    border-radius: 11px;
    padding: 12px 14px;
    font-size: 15px;
    font-weight: 600;
    cursor: pointer;
    transition: opacity 0.15s, transform 0.05s;
  }
  button:active {
    transform: translateY(1px);
  }
  button:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .primary {
    background: #5b6bff;
    color: white;
  }
  .danger {
    background: #ff5b6b;
    color: white;
  }
  .ghost {
    background: transparent;
    color: #a9a9c4;
    border: 1px solid rgba(255, 255, 255, 0.12);
  }
  .device {
    text-align: center;
  }
  .device .code {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 26px;
    font-weight: 700;
    letter-spacing: 4px;
    margin-top: 8px;
    color: #fff;
  }
  .warn {
    border-color: #e0a23a55;
  }
  .warn-text {
    color: #e0a23a;
  }
  .error {
    color: #ff8b97;
    font-size: 13px;
    text-align: center;
    margin-top: 14px;
  }
  code {
    background: rgba(255, 255, 255, 0.08);
    padding: 1px 5px;
    border-radius: 5px;
    font-size: 11px;
  }
</style>
