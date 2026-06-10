// Tiny client for the loopback UI API exposed by the tray process.
// The per-launch access token is passed to the webview in the URL (?t=...).

const token = new URLSearchParams(location.search).get('t') || ''

export async function api(method, path) {
  const res = await fetch(path, {
    method,
    headers: { 'X-Claimward-Token': token },
  })
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
