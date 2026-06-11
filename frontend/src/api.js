// Tiny client for the loopback UI API exposed by the tray process.
// The per-launch access token is passed to the webview in the URL (?t=...).

const token = new URLSearchParams(location.search).get('t') || ''

export async function api(method, path, body) {
  const opts = { method, headers: { 'X-Claimward-Token': token } }
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  const res = await fetch(path, opts)
  let data = {}
  try {
    data = await res.json()
  } catch {
    // no body
  }
  if (!res.ok) {
    throw new Error(data.error || res.statusText || 'request failed')
  }
  return data
}
