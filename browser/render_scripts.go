package browser

// searchExtractJS runs in the rendered Google results page. It walks every
// h3 (the title element across all current desktop/mobile variants), finds
// its enclosing anchor, and pulls the surrounding container's text as the
// raw snippet material — structural anchors instead of obfuscated class
// names, so it survives Google's class-name churn better than CSS selectors.
const searchExtractJS = `(() => {
  const out = [];
  const seen = new Set();
  for (const h3 of document.querySelectorAll('h3')) {
    const a = h3.closest('a');
    if (!a) continue;
    let u = '';
    try { u = a.href || ''; } catch (e) { continue; }
    if (!u || seen.has(u)) continue;
    seen.add(u);
    const container = a.closest('div.g, div.tF2Cxc, div.MjjYud, li') || a.parentElement;
    let snippet = '';
    if (container) {
      const lines = (container.innerText || '').split('\n').map(s => s.trim()).filter(Boolean);
      for (const line of lines) {
        if (line && line !== (h3.innerText || '').trim()) { snippet += line + ' '; }
        if (snippet.length > 400) break;
      }
    }
    out.push({ title: (h3.innerText || '').trim(), url: u, snippet: snippet.trim() });
  }
  return JSON.stringify(out);
})()`

// fetchExtractJS extracts the readable main content of any page: prefer
// <article>/<main>, else the text-heaviest ancestor of the largest <p>
// cluster, else body. Returns final URL too, since redirects are common.
const fetchExtractJS = `(() => {
  let root = document.querySelector('article') || document.querySelector('main');
  if (!root) {
    let best = document.body, bestLen = 0;
    for (const p of document.querySelectorAll('p')) {
      const c = p.closest('div, section') || document.body;
      const len = (c.innerText || '').length;
      if (len > bestLen) { best = c; bestLen = len; }
    }
    root = best || document.body;
  }
  const strip = (root.querySelectorAll('script, style, nav, header, footer, aside, form, noscript'));
  const removed = [];
  for (const el of strip) { removed.push([el, el.nextSibling]); el.remove(); }
  const text = (root.innerText || '').replace(/\n{3,}/g, '\n\n').slice(0, 20000);
  for (const [el, next] of removed.reverse()) { el.parentNode && el.parentNode.insertBefore(el, next); }
  return JSON.stringify({ title: document.title || '', url: location.href, content: text });
})()`

// pageStateJS reports where the tab currently is: final URL and title.
// Used for consent detection and for enriching failure diagnostics.
const pageStateJS = `(() => JSON.stringify({
  url: location.href,
  title: document.title || ''
}))()`

// consentAcceptClickJS clicks Google's cookie-consent accept button. The
// button is matched by visible text across common locales because Google
// renders it with rotating generated classes. Returns whether a click fired.
const consentAcceptClickJS = `(() => {
  const rx = /(accept all|i agree|kabul et|hepsini kabul|alle akzeptieren|tout accepter|aceptar todo|accettare tutto|同意|принять)/i;
  const candidates = document.querySelectorAll('button, div[role="button"], form[action*="consent"] button');
  for (const el of candidates) {
    if (rx.test((el.innerText || el.textContent || '').trim())) {
      el.click();
      return true;
    }
  }
  // Last resort: any consent form's primary submit.
  const form = document.querySelector('form[action*="consent"]');
  if (form) { const b = form.querySelector('button'); if (b) { b.click(); return true; } }
  return false;
})()`
