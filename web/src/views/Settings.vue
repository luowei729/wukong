<template>
  <!-- ===== 系统设置页面 ===== -->
  <div class="settings-page">
    <!-- 页面头部：标题 + 副标题说明 -->
    <header class="wk-page-header">
      <h2>系统设置</h2>
      <div class="muted">配置主题风格、安装节点、运营商 Ping、通知、告警阈值等</div>
    </header>

    <!-- 标签页：每个 Tab 对应一类设置，内容用 wk-card-solid 包裹 -->
    <el-tabs v-model="activeTab" class="wk-tabs">

      <!-- ========== 1. 主题风格 ========== -->
      <el-tab-pane label="主题风格" name="theme">
        <div class="wk-card-solid settings-card">
          <h3>主题与站点配置</h3>
          <el-form label-position="top">
            <!-- 主题预设：暗黑科技 / 浅色简洁 -->
            <el-form-item label="主题预设">
              <el-radio-group v-model="themeForm.preset">
                <el-radio value="dark">暗黑科技</el-radio>
                <el-radio value="light">浅色简洁</el-radio>
              </el-radio-group>
            </el-form-item>
            <!-- 主色调：颜色选择器 -->
            <el-form-item label="主色调">
              <el-color-picker v-model="themeForm.primary" />
            </el-form-item>
            <!-- 站点标题：浏览器标题栏 + 顶栏品牌名 -->
            <el-form-item label="站点标题">
              <el-input v-model="themeForm.title" placeholder="wukong 监控" />
            </el-form-item>
            <!-- 页脚文本 -->
            <el-form-item label="页脚文本">
              <el-input v-model="themeForm.footer_text" placeholder="Powered by wukong" />
            </el-form-item>
            <!-- 站点域名：用于生成安装脚本下载地址 -->
            <el-form-item label="站点域名 / 访问地址">
              <el-input v-model="themeForm.site_domain" placeholder="https://monitor.example.com 或 http://127.0.0.1:64443" />
              <div class="form-tip">用于生成安装脚本下载地址；未配置时不能复制安装命令。</div>
            </el-form-item>
            <!-- 探针 gRPC 地址：探针实际注册和上报地址 -->
            <el-form-item label="探针 gRPC 地址">
              <el-input v-model="themeForm.agent_server_addr" placeholder="monitor.example.com:443" />
              <div class="form-tip">探针实际注册和上报地址，必须是 host:port。生产环境推荐使用 server.lkz.pub:443。</div>
            </el-form-item>
            <!-- 保存按钮 -->
            <el-form-item>
              <el-button type="primary" :loading="saving" @click="saveTheme">
                保存主题与站点地址
              </el-button>
            </el-form-item>
          </el-form>
        </div>
      </el-tab-pane>

      <!-- ========== 2. 安装节点 ========== -->
      <el-tab-pane label="安装节点" name="install">
        <div class="wk-card-solid settings-card">
          <h3>安装新节点</h3>
          <!-- 未配置站点域名时警告 -->
          <el-alert
            v-if="!themeForm.site_domain"
            title="请先在「主题风格」里配置站点域名 / 访问地址，否则不能复制安装命令。"
            type="warning"
            :closable="false"
            class="settings-alert"
          />
          <!-- 安装说明 -->
          <el-alert
            title="安装脚本会自动识别 amd64 / arm64 节点架构，注册后由 systemd 后台常驻运行并设置开机自启。"
            type="info"
            :closable="false"
            class="settings-alert"
          />
          <!-- 生成安装命令按钮 -->
          <el-button type="primary" :loading="generating" @click="generateToken">
            生成安装命令
          </el-button>
          <!-- 生成结果消息 -->
          <div v-if="installMessage" class="install-message">
            {{ installMessage }}
          </div>
          <!-- 安装命令展示框 + 复制按钮 -->
          <div v-if="installCommand" class="install-command-wrap">
            <div class="command-box">
              <code>{{ installCommand }}</code>
            </div>
            <el-button
              size="small"
              :disabled="!installReady"
              @click="copyCommand"
              style="margin-top: 10px;"
            >
              复制命令
            </el-button>
          </div>
        </div>
      </el-tab-pane>

      <!-- ========== 3. Ping 运营商 ========== -->
      <el-tab-pane label="Ping 运营商" name="isp">
        <div class="wk-card-solid settings-card">
          <h3>Ping 运营商目标管理</h3>
          <!-- 说明：目标会下发到探针 -->
          <el-alert
            title="运营商目标会保存到 SQLite；新注册探针会自动下发，已安装探针重启后读取最新本地配置/后续热更新生效。"
            type="info"
            :closable="false"
            class="settings-alert"
          />
          <!-- 新增/编辑表单：内联布局 -->
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

          <!-- 已配置的运营商目标列表 -->
          <el-table v-loading="ispLoading" :data="ispTargets" style="width: 100%; margin-top: 12px;">
            <el-table-column prop="name" label="运营商" min-width="130" />
            <el-table-column prop="ip" label="目标" min-width="160" />
            <el-table-column prop="port" label="端口" width="90" />
            <el-table-column prop="mode" label="模式" width="90" />
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <span :class="['wk-badge', row.enabled ? 'ok' : 'warn']">
                  {{ row.enabled ? '启用' : '停用' }}
                </span>
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

      <!-- ========== 4. 修改密码 ========== -->
      <el-tab-pane label="修改密码" name="security">
        <div class="wk-card-solid settings-card">
          <h3>修改管理员密码</h3>
          <el-alert
            title="密码修改后会写入 SQLite 数据库固化，重启主控或容器后仍使用新密码。"
            type="info"
            :closable="false"
            class="settings-alert"
          />
          <el-form label-position="top">
            <!-- 当前密码 -->
            <el-form-item label="当前密码">
              <el-input v-model="passwordForm.old_password" type="password" autocomplete="current-password" show-password placeholder="输入当前管理员密码" />
            </el-form-item>
            <!-- 新密码 -->
            <el-form-item label="新密码">
              <el-input v-model="passwordForm.new_password" type="password" autocomplete="new-password" show-password placeholder="至少 8 位" />
            </el-form-item>
            <!-- 确认新密码 -->
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

      <!-- ========== 5. Telegram 通知 ========== -->
      <el-tab-pane label="Telegram 通知" name="telegram">
        <div class="wk-card-solid settings-card">
          <h3>Telegram Bot 配置</h3>
          <!-- 已保存 token 提示：不回显 token 防泄露 -->
          <el-alert
            v-if="tgForm.has_bot_token"
            title="已保存 Bot Token。为避免浏览器密码管理器误填或泄露，页面不会回显 token；留空保存表示保留原 token。"
            type="success"
            :closable="false"
            class="settings-alert"
          />
          <el-form label-position="top" autocomplete="off">
            <!-- Bot Token：不回显，留空保留原值 -->
            <el-form-item label="Bot Token">
              <el-input
                v-model="tgForm.bot_token"
                name="wukong-telegram-bot-token"
                autocomplete="off"
                placeholder="输入新的 Telegram Bot Token；留空则保留已保存 token"
              />
            </el-form-item>
            <!-- Chat ID -->
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

      <!-- ========== 6. 探针升级 ========== -->
      <el-tab-pane label="探针升级" name="upgrade">
        <div class="wk-card-solid settings-card">
          <h3>探针自动升级</h3>
          <el-alert
            title="探针自动升级机制：每次编译构建时版本号自动递增，探针每 5 分钟自动检查版本，发现新版本时自动下载、替换并重启。无需手动配置目标版本。"
            type="success"
            :closable="false"
            class="settings-alert"
          />
          <!-- 升级机制详情 -->
          <el-descriptions :column="1" border>
            <el-descriptions-item label="升级方式">自动升级（探针主动检查，无需干预）</el-descriptions-item>
            <el-descriptions-item label="检查频率">每 5 分钟</el-descriptions-item>
            <el-descriptions-item label="升级流程">下载新版本 → 备份当前 → 原子替换 → systemd 重启</el-descriptions-item>
            <el-descriptions-item label="回滚机制">升级失败自动回滚到备份版本</el-descriptions-item>
          </el-descriptions>
        </div>
      </el-tab-pane>

      <!-- ========== 7. 告警阈值 ========== -->
      <el-tab-pane label="告警阈值" name="thresholds">
        <div class="wk-card-solid settings-card">
          <h3>告警阈值配置</h3>
          <el-form label-position="top">
            <!-- 离线报警阈值：秒 -->
            <el-form-item label="离线报警阈值（秒）">
              <el-input-number v-model="thresholds.offline_seconds" :min="5" :max="3600" :step="5" />
              <div class="form-tip">节点最后心跳超过该秒数后触发离线报警，默认使用主控心跳超时配置。</div>
            </el-form-item>
            <!-- 资源告警持续时间：秒 -->
            <el-form-item label="资源告警持续时间（秒）">
              <el-input-number v-model="thresholds.metric_duration_seconds" :min="1" :max="3600" :step="5" />
              <div class="form-tip">CPU/内存/磁盘/Ping 等指标持续超过阈值多久后触发告警。</div>
            </el-form-item>
            <!-- CPU 告警阈值 -->
            <el-form-item label="CPU 告警阈值 (%)">
              <el-slider v-model="thresholds.cpu" :min="1" :max="100" show-input />
            </el-form-item>
            <!-- 内存告警阈值 -->
            <el-form-item label="内存告警阈值 (%)">
              <el-slider v-model="thresholds.mem" :min="1" :max="100" show-input />
            </el-form-item>
            <!-- 磁盘告警阈值 -->
            <el-form-item label="磁盘告警阈值 (%)">
              <el-slider v-model="thresholds.disk" :min="1" :max="100" show-input />
            </el-form-item>
            <!-- Ping 延迟告警阈值 -->
            <el-form-item label="Ping 延迟告警阈值 (ms)">
              <el-input-number v-model="thresholds.ping_latency" :min="1" :max="10000" :step="10" />
              <div class="form-tip">任一运营商线路最近延迟持续超过该阈值时触发。</div>
            </el-form-item>
            <!-- Ping 丢包告警阈值 -->
            <el-form-item label="Ping 丢包告警阈值 (%)">
              <el-slider v-model="thresholds.ping_loss" :min="1" :max="100" show-input />
              <div class="form-tip">任一运营商线路最近丢包率持续超过该阈值时触发。</div>
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
// ===== 系统设置页逻辑 =====
// 包含 7 个标签页：主题/安装/ISP/密码/Telegram/升级/阈值
// 所有配置通过 /api/* 端点读写，保持原有 API 调用不变
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import http from '@/utils/http'

