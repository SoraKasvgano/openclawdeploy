const state = {
  me: null,
  settings: {
    registration_enabled: true,
    smtp: { host: '', port: 25, username: '', password: '', from: '' },
    api_token_header: 'X-API-Token',
    swagger_url: '/swagger/',
  },
  devices: [],
  users: [],
  summary: { user_count: 0, device_count: 0, online_device_count: 0 },
  activeView: 'overview',
  deviceOwnerFilter: '',
  expandedDevices: new Set(),
  expandedUsers: new Set(),
};

const toast = document.getElementById('toast');

function notify(message, isError = false) {
  toast.textContent = message;
  toast.style.background = isError ? 'rgba(221, 81, 69, 0.92)' : 'rgba(22, 34, 52, 0.92)';
  toast.classList.remove('hidden');
  window.clearTimeout(notify.timer);
  notify.timer = window.setTimeout(() => toast.classList.add('hidden'), 2600);
}

async function api(path, options = {}) {
  const response = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.error || '请求失败');
  }
  return payload;
}

function escapeHTML(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

function activateTab(name) {
  document.querySelectorAll('.tab').forEach((tab) => {
    tab.classList.toggle('active', tab.dataset.tab === name);
  });
  document.querySelectorAll('.form-pane').forEach((pane) => {
    pane.classList.toggle('active', pane.id === `${name}Form`);
  });
}

function activateView(name) {
  state.activeView = name;
  document.querySelectorAll('.nav-link').forEach((button) => {
    button.classList.toggle('active', button.dataset.view === name);
  });
  document.querySelectorAll('.view').forEach((view) => {
    view.classList.toggle('active', view.id === `${name}View`);
  });
}

function renderDevices() {
  const container = document.getElementById('devicesList');
  if (!state.devices.length) {
    container.innerHTML = '<p class="empty-state">暂无设备。输入机器识别码完成绑定，客户端上线后自动回填状态。</p>';
    return;
  }

  const isAdmin = state.me?.is_admin;
  let html = '<table class="device-table"><thead><tr>';
  html += '<th>状态</th><th>备注 / 名称</th><th>识别码</th>';
  if (isAdmin) html += '<th>归属</th>';
  html += '<th>Hostname</th><th>IP</th><th>最后心跳</th>';
  html += '</tr></thead><tbody>';

  state.devices.forEach((device) => {
    const expanded = state.expandedDevices.has(device.device_id);
    const statusChip = device.online
      ? '<span class="chip">在线</span>'
      : '<span class="chip offline">离线</span>';
    const displayName = escapeHTML(device.remark || device.device_id.substring(0, 12) + '…');

    html += `<tr class="${expanded ? 'expanded' : ''}" data-toggle-device="${escapeHTML(device.device_id)}">`;
    html += `<td>${statusChip}</td>`;
    html += `<td>${displayName}</td>`;
    html += `<td class="device-id-cell">${escapeHTML(device.device_id)}</td>`;
    if (isAdmin) html += `<td>${escapeHTML(device.owner_username || '-')}</td>`;
    html += `<td>${escapeHTML(device.status?.hostname || '-')}</td>`;
    html += `<td>${escapeHTML(device.status?.local_ip || '-')}</td>`;
    html += `<td>${escapeHTML(device.last_seen_at || '-')}</td>`;
    html += '</tr>';

    if (expanded) {
      const colSpan = isAdmin ? 7 : 6;
      const configVal = escapeHTML(device.pending_openclaw_json || device.openclaw_json || '');
      html += `<tr class="device-expand-row"><td colspan="${colSpan}">`;
      html += '<div class="device-expand-content">';

      html += '<div class="device-meta-row">';
      html += `<span>MAC: <strong>${escapeHTML(device.status?.mac || '-')}</strong></span>`;
      html += `<span>系统: <strong>${escapeHTML(device.status?.system_version || '-')}</strong></span>`;
      if (isAdmin) html += `<span>归属: <strong>${escapeHTML(device.owner_username || '-')}</strong></span>`;
      html += '</div>';

      html += `<label class="field"><span>备注</span><input data-remark-input="${escapeHTML(device.device_id)}" value="${escapeHTML(device.remark || '')}" placeholder="自定义备注名"></label>`;
      html += `<label class="field"><span>openclaw.json</span><textarea data-config-input="${escapeHTML(device.device_id)}">${configVal}</textarea></label>`;

      html += '<div class="form-actions" style="grid-column:1/-1;">';
      html += `<button class="button primary sm" data-action="save-remark" data-device-id="${escapeHTML(device.device_id)}">保存备注</button>`;
      html += `<button class="button primary sm" data-action="save-config" data-device-id="${escapeHTML(device.device_id)}">下发配置</button>`;
      html += `<button class="button danger sm" data-action="delete-device" data-device-id="${escapeHTML(device.device_id)}">删除设备</button>`;
      html += '</div>';

      html += '</div></td></tr>';
    }
  });

  html += '</tbody></table>';
  container.innerHTML = html;
}

function renderUsers() {
  const container = document.getElementById('usersList');
  if (!state.me?.is_admin) {
    container.innerHTML = '';
    return;
  }
  if (!state.users.length) {
    container.innerHTML = '<p class="empty-state">暂无用户数据。</p>';
    return;
  }

  let html = '<table class="user-table"><thead><tr>';
  html += '<th>用户名</th><th>邮箱</th><th>角色</th><th>设备数</th><th>创建时间</th>';
  html += '</tr></thead><tbody>';

  state.users.forEach((user) => {
    const expanded = state.expandedUsers.has(String(user.id));
    const roleChip = user.is_admin
      ? '<span class="chip admin-chip">管理员</span>'
      : '<span class="chip user-chip">用户</span>';

    html += `<tr class="${expanded ? 'expanded' : ''}" data-toggle-user="${user.id}">`;
    html += `<td><strong>${escapeHTML(user.username)}</strong></td>`;
    html += `<td>${escapeHTML(user.email)}</td>`;
    html += `<td>${roleChip}</td>`;
    html += `<td>${user.device_count}</td>`;
    html += `<td>${escapeHTML(user.created_at)}</td>`;
    html += '</tr>';

    if (expanded) {
      html += '<tr class="user-expand-row"><td colspan="5">';
      html += '<div class="user-expand-content">';
      html += `<label class="field"><span>邮箱</span><input data-user-email="${user.id}" value="${escapeHTML(user.email)}"></label>`;
      html += `<label class="field"><span>新密码</span><input type="password" data-user-password="${user.id}" placeholder="留空不修改"></label>`;
      html += '<label class="toggle toggle-box">';
      html += `<input type="checkbox" data-user-admin="${user.id}" ${user.is_admin ? 'checked' : ''}>`;
      html += '<span>管理员</span></label>';
      html += '<div class="form-actions" style="grid-column:1/-1;">';
      html += `<button class="button primary sm" data-action="save-user" data-user-id="${user.id}">保存</button>`;
      html += `<button class="button danger sm" data-action="delete-user" data-user-id="${user.id}">删除用户</button>`;
      html += '</div>';
      html += '</div></td></tr>';
    }
  });

  html += '</tbody></table>';
  container.innerHTML = html;
}

function renderSMTPForm() {
  const form = document.getElementById('smtpForm');
  if (!form) return;
  form.host.value = state.settings.smtp?.host || '';
  form.port.value = state.settings.smtp?.port || 25;
  form.username.value = state.settings.smtp?.username || '';
  form.password.value = state.settings.smtp?.password || '';
  form.from.value = state.settings.smtp?.from || '';
}

function renderProfileForm() {
  const form = document.getElementById('profileForm');
  if (!form || !state.me) return;
  document.getElementById('profileUsername').value = state.me.username || '';
  form.email.value = state.me.email || '';
  form.password.value = '';
}

function renderShell() {
  const authed = Boolean(state.me);
  document.getElementById('authShell').classList.toggle('hidden', authed);
  document.getElementById('dashboard').classList.toggle('hidden', !authed);

  document.getElementById('registerHint').textContent = state.settings.registration_enabled
    ? '当前允许用户自行注册。'
    : '当前已关闭用户注册。';

  if (!authed) return;

  if (!state.me.is_admin && state.activeView === 'admin') {
    state.activeView = 'overview';
  }

  document.getElementById('userTitle').textContent = state.me.username;
  document.getElementById('userMeta').textContent = state.me.is_admin ? '管理员会话' : '普通用户会话';
  document.querySelectorAll('.admin-only').forEach((node) => {
    node.classList.toggle('hidden', !state.me.is_admin);
  });

  document.getElementById('statUsers').textContent = state.summary.user_count ?? 0;
  document.getElementById('statDevices').textContent = state.summary.device_count ?? state.devices.length;
  document.getElementById('statOnline').textContent = state.summary.online_device_count ?? 0;
  document.getElementById('registrationToggle').checked = Boolean(state.settings.registration_enabled);
  document.getElementById('apiHeaderValue').textContent = state.settings.api_token_header || 'X-API-Token';
  document.getElementById('swaggerValue').textContent = state.settings.swagger_url || '/swagger/';
  document.getElementById('swaggerLink').href = state.settings.swagger_url || '/swagger/';
  document.getElementById('deviceOwnerFilter').value = state.deviceOwnerFilter;

  renderProfileForm();
  renderSMTPForm();
  renderDevices();
  renderUsers();
  activateView(state.activeView);
}

async function loadPublicSettings() {
  const payload = await api('/api/v1/settings/public');
  state.settings.registration_enabled = Boolean(payload.registration_enabled);
  state.settings.api_token_header = payload.api_token_header || state.settings.api_token_header;
  state.settings.swagger_url = payload.swagger_url || state.settings.swagger_url;
}

async function loadAdminSettings() {
  const payload = await api('/api/v1/admin/settings');
  state.settings.registration_enabled = Boolean(payload.registration_enabled);
  state.settings.smtp = payload.smtp || state.settings.smtp;
  state.settings.api_token_header = payload.api_token_header || state.settings.api_token_header;
  state.settings.swagger_url = payload.swagger_url || state.settings.swagger_url;
}

async function loadSession() {
  try {
    const payload = await api('/api/v1/auth/me');
    state.me = payload.user;
  } catch {
    state.me = null;
  }
}

async function loadDashboard() {
  const deviceQuery = new URLSearchParams();
  if (state.me?.is_admin && state.deviceOwnerFilter.trim() !== '') {
    deviceQuery.set('owner_username', state.deviceOwnerFilter.trim());
  }
  const devicesPath = deviceQuery.size ? `/api/v1/devices?${deviceQuery.toString()}` : '/api/v1/devices';
  const devicesPayload = await api(devicesPath);
  state.devices = devicesPayload.devices || [];

  if (state.me?.is_admin) {
    state.summary = await api('/api/v1/admin/summary');
    const usersPayload = await api('/api/v1/admin/users');
    state.users = usersPayload.users || [];
    await loadAdminSettings();
  } else {
    state.summary = {
      user_count: 1,
      device_count: state.devices.length,
      online_device_count: state.devices.filter((device) => device.online).length,
    };
    state.users = [];
  }
}

async function bootstrap() {
  await loadPublicSettings();
  await loadSession();
  if (state.me) {
    await loadDashboard();
  }
  renderShell();
}

/* ── Tab / Nav events ── */
document.querySelectorAll('.tab').forEach((button) => {
  button.addEventListener('click', () => activateTab(button.dataset.tab));
});

document.querySelectorAll('.nav-link').forEach((button) => {
  button.addEventListener('click', () => activateView(button.dataset.view));
});

/* ── Auth events ── */
document.getElementById('loginForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  try {
    await api('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify(Object.fromEntries(form)),
    });
    await bootstrap();
    notify('登录成功');
  } catch (error) {
    notify(error.message, true);
  }
});

