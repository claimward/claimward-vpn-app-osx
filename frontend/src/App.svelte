<script>
  import { onMount } from 'svelte'
  import { api } from './api.js'

  let status = null
  let busy = ''
  let error = ''
  let view = 'main' // 'main' | 'settings'
  let cfg = null
  let savingCfg = false

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

  async function openSettings() {
    error = ''
    try {
      cfg = await api('GET', '/api/config')
      if (!cfg.provider) cfg.provider = 'github'
      view = 'settings'
    } catch (e) {
      error = String(e.message || e)
    }
  }

  async function saveSettings() {
    savingCfg = true
    error = ''
    try {
      await api('POST', '/api/config', cfg)
      await refresh()
      view = 'main'
    } catch (e) {
      error = String(e.message || e)
    } finally {
      savingCfg = false
    }
  }

  onMount(() => {
    refresh()
    if (location.hash === '#settings') openSettings()
    const id = setInterval(() => view === 'main' && refresh(), 4000)
    return () => clearInterval(id)
  })

  $: connected = status && status.connected
  $: loggedIn = status && status.logged_in
  $: configOK = status && status.config_ok
  $: deviceCode = status && status.device_user_code
</script>

<main>
  <header>
    <img class="logo" src="claimward-lockup.svg" alt="Claimward" />
    {#if status}
      <button
        class="gear"
        title="Configuration"
        aria-label="Configuration"
        on:click={() => (view === 'settings' ? (view = 'main') : openSettings())}
      >⚙</button>
    {/if}
  </header>

  {#if view === 'settings' && cfg}
    <div class="settings">
      <h2>Configuration</h2>
      <label>Server URL
        <input bind:value={cfg.server_url} placeholder="https://vpn.example.com" autocapitalize="off" autocorrect="off" spellcheck="false" />
      </label>
      <label>Identity provider
        <select bind:value={cfg.provider}>
          <option value="github">GitHub (device flow)</option>
          <option value="oidc">OIDC</option>
        </select>
      </label>
      {#if cfg.provider === 'oidc'}
        <label>OIDC issuer
          <input bind:value={cfg.oidc_issuer} placeholder="https://issuer.example.com" autocapitalize="off" autocorrect="off" spellcheck="false" />
        </label>
        <label>OIDC client ID
          <input bind:value={cfg.oidc_client_id} autocapitalize="off" autocorrect="off" spellcheck="false" />
        </label>
      {:else}
        <label>GitHub client ID
          <input bind:value={cfg.github_client_id} placeholder="Iv1.0123456789abcdef" autocapitalize="off" autocorrect="off" spellcheck="false" />
        </label>
      {/if}
      <div class="actions">
        <button class="primary" disabled={savingCfg} on:click={saveSettings}>
          {savingCfg ? 'Saving…' : 'Save'}
        </button>
        <button class="ghost" disabled={savingCfg} on:click={() => (view = 'main')}>Cancel</button>
      </div>
    </div>
  {:else if !status}
    <p class="muted">Loading…</p>
  {:else if !configOK}
    <div class="card warn">
      <strong>Not configured</strong>
      <p class="muted">{status.config_error}</p>
      <button class="primary" on:click={openSettings}>Open configuration</button>
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
    justify-content: space-between;
    gap: 10px;
    margin-bottom: 22px;
  }
  .gear {
    background: transparent;
    border: none;
    color: var(--muted, #a9a9c4);
    font-size: 20px;
    line-height: 1;
    padding: 6px;
    cursor: pointer;
    border-radius: 8px;
  }
  .gear:hover {
    color: #fff;
    background: rgba(255, 255, 255, 0.08);
  }
  .settings h2 {
    font-size: 17px;
    margin: 0 0 16px;
  }
  .settings label {
    display: block;
    font-size: 12px;
    color: #a9a9c4;
    margin-bottom: 14px;
  }
  .settings input,
  .settings select {
    display: block;
    width: 100%;
    box-sizing: border-box;
    margin-top: 5px;
    padding: 9px 11px;
    border-radius: 9px;
    border: 1px solid rgba(255, 255, 255, 0.14);
    background: rgba(255, 255, 255, 0.06);
    color: #f5f5fa;
    font-size: 14px;
  }
  .settings input:focus,
  .settings select:focus {
    outline: none;
    border-color: #5b6bff;
  }
  .logo {
    height: 30px;
    width: auto;
    display: block;
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
    background: #ff5b6b;
    box-shadow: 0 0 12px #ff5b6b99;
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