// 当前激活的标签页（默认主题风格）
const activeTab = ref('theme')

// ===== 主题表单 =====
const themeForm = reactive({
  preset: 'dark',           // 主题预设：dark=暗黑 / light=浅色
  primary: '#38bdf8',       // 主色调
  title: 'wukong 监控',     // 站点标题
  footer_text: 'Powered by wukong', // 页脚文本
  site_domain: '',          // 站点域名（用于安装脚本）
  agent_server_addr: '',    // 探针 gRPC 地址
})
const saving = ref(false) // 主题保存中

// ===== 安装节点 =====
const generating = ref(false)      // 正在生成安装命令
const installCommand = ref('')     // 生成的安装命令
const installReady = ref(false)    // 安装命令是否可用（站点域名已配置）
const installMessage = ref('')     // 生成结果消息

// ===== 修改密码 =====
const passwordForm = reactive({
  old_password: '',    // 当前密码
  new_password: '',    // 新密码
  confirm_password: '', // 确认新密码
})
const passwordSaving = ref(false) // 密码修改中

// ===== Telegram 配置 =====
const tgForm = reactive({
  bot_token: '',      // Bot Token（不回显，留空保留原值）
  chat_id: '',        // Chat ID
  has_bot_token: false, // 后端是否已保存 token
})
const telegramSaving = ref(false)  // Telegram 保存中
const telegramTesting = ref(false) // 测试通知发送中

