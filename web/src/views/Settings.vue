<template>
  <!-- 系统设置页 -->
  <div class="settings-page">
    <h2 style="margin-bottom: 20px; font-size: 20px;">系统设置</h2>

    <el-tabs v-model="activeTab">
      <!-- 主题设置 -->
      <el-tab-pane label="主题风格" name="theme">
        <div class="wk-card-solid" style="padding: 20px;">
          <el-form label-position="top">
            <el-form-item label="主题预设">
              <el-radio-group v-model="themeForm.preset">
                <el-radio value="dark">暗黑科技</el-radio>
                <el-radio value="light">浅色简洁</el-radio>
              </el-radio-group>
            </el-form-item>
            <el-form-item label="主色调">
              <el-color-picker v-model="themeForm.primary" />
            </el-form-item>
            <el-form-item label="站点标题">
              <el-input v-model="themeForm.title" placeholder="wukong 监控" />
            </el-form-item>
            <el-form-item label="页脚文本">
              <el-input v-model="themeForm.footer_text" placeholder="Powered by wukong" />
            </el-form-item>
            <el-form-item label="站点域名 / 访问地址">
              <el-input v-model="themeForm.site_domain" placeholder="https://monitor.example.com 或 http://127.0.0.1:64443" />
              <div class="form-tip">用于生成安装脚本下载地址；未配置时不能复制安装命令。</div>
            </el-form-item>
            <el-form-item label="探针 gRPC 地址">
              <el-input v-model="themeForm.agent_server_addr" placeholder="monitor.example.com:443" />
              <div class="form-tip">探针实际注册和上报地址，必须是 host:port。生产环境按你的要求使用 server.lkz.pub:443。</div>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="saving" @click="saveTheme">
                保存主题与站点地址
              </el-button>
            </el-form-item>
          </el-form>
        </div>
      </el-tab-pane>

      <!-- 安装新节点 -->
      <el-tab-pane label="安装节点" name="install">
        <div class="wk-card-solid" style="padding: 20px;">
          <el-alert
            v-if="!themeForm.site_domain"
            title="请先在“主题风格”里配置站点域名 / 访问地址，否则不能复制安装命令。"
            type="warning"
            :closable="false"
            style="margin-bottom: 16px;"
          />
          <el-alert
            title="安装脚本会自动识别 amd64 / arm64 节点架构，注册后由 systemd 后台常驻运行并设置开机自启。"
            type="info"
            :closable="false"
            style="margin-bottom: 16px;"
          />
          <el-button type="primary" :loading="generating" @click="generateToken">
            生成安装命令
          </el-button>
          <div v-if="installMessage" class="install-message">
            {{ installMessage }}
          </div>
          <div v-if="installCommand" style="margin-top: 16px;">
            <div class="command-box">
              <code>{{ installCommand }}</code>
            </div>
            <el-button
              size="small"
              style="margin-top: 8px;"
              :disabled="!installReady"
              @click="copyCommand"
            >
              复制命令
            </el-button>
          </div>
        </div>
      </el-tab-pane>

      <!-- 修改密码 -->
      <el-tab-pane label="修改密码" name="security">
        <div class="wk-card-solid" style="padding: 20px;">
          <el-alert
            title="密码修改后会写入 SQLite 数据库固化，重启主控或容器后仍使用新密码。"
            type="info"
            :closable="false"
            style="margin-bottom: 16px;"
          />
          <el-form label-position="top">
            <el-form-item label="当前密码">
              <el-input v-model="passwordForm.old_password" type="password" autocomplete="current-password" show-password placeholder="输入当前管理员密码" />
            </el-form-item>
            <el-form-item label="新密码">
              <el-input v-model="passwordForm.new_password" type="password" autocomplete="new-password" show-password placeholder="至少 8 位" />
            </el-form-item>
            <el-form-item label="确认新密码">
              <el-input v-model="passwordForm.confirm_password" type="password" autocomplete="new-password" show-password placeholder="再次输入新密码" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="passwordSaving" @click="changePassword">
                修改并固化密码
              </el-button>
            </el-form-item>
          </el-form>
        </div>
      </el-tab-pane>

      <!-- Telegram 配置 -->
      <el-tab-pane label="Telegram 通知" name="telegram">
        <div class="wk-card-solid" style="padding: 20px;">
          <el-alert
            v-if="tgForm.has_bot_token"
            title="已保存 Bot Token。为避免浏览器密码管理器误填或泄露，页面不会回显 token；留空保存表示保留原 token。"
            type="success"
            :closable="false"
            style="margin-bottom: 16px;"
          />
          <el-form label-position="top" autocomplete="off">
            <el-form-item label="Bot Token">
              <el-input
                v-model="tgForm.bot_token"
                name="wukong-telegram-bot-token"
                autocomplete="off"
                placeholder="输入新的 Telegram Bot Token；留空则保留已保存 token"
              />
            </el-form-item>
            <el-form-item label="Chat ID">
              <el-input
                v-model="tgForm.chat_id"
                name="wukong-telegram-chat-id"
                autocomplete="off"
                placeholder="输入 Chat ID"
              />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="telegramSaving" @click="saveTelegram">保存</el-button>
              <el-button :loading="telegramTesting" @click="testTelegram">发送测试通知</el-button>
            </el-form-item>
          </el-form>
        </div>
      </el-tab-pane>

      <!-- 告警阈值 -->
      <el-tab-pane label="告警阈值" name="thresholds">
        <div class="wk-card-solid" style="padding: 20px;">
          <el-form label-position="top">
            <el-form-item label="离线报警阈值（秒）">
              <el-input-number v-model="thresholds.offline_seconds" :min="5" :max="3600" :step="5" />
              <div class="form-tip">节点最后心跳超过该秒数后触发离线报警，默认使用主控心跳超时配置。</div>
            </el-form-item>
            <el-form-item label="资源告警持续时间（秒）">
              <el-input-number v-model="thresholds.metric_duration_seconds" :min="1" :max="3600" :step="5" />
            </el-form-item>
            <el-form-item label="CPU 告警阈值 (%)">
              <el-slider v-model="thresholds.cpu" :min="1" :max="100" show-input />
            </el-form-item>
            <el-form-item label="内存告警阈值 (%)">
              <el-slider v-model="thresholds.mem" :min="1" :max="100" show-input />
            </el-form-item>
            <el-form-item label="磁盘告警阈值 (%)">
              <el-slider v-model="thresholds.disk" :min="1" :max="100" show-input />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="thresholdSaving" @click="saveThresholds">保存阈值</el-button>
            </el-form-item>
          </el-form>
        </div>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import axios from 'axios'

