const state = {
  me: null,
  status: null,
  pathBrowser: null,
};

const toast = document.getElementById('toast');

function notify(message, isError = false) {
  toast.textContent = message;
  toast.style.background = isError ? 'rgba(223, 75, 63, 0.92)' : 'rgba(19, 34, 56, 0.92)';
  toast.classList.remove('hidden');
  window.clearTimeout(notify.timer);
  notify.timer = window.setTimeout(() => toast.classList.add('hidden'), 2600);
}

function handleUnauthorized() {
  state.me = null;
  state.status = null;
  closePathBrowser();
  renderShell();
}

async function api(path, options = {}, extra = {}) {
  const response = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    if (response.status === 401 && !extra.allowUnauthorized) {
      handleUnauthorized();
    }
    const error = new Error(payload.error || '请求失败');
    error.status = response.status;
    throw error;
  }
  return payload;
}

function renderConnection(sync) {
  const configured = Boolean(sync?.server_configured);
  const connected = Boolean(sync?.connected);

  let text = '服务端未配置';
  let badgeMode = 'neutral';
  if (configured) {
    text = connected ? '已连接服务端' : '服务端未连通';
    badgeMode = connected ? 'online' : 'offline';
  }

  const serverBadge = document.getElementById('serverCommBadge');
  serverBadge.textContent = text;
  serverBadge.classList.remove('neutral', 'offline', 'online');
  serverBadge.classList.add(badgeMode);

  const commValue = document.getElementById('commStatusValue');
  const message = sync?.last_sync_message || '-';
  commValue.textContent = configured ? `${text} / ${message}` : text;
  document.getElementById('lastSyncAtValue').textContent = sync?.last_sync_at || '-';
}

function renderStatus(status) {
  state.status = status;
  document.getElementById('deviceId').textContent = status.device_id || '-';
  document.getElementById('hostnameValue').textContent = status.identity?.hostname || '-';
  document.getElementById('ipValue').textContent = status.identity?.local_ip || '-';
  document.getElementById('macValue').textContent = status.identity?.mac || '-';
  document.getElementById('serverValue').textContent = status.config?.server_url || '未配置';
  document.getElementById('listenValue').textContent = status.listen_address || '-';
  document.getElementById('configPathValue').textContent = status.config_path || '-';
  document.getElementById('openclawPathValue').textContent = status.openclaw_path || '-';
  document.getElementById('openclawDefaultPath').textContent = status.config?.default_openclaw_path || '-';
  document.getElementById('intervalValue').textContent = `${status.config?.sync_interval_seconds || 0} 秒`;
  document.getElementById('syncStatus').textContent = status.sync?.last_sync_message || '尚未同步';
  document.getElementById('configEditor').value = status.openclaw_json || '';
  document.getElementById('qrPreview').innerHTML = status.identity_qr_svg || status.identity_matrix_svg || '';
  document.getElementById('serverForm').server_url.value = status.config?.server_url || '';
  document.getElementById('openclawPathForm').openclaw_config_path.value = status.config?.openclaw_config_path || '';

  const networkBadge = document.getElementById('networkBadge');
  const networkOK = Boolean(status.identity?.network_ok);
  networkBadge.textContent = networkOK ? '网络正常' : '网络异常';
  networkBadge.classList.remove('neutral', 'offline', 'online');
  networkBadge.classList.add(networkOK ? 'online' : 'offline');

  renderConnection(status.sync);
}

function renderShell() {
  const authed = Boolean(state.me);
  document.getElementById('authShell').classList.toggle('hidden', authed);
  document.getElementById('dashboardShell').classList.toggle('hidden', !authed);

  if (!authed) {
    document.getElementById('loginForm').password.value = 'admin';
    document.getElementById('configEditor').value = '';
    return;
  }

  const accountForm = document.getElementById('accountForm');
  accountForm.username.value = state.me.username || '';
  accountForm.password.value = '';

  if (state.status) {
    renderStatus(state.status);
  }
}

function closePathBrowser() {
  state.pathBrowser = null;
  document.getElementById('pathBrowserModal').classList.add('hidden');
}

function selectOpenClawPath(path, message) {
  document.getElementById('openclawPathForm').openclaw_config_path.value = path || '';
  closePathBrowser();
  if (message) {
    notify(message);
  }
}

function renderPathBrowser() {
  if (!state.pathBrowser) {
    return;
  }

  const currentPath = document.getElementById('pathBrowserCurrent');
  const roots = document.getElementById('pathBrowserRoots');
  const entries = document.getElementById('pathBrowserEntries');
  const upButton = document.getElementById('pathBrowserUpButton');
  const useCurrentButton = document.getElementById('pathBrowserUseCurrentButton');

  currentPath.textContent = state.pathBrowser.current_path || '-';
  upButton.disabled = !state.pathBrowser.can_go_up;
  useCurrentButton.disabled = !state.pathBrowser.suggested_file_path;

  roots.innerHTML = '';
  (state.pathBrowser.roots || []).forEach((entry) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'button secondary';
    button.textContent = entry.name;
    button.addEventListener('click', () => {
      browseLocalPath(entry.path).catch((error) => notify(error.message, true));
    });
    roots.appendChild(button);
  });

  entries.innerHTML = '';
  if (!state.pathBrowser.entries || state.pathBrowser.entries.length === 0) {
    const empty = document.createElement('div');
    empty.className = 'browser-empty';
    empty.textContent = '当前目录没有子目录或 JSON 文件。';
    entries.appendChild(empty);
    return;
  }

  state.pathBrowser.entries.forEach((entry) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = `browser-item ${entry.is_dir ? 'dir' : 'file'}`;
    button.textContent = entry.is_dir ? `[目录] ${entry.name}` : entry.name;
    button.addEventListener('click', () => {
      if (entry.is_dir) {
        browseLocalPath(entry.path).catch((error) => notify(error.message, true));
        return;
      }
      selectOpenClawPath(entry.path, '路径已回填，点击“保存路径”即可热生效');
    });
    entries.appendChild(button);
  });
}