// ===== 告警阈值 =====
const thresholds = reactive({
  cpu: 90,                     // CPU 告警阈值 %
  mem: 90,                     // 内存告警阈值 %
  disk: 90,                    // 磁盘告警阈值 %
  ping_latency: 200,           // Ping 延迟告警阈值 ms
  ping_loss: 20,               // Ping 丢包告警阈值 %
  offline_seconds: 30,         // 离线报警阈值 秒
  metric_duration_seconds: 60, // 资源告警持续时间 秒
})
const thresholdSaving = ref(false) // 阈值保存中

// ===== 探针升级（自动升级，保留变量兼容） =====
const upgradeForm = reactive({
  target_version: '', // 目标版本（已废弃，自动升级）
  upgrade_url: '',    // 升级 URL（已废弃，自动升级）
})
const upgradeSaving = ref(false)

// ===== Ping 运营商目标 =====
const ispTargets = ref<any[]>([]) // 已配置的 ISP 目标列表
const ispLoading = ref(false)     // 列表加载中
const ispSaving = ref(false)      // 目标保存中
const ispForm = reactive({
  id: 0,           // 编辑时携带 ID，0 表示新增
  name: '',        // 运营商名称
  ip: '',          // 目标 IP/域名
  port: 80,        // 端口
  mode: 'auto',    // 探测模式：auto/icmp/tcp
  enabled: true,   // 是否启用
})