const activeTab = ref('theme')

// 主题
const themeForm = reactive({
  preset: 'dark',
  primary: '#38bdf8',
  title: 'wukong 监控',
  footer_text: 'Powered by wukong',
  site_domain: '',
  agent_server_addr: '',
})
const saving = ref(false)

// 安装
const generating = ref(false)
const installCommand = ref('')
const installReady = ref(false)
const installMessage = ref('')

// 修改密码
const passwordForm = reactive({
  old_password: '',
  new_password: '',
  confirm_password: '',
})
const passwordSaving = ref(false)

// Telegram
const tgForm = reactive({
  bot_token: '',
  chat_id: '',
  has_bot_token: false,
})
const telegramSaving = ref(false)
const telegramTesting = ref(false)

// 阈值
const thresholds = reactive({
  cpu: 90,
  mem: 90,
  disk: 90,
  offline_seconds: 30,
  metric_duration_seconds: 60,
})
const thresholdSaving = ref(false)

function authHeaders() {
  const token = localStorage.getItem('access_token')
  return { Authorization: `Bearer ${token}` }
}

async function saveTheme() {
  saving.value = true
  try {
    await axios.put('/api/theme', themeForm, { headers: authHeaders() })
    applyTheme()
    ElMessage.success('主题和站点地址已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

function applyTheme() {
  document.documentElement.dataset.theme = themeForm.preset
  document.documentElement.classList.toggle('dark', themeForm.preset === 'dark')
  localStorage.setItem('theme', themeForm.preset)
  document.title = themeForm.title
}

async function generateToken() {
  generating.value = true
  installCommand.value = ''
  installReady.value = false
  installMessage.value = ''
  try {
    const res = await axios.post('/api/install-tokens', {}, { headers: authHeaders() })
    installReady.value = Boolean(res.data.ready)
    installCommand.value = res.data.script_url || ''
    installMessage.value = res.data.message || ''
    if (!installReady.value) {
      ElMessage.warning(installMessage.value || '请先配置站点域名')
      return
    }
    ElMessage.success('安装命令已生成')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '生成失败')
  } finally {
    generating.value = false
  }
}

async function copyCommand() {
  if (!installReady.value || !installCommand.value || installCommand.value.includes('<你的域名>')) {
    ElMessage.warning('站点域名未配置，不能复制安装命令')
    return
  }
  try {
    await navigator.clipboard.writeText(installCommand.value)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.warning('复制失败，请手动复制')
  }
}

async function changePassword() {
  // 前端先做基础校验，后端仍会再次校验当前密码和新密码长度。
  if (!passwordForm.old_password || !passwordForm.new_password) {
    ElMessage.warning('当前密码和新密码不能为空')
    return
  }
  if (passwordForm.new_password.length < 8) {
    ElMessage.warning('新密码至少需要 8 位')
    return
  }
  if (passwordForm.new_password !== passwordForm.confirm_password) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }

  passwordSaving.value = true
  try {
    await axios.put('/api/auth/password', {
      old_password: passwordForm.old_password,
      new_password: passwordForm.new_password,
    }, { headers: authHeaders() })
    passwordForm.old_password = ''
    passwordForm.new_password = ''
    passwordForm.confirm_password = ''
    ElMessage.success('密码已修改并固化')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '修改密码失败')
  } finally {
    passwordSaving.value = false
  }
}

