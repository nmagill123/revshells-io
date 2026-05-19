window.RSD_VERSION = window.RSD_VERSION || '0.0.0';

const RSD_STORAGE_SESSIONS = 'rsd_sessions';
const RSD_STORAGE_THEME = 'rsd_theme';
const RSD_STORAGE_DISCLAIMER = 'rsd_disclaimer_accepted';
const RSD_STORAGE_OPERATOR = 'rsd_operator';

function rsdInitTopbar() {
  const ver = document.getElementById('rsd-version');
  if (ver) ver.textContent = 'v' + window.RSD_VERSION;
  const themeBtn = document.getElementById('theme-toggle');
  if (themeBtn) {
    themeBtn.addEventListener('click', rsdToggleTheme);
    themeBtn.textContent = document.documentElement.dataset.theme === 'light' ? '🌙' : '☀️';
  }
  rsdShowDisclaimerIfNeeded();
}

function rsdGetTheme() {
  return localStorage.getItem(RSD_STORAGE_THEME) || 'dark';
}

function rsdApplyTheme(theme) {
  document.documentElement.dataset.theme = theme;
  localStorage.setItem(RSD_STORAGE_THEME, theme);
  const btn = document.getElementById('theme-toggle');
  if (btn) btn.textContent = theme === 'light' ? '🌙' : '☀️';
  window.dispatchEvent(new CustomEvent('rsd-theme', { detail: theme }));
}

function rsdToggleTheme() {
  rsdApplyTheme(rsdGetTheme() === 'light' ? 'dark' : 'light');
}

function rsdShowDisclaimerIfNeeded() {
  if (localStorage.getItem(RSD_STORAGE_DISCLAIMER) === '1') return;
  const overlay = document.getElementById('disclaimer-modal');
  if (!overlay) return;
  overlay.classList.remove('hidden');
  document.getElementById('disclaimer-accept')?.addEventListener('click', () => {
    localStorage.setItem(RSD_STORAGE_DISCLAIMER, '1');
    overlay.classList.add('hidden');
  });
}

function rsdGetSessions() {
  try {
    return JSON.parse(localStorage.getItem(RSD_STORAGE_SESSIONS) || '[]');
  } catch {
    return [];
  }
}

function rsdSaveSession(entry) {
  const list = rsdGetSessions().filter(s => s.id !== entry.id);
  list.unshift(entry);
  localStorage.setItem(RSD_STORAGE_SESSIONS, JSON.stringify(list.slice(0, 50)));
}

function rsdRemoveSession(id) {
  const list = rsdGetSessions().filter(s => s.id !== id);
  localStorage.setItem(RSD_STORAGE_SESSIONS, JSON.stringify(list));
}

function rsdGetSessionToken(id) {
  const s = rsdGetSessions().find(x => x.id === id);
  return s ? s.token : null;
}

async function rsdCopyText(text, btn) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    const ta = document.createElement('textarea');
    ta.value = text;
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    document.body.removeChild(ta);
  }
  if (btn) {
    btn.classList.add('copied');
    btn.textContent = '✓';
    setTimeout(() => { btn.classList.remove('copied'); btn.textContent = '📋'; }, 1500);
  }
}

function rsdAddCopyRow(container, label, text) {
  const row = document.createElement('div');
  row.className = 'copy-row';
  const lbl = document.createElement('span');
  lbl.className = 'copy-label';
  lbl.textContent = label;
  const field = document.createElement('span');
  field.className = 'copy-field';
  field.textContent = text;
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'btn icon-btn copy-btn';
  btn.title = 'Copy';
  btn.textContent = '📋';
  btn.addEventListener('click', () => rsdCopyText(text, btn));
  row.appendChild(lbl);
  row.appendChild(field);
  row.appendChild(btn);
  container.appendChild(row);
}

function escapeHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}
window.rsdEscapeHtml = escapeHtml;

document.documentElement.dataset.theme = rsdGetTheme();