// ===== 保存主题配置 =====
// 写入 /api/theme，同时在前端应用主题（data-theme + class + title）
async function saveTheme() {
  saving.value = true
  try {
    await http.put('/api/theme', themeForm)
    // 应用主题到 DOM
    applyTheme()
    ElMessage.success('主题和站点地址已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

// 应用主题到前端 DOM
// 设置 data-theme 属性、dark class、localStorage、文档标题
function applyTheme() {
  document.documentElement.dataset.theme = themeForm.preset
  document.documentElement.classList.toggle('dark', themeForm.preset === 'dark')
  localStorage.setItem('theme', themeForm.preset)
  document.title = themeForm.title
}

// ===== 生成安装命令 =====
// 调用 /api/install-tokens 生成一次性 token 和安装脚本 URL
async function generateToken() {
  generating.value = true
  installCommand.value = ''
  installReady.value = false
  installMessage.value = ''
  try {
    const res = await http.post('/api/install-tokens', {})
    // ready=true 表示站点域名已配置，可以复制安装命令
    installReady.value = Boolean(res.data.ready)
    installCommand.value = res.data.script_url || ''
    installMessage.value = res.data.message || ''
    if (!installReady.value) {
      // 未配置站点域名时提示
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

// ===== 复制安装命令到剪贴板 =====
async function copyCommand() {
  // 未就绪或命令包含占位符时不允许复制
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

// ===== 修改密码 =====
// 前端先做基础校验，后端仍会再次校验当前密码和新密码长度
async function changePassword() {
  // 校验：当前密码和新密码不能为空
  if (!passwordForm.old_password || !passwordForm.new_password) {
    ElMessage.warning('当前密码和新密码不能为空')
    return
  }
  // 校验：新密码至少 8 位
  if (passwordForm.new_password.length < 8) {
    ElMessage.warning('新密码至少需要 8 位')
    return
  }
  // 校验：两次输入的新密码必须一致
  if (passwordForm.new_password !== passwordForm.confirm_password) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }

  passwordSaving.value = true
  try {
    // 调用后端修改密码接口，bcrypt hash 写入 SQLite 固化
    await http.put('/api/auth/password', {
      old_password: passwordForm.old_password,
      new_password: passwordForm.new_password,
    })
    // 清空表单
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

// ===== 加载 Telegram 配置 =====
// 不回显 bot_token，只读取 chat_id 和 has_bot_token 标记
async function loadTelegram() {
  try {
    const res = await http.get(`/api/telegram?_=${Date.now()}`)
    tgForm.bot_token = '' // 始终清空，不回显
    tgForm.chat_id = res.data.chat_id || ''
    tgForm.has_bot_token = Boolean(res.data.has_bot_token)
  } catch {}
}

// ===== 保存 Telegram 配置 =====
async function saveTelegram() {
  telegramSaving.value = true
  try {
    await http.put('/api/telegram', {
      bot_token: tgForm.bot_token,
      chat_id: tgForm.chat_id,
    })
    // 保存后清空 token 输入框并重新加载
    tgForm.bot_token = ''
    await loadTelegram()
    ElMessage.success('Telegram 配置已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存 Telegram 配置失败')
  } finally {
    telegramSaving.value = false
  }
}

// ===== 发送测试通知 =====
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

// ===== 加载探针升级配置（保留兼容，自动升级模式下无需手动配置） =====
async function loadUpgradeSettings() {
  try {
    const [targetRes, urlRes] = await Promise.all([
      http.get(`/api/settings/agent_target_version?_=${Date.now()}`),
      http.get(`/api/settings/agent_upgrade_url?_=${Date.now()}`),
    ])
    upgradeForm.target_version = targetRes.data.value || ''
    upgradeForm.upgrade_url = urlRes.data.value || ''
  } catch {}
}

// ===== 加载告警阈值 =====
async function loadThresholds() {
  try {
    const res = await http.get(`/api/alert-settings?_=${Date.now()}`)
    // 用后端返回值覆盖默认值，?? 保留默认值防止字段缺失
    thresholds.cpu = res.data.cpu ?? thresholds.cpu
    thresholds.mem = res.data.mem ?? thresholds.mem
    thresholds.disk = res.data.disk ?? thresholds.disk
    thresholds.ping_latency = res.data.ping_latency ?? thresholds.ping_latency
    thresholds.ping_loss = res.data.ping_loss ?? thresholds.ping_loss
    thresholds.offline_seconds = res.data.offline_seconds ?? thresholds.offline_seconds
    thresholds.metric_duration_seconds = res.data.metric_duration_seconds ?? thresholds.metric_duration_seconds
  } catch {}
}

// ===== 保存告警阈值 =====
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

// ===== 加载 ISP 运营商目标列表 =====
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

// ===== 重置 ISP 表单（取消编辑） =====
function resetISPForm() {
  ispForm.id = 0
  ispForm.name = ''
  ispForm.ip = ''
  ispForm.port = 80
  ispForm.mode = 'auto'
  ispForm.enabled = true
}

// ===== 编辑 ISP 目标（填充表单） =====
function editISPTarget(row: any) {
  ispForm.id = row.id
  ispForm.name = row.name || ''
  ispForm.ip = row.ip || ''
  ispForm.port = row.port || 80
  ispForm.mode = row.mode || 'auto'
  ispForm.enabled = Boolean(row.enabled)
}

// ===== 新增/保存 ISP 目标 =====
async function saveISPTarget() {
  // 校验：名称和目标不能为空
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
      // 有 ID：更新已有目标
      await http.put(`/api/isp-targets/${ispForm.id}`, payload)
      ElMessage.success('Ping 目标已保存')
    } else {
      // 无 ID：新增目标
      await http.post('/api/isp-targets', payload)
      ElMessage.success('Ping 目标已新增')
    }
    // 重置表单并刷新列表
    resetISPForm()
    await loadISPTargets()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存 Ping 目标失败')
  } finally {
    ispSaving.value = false
  }
}

// ===== 删除 ISP 目标 =====
async function deleteISPTarget(row: any) {
  try {
    // 二次确认
    await ElMessageBox.confirm(`确认删除 Ping 目标"${row.name}"？`, '删除确认', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
    })
    await http.delete(`/api/isp-targets/${row.id}`)
    ElMessage.success('Ping 目标已删除')
    await loadISPTargets()
  } catch (e: any) {
    // 用户点取消时不报错
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || '删除 Ping 目标失败')
    }
  }
}

// ===== 组件挂载：加载所有配置 =====
onMounted(async () => {
  // 先加载主题配置并应用
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
  // 并行加载其余配置
  await Promise.all([loadTelegram(), loadThresholds(), loadISPTargets(), loadUpgradeSettings()])
})
</script>

<style scoped>
/* ===== 系统设置页局部样式 ===== */

/* 标签页容器：适配新主题 */
.wk-tabs {
  :deep(.el-tabs__header) {
    margin-bottom: 18px;
  }

  :deep(.el-tabs__item) {
    font-weight: 550;
    font-size: 14px;
    color: var(--wk-text-muted);
    transition: color .15s;

    &.is-active {
      color: var(--wk-primary);
      font-weight: 650;
    }

    &:hover {
      color: var(--wk-primary-hover);
    }
  }

  :deep(.el-tabs__active-bar) {
    background-color: var(--wk-primary);
    height: 3px;
    border-radius: 2px;
  }

  :deep(.el-tabs__nav-wrap::after) {
    background-color: var(--wk-border);
    height: 1px;
  }
}

/* 设置卡片：统一内边距和间距 */
.settings-card {
  padding: 22px 24px;
  margin-bottom: 0;

  h3 {
    font-size: 15px;
    font-weight: 650;
    color: var(--wk-text);
    margin-bottom: 16px;
    padding-bottom: 12px;
    border-bottom: 1px solid var(--wk-border);
  }
}

/* 设置页内的 alert 统一间距 */
.settings-alert {
  margin-bottom: 16px;
}

/* 表单提示文字 */
.form-tip {
  margin-top: 6px;
  color: var(--wk-text-muted);
  font-size: 12px;
  line-height: 1.5;
}

/* 安装命令消息 */
.install-message {
  margin-top: 12px;
  color: var(--wk-text-muted);
  font-size: 13px;
}

/* 安装命令展示区域 */
.install-command-wrap {
  margin-top: 16px;
}

/* 命令框：暗色背景 + 等宽字体 + 横向滚动 */
.command-box {
  background: var(--wk-bg-soft);
  border: 1px solid var(--wk-border);
  border-radius: 10px;
  padding: 16px;
  overflow-x: auto;

  code {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 13px;
    color: var(--wk-primary);
    word-break: break-all;
    line-height: 1.6;
  }
}

/* ISP 内联表单：小屏下自动换行 */
.isp-form {
  :deep(.el-form-item) {
    margin-bottom: 12px;
  }
}

/* 响应式：小屏调整 */
@media (max-width: 768px) {
  .settings-card {
    padding: 16px 14px;
  }

  .isp-form {
    :deep(.el-form-item) {
      width: 100%;
      margin-right: 0;
    }
  }
}
</style>