async function browseLocalPath(path = '') {
  const search = new URLSearchParams();
  if (path) {
    search.set('path', path);
  }
  const query = search.toString();
  const payload = await api(query ? `/api/v1/client/fs/browse?${query}` : '/api/v1/client/fs/browse');
  state.pathBrowser = payload;
  renderPathBrowser();
  document.getElementById('pathBrowserModal').classList.remove('hidden');
}

async function loadMe() {
  const payload = await api('/api/v1/client/auth/me', {}, { allowUnauthorized: true });
  state.me = { username: payload.username || '' };
}

async function loadStatus() {
  const status = await api('/api/v1/client/status');
  renderStatus(status);
}

async function saveConfig() {
  const openclawJSON = document.getElementById('configEditor').value;
  await api('/api/v1/client/openclaw', {
    method: 'POST',
    body: JSON.stringify({ openclaw_json: openclawJSON }),
  });
  notify('配置已保存并尝试重启 OpenClaw 网关');
  await loadStatus();
}

async function syncNow() {
  const payload = await api('/api/v1/client/sync', { method: 'POST', body: '{}' });
  notify(payload.message || '同步完成');
  await loadStatus();
}

async function saveOpenClawPath(pathValue, message) {
  await api('/api/v1/client/openclaw/path', {
    method: 'POST',
    body: JSON.stringify({ openclaw_config_path: pathValue || '' }),
  });
  await loadStatus();
  notify(message || 'OpenClaw 路径已保存并热生效');
}

async function bootstrap() {
  try {
    await loadMe();
    renderShell();
    await loadStatus();
  } catch (error) {
    if (error.status !== 401) {
      notify(error.message, true);
    }
    renderShell();
  }
}

document.getElementById('loginForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  try {
    const payload = await api('/api/v1/client/auth/login', {
      method: 'POST',
      body: JSON.stringify(Object.fromEntries(form)),
    }, { allowUnauthorized: true });
    state.me = { username: payload.username || '' };
    renderShell();
    await loadStatus();
    notify('登录成功');
  } catch (error) {
    notify(error.message, true);
  }
});

document.getElementById('accountForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  try {
    const payload = await api('/api/v1/client/auth/account', {
      method: 'POST',
      body: JSON.stringify(Object.fromEntries(form)),
    });
    state.me = { username: payload.username || '' };
    renderShell();
    notify('本地访问账户已更新');
  } catch (error) {
    notify(error.message, true);
  }
});

document.getElementById('serverForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  try {
    await api('/api/v1/client/server', {
      method: 'POST',
      body: JSON.stringify(Object.fromEntries(form)),
    });
    await loadStatus();
    window.setTimeout(() => {
      if (state.me) {
        loadStatus().catch(() => {});
      }
    }, 1000);
    notify('服务端地址已保存并热生效');
  } catch (error) {
    notify(error.message, true);
  }
});

document.getElementById('openclawPathForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  try {
    await saveOpenClawPath(form.get('openclaw_config_path') || '', 'OpenClaw 路径已保存并热生效');
  } catch (error) {
    notify(error.message, true);
  }
});

document.getElementById('openclawPathDefaultButton').addEventListener('click', () => {
  saveOpenClawPath('', '已恢复默认 OpenClaw 路径').catch((error) => notify(error.message, true));
});

document.getElementById('openclawPathBrowseButton').addEventListener('click', () => {
  const current = document.getElementById('openclawPathForm').openclaw_config_path.value || state.status?.config?.openclaw_config_path || '';
  browseLocalPath(current).catch((error) => notify(error.message, true));
});

document.getElementById('pathBrowserCloseButton').addEventListener('click', closePathBrowser);

document.getElementById('pathBrowserModal').addEventListener('click', (event) => {
  if (event.target === event.currentTarget) {
    closePathBrowser();
  }
});

document.getElementById('pathBrowserUpButton').addEventListener('click', () => {
  if (!state.pathBrowser?.can_go_up) {
    return;
  }
  browseLocalPath(state.pathBrowser.parent_path).catch((error) => notify(error.message, true));
});

document.getElementById('pathBrowserUseCurrentButton').addEventListener('click', () => {
  if (!state.pathBrowser?.suggested_file_path) {
    return;
  }
  selectOpenClawPath(state.pathBrowser.suggested_file_path, '路径已回填，点击“保存路径”即可热生效');
});

document.getElementById('logoutButton').addEventListener('click', async () => {
  try {
    await api('/api/v1/client/auth/logout', { method: 'POST', body: '{}' });
  } catch (_) {
  }
  handleUnauthorized();
  notify('已退出登录');
});

document.getElementById('saveButton').addEventListener('click', () => {
  saveConfig().catch((error) => notify(error.message, true));
});

document.getElementById('syncButton').addEventListener('click', () => {
  syncNow().catch((error) => notify(error.message, true));
});

bootstrap().catch((error) => notify(error.message, true));
window.setInterval(() => {
  if (state.me) {
    loadStatus().catch(() => {});
  }
}, 10000);