document.getElementById('registerForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  try {
    await api('/api/v1/auth/register', {
      method: 'POST',
      body: JSON.stringify(Object.fromEntries(form)),
    });
    notify('注册成功，请登录');
    activateTab('login');
  } catch (error) {
    notify(error.message, true);
  }
});

document.getElementById('forgotForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  try {
    await api('/api/v1/auth/forgot-password', {
      method: 'POST',
      body: JSON.stringify(Object.fromEntries(form)),
    });
    notify('若账号存在，系统已发送重置链接');
  } catch (error) {
    notify(error.message, true);
  }
});

document.getElementById('resetForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  try {
    await api('/api/v1/auth/reset-password', {
      method: 'POST',
      body: JSON.stringify(Object.fromEntries(form)),
    });
    notify('密码已更新，请重新登录');
    activateTab('login');
  } catch (error) {
    notify(error.message, true);
  }
});

document.getElementById('logoutButton').addEventListener('click', async () => {
  try {
    await api('/api/v1/auth/logout', { method: 'POST', body: '{}' });
    state.me = null;
    state.devices = [];
    state.users = [];
    state.deviceOwnerFilter = '';
    state.expandedDevices.clear();
    state.expandedUsers.clear();
    renderShell();
    notify('已退出登录');
  } catch (error) {
    notify(error.message, true);
  }
});

