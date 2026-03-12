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
    container.innerHTML = '<div class="panel panel-tight"><p class="muted">暂无设备。可先输入机器识别码完成绑定，客户端上线后会自动回填状态。</p></div>';
    return;
  }

  container.innerHTML = state.devices.map((device) => `
    <article class="device-card">
      <div class="device-head">
        <div>
          <h3>${device.remark || device.device_id}</h3>
          <p class="muted">${device.device_id}</p>
        </div>
        <span class="chip ${device.online ? '' : 'offline'}">${device.online ? '在线' : '离线'}</span>
      </div>
      <div class="meta">
        <div><strong>Owner</strong><p>${device.owner_username || '-'}</p></div>
        <div><strong>Hostname</strong><p>${device.status?.hostname || '-'}</p></div>
        <div><strong>IP</strong><p>${device.status?.local_ip || '-'}</p></div>
        <div><strong>MAC</strong><p>${device.status?.mac || '-'}</p></div>
        <div><strong>系统</strong><p>${device.status?.system_version || '-'}</p></div>
        <div><strong>最后心跳</strong><p>${device.last_seen_at || '-'}</p></div>
      </div>
      <label class="field"> <span>备注</span>
        <input data-remark-input="${device.device_id}" value="${device.remark || ''}" placeholder="自定义备注名">
      </label>
      <label class="field"> <span>openclaw.json</span>
        <textarea data-config-input="${device.device_id}">${device.pending_openclaw_json || device.openclaw_json || ''}</textarea>
      </label>
      <div class="actions">
        <button class="button primary" data-action="save-remark" data-device-id="${device.device_id}">保存备注</button>
        <button class="button primary" data-action="save-config" data-device-id="${device.device_id}">下发配置</button>
        <button class="button danger" data-action="delete-device" data-device-id="${device.device_id}">删除设备</button>
      </div>
    </article>
  `).join('');
}

function renderUsers() {
  const container = document.getElementById('usersList');
  if (!state.me?.is_admin) {
    container.innerHTML = '';
    return;
  }
  if (!state.users.length) {
    container.innerHTML = '<div class="panel panel-tight"><p class="muted">暂无用户数据。</p></div>';
    return;
  }

  container.innerHTML = state.users.map((user) => `
    <article class="user-card">
      <div class="user-head">
        <div>
          <h3>${user.username}</h3>
          <p class="muted">${user.email}</p>
        </div>
        <span class="chip ${user.is_admin ? '' : 'offline'}">${user.is_admin ? '管理员' : '普通用户'}</span>
      </div>
      <div class="meta">
        <div><strong>设备数</strong><p>${user.device_count}</p></div>
        <div><strong>创建时间</strong><p>${user.created_at}</p></div>
        <div><strong>更新时间</strong><p>${user.updated_at}</p></div>
      </div>
      <div class="compact-grid">
        <label class="field">
          <span>邮箱</span>
          <input data-user-email="${user.id}" value="${user.email}">
        </label>
        <label class="field">
          <span>新密码</span>
          <input type="password" data-user-password="${user.id}" placeholder="留空表示不修改">
        </label>
        <label class="toggle toggle-box">
          <input type="checkbox" data-user-admin="${user.id}" ${user.is_admin ? 'checked' : ''}>
          <span>管理员</span>
        </label>
      </div>
      <div class="actions">
        <button class="button primary" data-action="save-user" data-user-id="${user.id}">保存用户</button>
        <button class="button danger" data-action="delete-user" data-user-id="${user.id}">删除用户</button>
      </div>
    </article>
  `).join('');
}

function renderSMTPForm() {
  const form = document.getElementById('smtpForm');
  if (!form) {
    return;
  }

  form.host.value = state.settings.smtp?.host || '';
  form.port.value = state.settings.smtp?.port || 25;
  form.username.value = state.settings.smtp?.username || '';
  form.password.value = state.settings.smtp?.password || '';
  form.from.value = state.settings.smtp?.from || '';
}

function renderProfileForm() {
  const form = document.getElementById('profileForm');
  if (!form || !state.me) {
    return;
  }

  const username = document.getElementById('profileUsername');
  username.value = state.me.username || '';
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

  if (!authed) {
    return;
  }

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

document.querySelectorAll('.tab').forEach((button) => {
  button.addEventListener('click', () => activateTab(button.dataset.tab));
});

document.querySelectorAll('.nav-link').forEach((button) => {
  button.addEventListener('click', () => activateView(button.dataset.view));
});

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
    renderShell();
    notify('已退出登录');
  } catch (error) {
    notify(error.message, true);
  }
});

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

document.getElementById('applyDeviceFilterButton').addEventListener('click', async () => {
  if (!state.me?.is_admin) {
    return;
  }
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
  if (!state.me?.is_admin) {
    return;
  }
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
  if (event.key !== 'Enter') {
    return;
  }
  event.preventDefault();
  document.getElementById('applyDeviceFilterButton').click();
});

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

document.addEventListener('click', async (event) => {
  const button = event.target.closest('button[data-action]');
  if (!button) {
    return;
  }

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
      const confirmed = window.confirm('确认删除该设备记录？在线客户端下次心跳后可能会重新出现。');
      if (!confirmed) {
        return;
      }
      await api(`/api/v1/devices/${encodeURIComponent(deviceId)}`, {
        method: 'DELETE',
      });
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
      await api(`/api/v1/admin/users/${encodeURIComponent(userId)}`, {
        method: 'DELETE',
        body: '{}',
      });
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
