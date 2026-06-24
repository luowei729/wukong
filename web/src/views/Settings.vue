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

      <!-- Ping 运营商配置 -->
      <el-tab-pane label="Ping 运营商" name="isp">
        <div class="wk-card-solid" style="padding: 20px;">
          <el-alert
            title="运营商目标会保存到 SQLite；新注册探针会自动下发，已安装探针重启后读取最新本地配置/后续热更新生效。"
            type="info"
            :closable="false"
            style="margin-bottom: 16px;"
          />
          <el-form :inline="true" class="isp-form">
            <el-form-item label="运营商">
              <el-input v-model="ispForm.name" placeholder="电信 / 联通 / Cloudflare" style="width: 180px;" />
            </el-form-item>
            <el-form-item label="目标 IP/域名">
              <el-input v-model="ispForm.ip" placeholder="1.1.1.1" style="width: 180px;" />
            </el-form-item>
            <el-form-item label="端口">
              <el-input-number v-model="ispForm.port" :min="1" :max="65535" :step="1" style="width: 130px;" />
            </el-form-item>
            <el-form-item label="模式">
              <el-select v-model="ispForm.mode" style="width: 120px;">
                <el-option label="auto" value="auto" />
                <el-option label="icmp" value="icmp" />
                <el-option label="tcp" value="tcp" />
              </el-select>
            </el-form-item>
            <el-form-item label="启用">
              <el-switch v-model="ispForm.enabled" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="ispSaving" @click="saveISPTarget">
                {{ ispForm.id ? '保存目标' : '新增目标' }}
              </el-button>
              <el-button v-if="ispForm.id" @click="resetISPForm">取消编辑</el-button>
            </el-form-item>
          </el-form>

          <el-table v-loading="ispLoading" :data="ispTargets" style="width: 100%; margin-top: 12px;">
            <el-table-column prop="name" label="运营商" min-width="130" />
            <el-table-column prop="ip" label="目标" min-width="160" />
            <el-table-column prop="port" label="端口" width="90" />
            <el-table-column prop="mode" label="模式" width="90" />
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '停用' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="170">
              <template #default="{ row }">
                <el-button size="small" text type="primary" @click="editISPTarget(row)">编辑</el-button>
                <el-button size="small" text type="danger" @click="deleteISPTarget(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
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
import { ElMessage, ElMessageBox } from 'element-plus'
import http from '@/utils/http'

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

// Ping 运营商目标
const ispTargets = ref<any[]>([])
const ispLoading = ref(false)
const ispSaving = ref(false)
const ispForm = reactive({
  id: 0,
  name: '',
  ip: '',
  port: 80,
  mode: 'auto',
  enabled: true,
})

async function saveTheme() {
  saving.value = true
  try {
    await http.put('/api/theme', themeForm)
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
    const res = await http.post('/api/install-tokens', {})
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
    await http.put('/api/auth/password', {
      old_password: passwordForm.old_password,
      new_password: passwordForm.new_password,
    })
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
    const res = await http.get(`/api/telegram?_=${Date.now()}`)
    tgForm.bot_token = ''
    tgForm.chat_id = res.data.chat_id || ''
    tgForm.has_bot_token = Boolean(res.data.has_bot_token)
  } catch {}
}

async function saveTelegram() {
  telegramSaving.value = true
  try {
    await http.put('/api/telegram', {
      bot_token: tgForm.bot_token,
      chat_id: tgForm.chat_id,
    })
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
    await http.post('/api/telegram/test', {
      bot_token: tgForm.bot_token,
      chat_id: tgForm.chat_id,
    })
    ElMessage.success('测试通知已发送')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '测试通知发送失败')
  } finally {
    telegramTesting.value = false
  }
}

async function loadThresholds() {
  try {
    const res = await http.get(`/api/alert-settings?_=${Date.now()}`)
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
    await http.put('/api/alert-settings', thresholds)
    ElMessage.success('告警阈值已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存告警阈值失败')
  } finally {
    thresholdSaving.value = false
  }
}

async function loadISPTargets() {
  ispLoading.value = true
  try {
    const res = await http.get(`/api/isp-targets?_=${Date.now()}`)
    ispTargets.value = res.data || []
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '加载 Ping 运营商目标失败')
  } finally {
    ispLoading.value = false
  }
}

function resetISPForm() {
  ispForm.id = 0
  ispForm.name = ''
  ispForm.ip = ''
  ispForm.port = 80
  ispForm.mode = 'auto'
  ispForm.enabled = true
}

function editISPTarget(row: any) {
  ispForm.id = row.id
  ispForm.name = row.name || ''
  ispForm.ip = row.ip || ''
  ispForm.port = row.port || 80
  ispForm.mode = row.mode || 'auto'
  ispForm.enabled = Boolean(row.enabled)
}

async function saveISPTarget() {
  if (!ispForm.name.trim() || !ispForm.ip.trim()) {
    ElMessage.warning('运营商名称和目标不能为空')
    return
  }
  ispSaving.value = true
  try {
    const payload = {
      name: ispForm.name.trim(),
      ip: ispForm.ip.trim(),
      port: ispForm.port,
      mode: ispForm.mode,
      enabled: ispForm.enabled,
    }
    if (ispForm.id) {
      await http.put(`/api/isp-targets/${ispForm.id}`, payload)
      ElMessage.success('Ping 目标已保存')
    } else {
      await http.post('/api/isp-targets', payload)
      ElMessage.success('Ping 目标已新增')
    }
    resetISPForm()
    await loadISPTargets()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存 Ping 目标失败')
  } finally {
    ispSaving.value = false
  }
}

async function deleteISPTarget(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除 Ping 目标“${row.name}”？`, '删除确认', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
    })
    await http.delete(`/api/isp-targets/${row.id}`)
    ElMessage.success('Ping 目标已删除')
    await loadISPTargets()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || '删除 Ping 目标失败')
    }
  }
}

onMounted(async () => {
  try {
    const res = await http.get(`/api/theme?_=${Date.now()}`)
    if (res.data.preset) themeForm.preset = res.data.preset
    if (res.data.primary) themeForm.primary = res.data.primary
    if (res.data.title) themeForm.title = res.data.title
    if (res.data.footer_text) themeForm.footer_text = res.data.footer_text
    themeForm.site_domain = res.data.site_domain || ''
    themeForm.agent_server_addr = res.data.agent_server_addr || ''
    applyTheme()
  } catch {}
  await Promise.all([loadTelegram(), loadThresholds(), loadISPTargets()])
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