/* ── Profile ── */
document.getElementById('profileForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  const payload = Object.fromEntries(form);
  try {
    const response = await api('/api/v1/auth/profile', {
      method: 'PUT',
      body: JSON.stringify(payload),
    });
    state.me = response.user || state.me;
    renderShell();
    notify('账户信息已更新');
  } catch (error) {
    notify(error.message, true);
  }
});

/* ── Device binding ── */
document.getElementById('bindButton').addEventListener('click', async () => {
  const input = document.getElementById('bindDeviceId');
  try {
    await api('/api/v1/devices/bind', {
      method: 'POST',
      body: JSON.stringify({ device_id: input.value }),
    });
    input.value = '';
    await loadDashboard();
    renderShell();
    notify('设备绑定成功');
  } catch (error) {
    notify(error.message, true);
  }
});

/* ── Device filter ── */
document.getElementById('applyDeviceFilterButton').addEventListener('click', async () => {
  if (!state.me?.is_admin) return;
  state.deviceOwnerFilter = document.getElementById('deviceOwnerFilter').value.trim();
  try {
    await loadDashboard();
    renderShell();
    notify(state.deviceOwnerFilter ? '筛选已应用' : '已显示全部设备');
  } catch (error) {
    notify(error.message, true);
  }
});

document.getElementById('clearDeviceFilterButton').addEventListener('click', async () => {
  if (!state.me?.is_admin) return;
  state.deviceOwnerFilter = '';
  document.getElementById('deviceOwnerFilter').value = '';
  try {
    await loadDashboard();
    renderShell();
    notify('已清空筛选');
  } catch (error) {
    notify(error.message, true);
  }
});