async function loadTelegram() {
  try {
    const res = await axios.get(`/api/telegram?_=${Date.now()}`, { headers: authHeaders() })
    tgForm.bot_token = ''
    tgForm.chat_id = res.data.chat_id || ''
    tgForm.has_bot_token = Boolean(res.data.has_bot_token)
  } catch {}
}

async function saveTelegram() {
  telegramSaving.value = true
  try {
    await axios.put('/api/telegram', {
      bot_token: tgForm.bot_token,
      chat_id: tgForm.chat_id,
    }, { headers: authHeaders() })
    tgForm.bot_token = ''
    await loadTelegram()
    ElMessage.success('Telegram 配置已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存 Telegram 配置失败')
  } finally {
    telegramSaving.value = false
  }
}

async function testTelegram() {
  telegramTesting.value = true
  try {
    await axios.post('/api/telegram/test', {
      bot_token: tgForm.bot_token,
      chat_id: tgForm.chat_id,
    }, { headers: authHeaders() })
    ElMessage.success('测试通知已发送')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '测试通知发送失败')
  } finally {
    telegramTesting.value = false
  }
}

async function loadThresholds() {
  try {
    const res = await axios.get(`/api/alert-settings?_=${Date.now()}`, { headers: authHeaders() })
    thresholds.cpu = res.data.cpu ?? thresholds.cpu
    thresholds.mem = res.data.mem ?? thresholds.mem
    thresholds.disk = res.data.disk ?? thresholds.disk
    thresholds.offline_seconds = res.data.offline_seconds ?? thresholds.offline_seconds
    thresholds.metric_duration_seconds = res.data.metric_duration_seconds ?? thresholds.metric_duration_seconds
  } catch {}
}

async function saveThresholds() {
  thresholdSaving.value = true
  try {
    await axios.put('/api/alert-settings', thresholds, { headers: authHeaders() })
    ElMessage.success('告警阈值已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存告警阈值失败')
  } finally {
    thresholdSaving.value = false
  }
}

onMounted(async () => {
  try {
    const res = await axios.get(`/api/theme?_=${Date.now()}`, { headers: authHeaders() })
    if (res.data.preset) themeForm.preset = res.data.preset
    if (res.data.primary) themeForm.primary = res.data.primary
    if (res.data.title) themeForm.title = res.data.title
    if (res.data.footer_text) themeForm.footer_text = res.data.footer_text
    themeForm.site_domain = res.data.site_domain || ''
    themeForm.agent_server_addr = res.data.agent_server_addr || ''
    applyTheme()
  } catch {}
  await Promise.all([loadTelegram(), loadThresholds()])
})
</script>

<style scoped>
.form-tip,
.install-message {
  margin-top: 6px;
  color: var(--wk-text-muted);
  font-size: 12px;
}

.install-message {
  margin-top: 12px;
}

.command-box {
  background: var(--wk-bg-soft);
  border: 1px solid var(--wk-border);
  border-radius: 8px;
  padding: 16px;
  overflow-x: auto;

  code {
    font-family: 'JetBrains Mono', monospace;
    font-size: 13px;
    color: var(--wk-primary);
    word-break: break-all;
  }
}
</style>