document.getElementById('deviceOwnerFilter').addEventListener('keydown', async (event) => {
  if (event.key !== 'Enter') return;
  event.preventDefault();
  document.getElementById('applyDeviceFilterButton').click();
});

/* ── Admin: create user ── */
document.getElementById('createUserForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  const payload = Object.fromEntries(form);
  payload.is_admin = form.get('is_admin') === 'on';
  try {
    await api('/api/v1/admin/users', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
    event.currentTarget.reset();
    await loadDashboard();
    renderShell();
    notify('用户已创建');
  } catch (error) {
    notify(error.message, true);
  }
});

/* ── Admin: SMTP ── */
document.getElementById('smtpForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  const payload = Object.fromEntries(form);
  payload.port = Number(payload.port) || 25;
  try {
    const response = await api('/api/v1/admin/settings/smtp', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
    state.settings.smtp = response.smtp || state.settings.smtp;
    renderShell();
    notify('SMTP 配置已保存');
  } catch (error) {
    notify(error.message, true);
  }
});

/* ── Admin: registration toggle ── */
document.getElementById('registrationToggle').addEventListener('change', async (event) => {
  try {
    await api('/api/v1/admin/settings/registration', {
      method: 'POST',
      body: JSON.stringify({ enabled: event.target.checked }),
    });
    state.settings.registration_enabled = event.target.checked;
    renderShell();
    notify('注册开关已更新');
  } catch (error) {
    notify(error.message, true);
  }
});

/* ── Delegated click handler for table rows & actions ── */
document.addEventListener('click', async (event) => {
  /* ─ Toggle device expand ─ */
  const deviceRow = event.target.closest('tr[data-toggle-device]');
  if (deviceRow && !event.target.closest('button, input, textarea, label')) {
    const id = deviceRow.dataset.toggleDevice;
    if (state.expandedDevices.has(id)) {
      state.expandedDevices.delete(id);
    } else {
      state.expandedDevices.add(id);
    }
    renderDevices();
    return;
  }

  /* ─ Toggle user expand ─ */
  const userRow = event.target.closest('tr[data-toggle-user]');
  if (userRow && !event.target.closest('button, input, textarea, label')) {
    const id = String(userRow.dataset.toggleUser);
    if (state.expandedUsers.has(id)) {
      state.expandedUsers.delete(id);
    } else {
      state.expandedUsers.add(id);
    }
    renderUsers();
    return;
  }

  /* ─ Action buttons ─ */
  const button = event.target.closest('button[data-action]');
  if (!button) return;

  try {
    if (button.dataset.action === 'save-remark') {
      const deviceId = button.dataset.deviceId;
      const remark = document.querySelector(`[data-remark-input="${deviceId}"]`).value;
      await api(`/api/v1/devices/${encodeURIComponent(deviceId)}/remark`, {
        method: 'PUT',
        body: JSON.stringify({ remark }),
      });
      notify('备注已更新');
    }

    if (button.dataset.action === 'save-config') {
      const deviceId = button.dataset.deviceId;
      const openclawJSON = document.querySelector(`[data-config-input="${deviceId}"]`).value;
      await api(`/api/v1/devices/${encodeURIComponent(deviceId)}/config`, {
        method: 'PUT',
        body: JSON.stringify({ openclaw_json: openclawJSON }),
      });
      notify('配置已下发，等待客户端同步');
    }

    if (button.dataset.action === 'delete-device') {
      const deviceId = button.dataset.deviceId;
      if (!window.confirm('确认删除该设备记录？')) return;
      await api(`/api/v1/devices/${encodeURIComponent(deviceId)}`, { method: 'DELETE' });
      state.expandedDevices.delete(deviceId);
      notify('设备已删除');
    }

    if (button.dataset.action === 'save-user') {
      const userId = button.dataset.userId;
      const email = document.querySelector(`[data-user-email="${userId}"]`).value;
      const password = document.querySelector(`[data-user-password="${userId}"]`).value;
      const isAdmin = document.querySelector(`[data-user-admin="${userId}"]`).checked;
      await api(`/api/v1/admin/users/${encodeURIComponent(userId)}`, {
        method: 'PUT',
        body: JSON.stringify({ email, password, is_admin: isAdmin }),
      });
      notify('用户信息已更新');
    }

    if (button.dataset.action === 'delete-user') {
      const userId = button.dataset.userId;
      if (!window.confirm('确认删除该用户？')) return;
      await api(`/api/v1/admin/users/${encodeURIComponent(userId)}`, {
        method: 'DELETE',
        body: '{}',
      });
      state.expandedUsers.delete(String(userId));
      notify('用户已删除');
    }

    await loadDashboard();
    renderShell();
  } catch (error) {
    notify(error.message, true);
  }
});

bootstrap().catch((error) => notify(error.message, true));

const resetToken = new URLSearchParams(window.location.search).get('reset_token');
if (resetToken) {
  activateTab('reset');
  document.querySelector('#resetForm [name="token"]').value = resetToken;
}
