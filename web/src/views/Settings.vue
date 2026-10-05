<template>
  <!-- ============ 系统设置 ============ -->
  <!-- 结构：左侧竖排分区导航 + 右侧内容卡片（分区与接口与原 el-tabs 版本完全一致） -->
  <div class="wk-settings">
    <nav class="wk-settings-nav" aria-label="设置分区">
      <div v-for="group in navGroups" :key="group.title" class="wk-nav-group">
        <div class="wk-nav-group-title">{{ group.title }}</div>
        <button
          v-for="item in group.items"
          :key="item.key"
          type="button"
          :class="['wk-set-item', { active: activeSection === item.key }]"
          @click="activeSection = item.key"
        >
          <span class="wk-set-ico" v-html="item.icon"></span>
          <span>{{ item.label }}</span>
        </button>
      </div>
    </nav>

    <div class="wk-settings-body">
      <!-- ================= 1. 主题风格 ================= -->
      <WkCard v-show="activeSection === 'theme'" title="主题与站点" subtitle="改动即时预览，保存后写入 SQLite 固化">
        <div class="wk-set-grid">
          <!-- 预设选择：两张卡片比单选按钮更直观 -->
          <div class="wk-set-field">
            <label class="wk-eyebrow">主题预设</label>
            <div class="wk-preset-picker">
              <button
                v-for="preset in presetOptions"
                :key="preset.value"
                type="button"
                :class="['wk-preset', { active: themeForm.preset === preset.value }]"
                @click="selectPreset(preset.value)"
              >
                <span class="wk-preset-swatch" :style="preset.style" />
                <span class="wk-preset-label">{{ preset.label }}</span>
              </button>
            </div>
          </div>

          <!-- 主色：预设色板 + 自由取色，选中即预览（本次修复的核心：主色真正写进 CSS 变量） -->
          <div class="wk-set-field">
            <label class="wk-eyebrow">主色调</label>
            <div class="wk-swatches">
              <button
                v-for="color in primarySwatches"
                :key="color"
                type="button"
                :class="['wk-swatch', { active: themeForm.primary?.toLowerCase() === color }]"
                :style="{ background: color }"
                :aria-label="`使用主色 ${color}`"
                @click="selectPrimary(color)"
              />
              <el-color-picker v-model="themeForm.primary" size="small" @change="onPrimaryChange" />
            </div>
            <div class="form-tip">主色用于导航激活态、进度条与图表首色；保存后公开页同样生效。</div>
          </div>

          <div class="wk-set-field">
            <label class="wk-eyebrow" for="site-title">站点标题</label>
            <el-input id="site-title" v-model="themeForm.title" placeholder="wukong 监控" />
            <div class="form-tip">显示在浏览器标签、登录页与公开首页。</div>
          </div>

          <div class="wk-set-field">
            <label class="wk-eyebrow" for="site-footer">页脚文本</label>
            <el-input id="site-footer" v-model="themeForm.footer_text" placeholder="Powered by wukong" />
          </div>

          <div class="wk-set-field">
            <label class="wk-eyebrow" for="site-domain">站点域名 / 访问地址</label>
            <el-input
              id="site-domain"
              v-model="themeForm.site_domain"
              placeholder="https://monitor.example.com 或 http://127.0.0.1:64443"
            />
            <div class="form-tip">用于生成安装脚本下载地址；未配置时不能复制安装命令。</div>
          </div>

          <div class="wk-set-field">
            <label class="wk-eyebrow" for="agent-addr">探针 gRPC 地址</label>
            <el-input
              id="agent-addr"
              v-model="themeForm.agent_server_addr"
              placeholder="monitor.example.com:443"
            />
            <div class="form-tip">探针实际注册与上报地址，必须是 host:port 格式。</div>
          </div>
        </div>

        <div class="wk-set-actions">
          <el-button type="primary" :loading="saving" @click="saveTheme">保存主题与站点地址</el-button>
        </div>
      </WkCard>

      <!-- ================= 2. 安装节点 ================= -->
      <WkCard v-show="activeSection === 'install'" title="安装新节点" subtitle="一次性 token 30 分钟过期，使用后立即作废">
        <ol class="wk-steps">
          <li>配置好上方的「站点域名 / 访问地址」，否则无法生成可复制的安装命令。</li>
          <li>点击下方按钮生成安装命令，脚本会自动识别 amd64 / arm64 架构。</li>
          <li>在目标服务器以 root 执行命令，探针注册成功后由 systemd 常驻并开机自启。</li>
        </ol>

        <el-alert
          v-if="!themeForm.site_domain"
          title="尚未配置站点域名，安装命令不可用。请先到「主题与站点」填写访问地址。"
          type="warning"
          :closable="false"
          style="margin-bottom: var(--wk-space-4)"
        />

        <div class="wk-row" style="flex-wrap: wrap">
          <el-button type="primary" :loading="generating" @click="generateToken">生成安装命令</el-button>
          <span v-if="installMessage" class="wk-sub">{{ installMessage }}</span>
        </div>

        <!-- 命令框：等宽 + 复制按钮常驻右上角 -->
        <div v-if="installCommand" class="wk-codebox" style="margin-top: var(--wk-space-4)">
          <el-button
            class="wk-codebox-copy"
            size="small"
            :disabled="!installReady"
            @click="copyCommand"
          >
            复制命令
          </el-button>
          <code>{{ installCommand }}</code>
        </div>
      </WkCard>

      <!-- ================= 3. Ping 运营商 ================= -->
      <WkCard v-show="activeSection === 'isp'" title="Ping 运营商目标" subtitle="目标写入 SQLite，并下发给探针执行 ICMP / TCP 探测">
        <el-form :inline="true" class="wk-isp-form">
          <el-form-item label="运营商">
            <el-input v-model="ispForm.name" placeholder="电信 / 联通" style="width: 170px" />
          </el-form-item>
          <el-form-item label="目标 IP/域名">
            <el-input v-model="ispForm.ip" placeholder="1.1.1.1" style="width: 180px" />
          </el-form-item>
          <el-form-item label="端口">
            <el-input-number v-model="ispForm.port" :min="1" :max="65535" :step="1" style="width: 120px" />
          </el-form-item>
          <el-form-item label="模式">
            <el-select v-model="ispForm.mode" style="width: 110px">
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

        <!-- 作用域：无公网 IPv6 出口的节点探测 IPv6 目标会全部失败，需要按节点选中/排除 -->
        <div class="wk-isp-scope">
          <div class="wk-isp-scope-row">
            <span class="wk-isp-scope-label">作用域</span>
            <el-select v-model="ispForm.scope" style="width: 158px">
              <el-option label="全部节点" value="all" />
              <el-option label="仅选中节点" value="include" />
              <el-option label="排除选中节点" value="exclude" />
            </el-select>
            <template v-if="ispForm.scope !== 'all'">
              <el-select
                v-model="ispForm.agent_ids"
                multiple
                collapse-tags
                collapse-tags-tooltip
                filterable
                placeholder="选择节点"
                style="flex: 1; min-width: 200px"
              >
                <el-option
                  v-for="node in agentOptions"
                  :key="node.id"
                  :label="node.label"
                  :value="node.id"
                >
                  <span>{{ node.label }}</span>
                  <span class="wk-opt-v6">{{ node.ip_v6 ? 'IPv6' : 'IPv4' }}</span>
                </el-option>
              </el-select>
              <!-- IPv6 目标的一键选择：按探针上报的公网出口是否含 IPv6 判定 -->
              <el-button v-if="isIPv6Target" size="small" @click="pickIPv6Nodes">
                {{ ispForm.scope === 'include' ? '选中有 IPv6 的' : '选中无 IPv6 的' }}
              </el-button>
            </template>
          </div>
          <div class="form-tip">{{ scopeHint }}</div>
        </div>

        <el-table v-loading="ispLoading" :data="ispTargets" style="width: 100%">
          <el-table-column prop="name" label="运营商" min-width="130" />
          <el-table-column prop="ip" label="目标" min-width="170">
            <template #default="{ row }">
              <span class="wk-num">{{ row.ip }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="port" label="端口" width="90" align="right">
            <template #default="{ row }">
              <span class="wk-num">{{ row.port }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="mode" label="模式" width="90">
            <template #default="{ row }">
              <span class="wk-tag">{{ row.mode }}</span>
            </template>
          </el-table-column>
          <!-- 作用域列：一眼看出该线路跑在哪些节点上 -->
          <el-table-column label="作用域" min-width="150">
            <template #default="{ row }">
              <span class="wk-sub">{{ scopeText(row) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <WkBadge :tone="row.enabled ? 'ok' : 'warn'">{{ row.enabled ? '启用' : '停用' }}</WkBadge>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="150" align="right">
            <template #default="{ row }">
              <el-button size="small" text type="primary" @click="editISPTarget(row)">编辑</el-button>
              <el-button size="small" text type="danger" @click="deleteISPTarget(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </WkCard>

      <!-- ================= 4. 告警阈值 ================= -->
      <WkCard v-show="activeSection === 'thresholds'" title="告警阈值" subtitle="固定 6 项指标 + 持续时间，支持探针 / 分组 / 全局三级回退">
        <div class="wk-set-grid">
          <div class="wk-set-field">
            <label class="wk-eyebrow">离线报警阈值（秒）</label>
            <el-input-number v-model="thresholds.offline_seconds" :min="5" :max="3600" :step="5" />
            <div class="form-tip">节点最后心跳超过该秒数后触发离线告警。</div>
          </div>

          <div class="wk-set-field">
            <label class="wk-eyebrow">资源告警持续时间（秒）</label>
            <el-input-number v-model="thresholds.metric_duration_seconds" :min="1" :max="3600" :step="5" />
            <div class="form-tip">指标持续超过阈值多久后才告警，用于滞回防抖。</div>
          </div>
        </div>

        <!-- 百分比阈值统一用滑杆，输入框联动，比纯数字更容易设定区间 -->
        <div class="wk-slider-group">
          <div v-for="item in sliderThresholds" :key="item.key" class="wk-slider-row">
            <span class="wk-slider-label">{{ item.label }}</span>
            <el-slider v-model="thresholds[item.key]" :min="1" :max="100" show-input :show-input-controls="false" />
          </div>
        </div>

        <div class="wk-set-grid" style="margin-top: var(--wk-space-4)">
          <div class="wk-set-field">
            <label class="wk-eyebrow">Ping 延迟告警阈值 (ms)</label>
            <el-input-number v-model="thresholds.ping_latency" :min="1" :max="10000" :step="10" />
            <div class="form-tip">任一运营商线路最近延迟持续超过该阈值时触发。</div>
          </div>
        </div>

        <div class="wk-set-actions">
          <el-button type="primary" :loading="thresholdSaving" @click="saveThresholds">保存阈值</el-button>
        </div>
      </WkCard>

      <!-- ================= 5. Telegram 通知 ================= -->
      <WkCard v-show="activeSection === 'telegram'" title="Telegram 通知" subtitle="Token 只写入后端加密存储，页面不回显">
        <el-alert
          v-if="tgForm.has_bot_token"
          title="已保存 Bot Token。为避免误填或泄露，页面不会回显 token；留空保存表示保留原 token。"
          type="success"
          :closable="false"
          style="margin-bottom: var(--wk-space-4)"
        />

        <el-form label-position="top" autocomplete="off" class="wk-narrow">
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
              placeholder="输入 Chat ID（需先给 bot 发过消息）"
            />
          </el-form-item>
        </el-form>

        <div class="wk-set-actions">
          <el-button type="primary" :loading="telegramSaving" @click="saveTelegram">保存</el-button>
          <el-button :loading="telegramTesting" @click="testTelegram">发送测试通知</el-button>
        </div>
      </WkCard>

      <!-- ================= 5.5 微信推送（pushplus 中转） ================= -->
      <WkCard
        v-show="activeSection === 'pushplus'"
        title="微信推送（pushplus 中转）"
        subtitle="支持微信 ClawBot / 公众号 / 企业微信应用，令牌只写后端不回显"
      >
        <el-alert
          title="使用步骤：① 打开 pushplus 官网，关注公众号，在「个人中心 → 渠道配置」绑定对应渠道；② 把「我的凭证」里的用户令牌粘到下面；③ 点发送测试通知后到微信确认是否收到。"
          type="info"
          :closable="false"
          style="margin-bottom: var(--wk-space-3)"
        />
        <el-alert
          title="微信 ClawBot 官方限制：每下发 10 条消息、或每隔 24 小时，都需要你在微信里主动给 ClawBot 发一条消息才能继续下发。因此本渠道只推 warning/critical 触发与恢复通知，并把一个窗口内的多条告警合并成一条（第一条仍立即发）。若不想受此限制，把渠道改成「微信公众号」即可，它没有条数激活限制。"
          type="warning"
          :closable="false"
          style="margin-bottom: var(--wk-space-3)"
        />
        <el-alert
          v-if="ppForm.has_token"
          title="已保存 pushplus 令牌。为避免泄露页面不回显；留空保存表示保留原令牌。"
          type="success"
          :closable="false"
          style="margin-bottom: var(--wk-space-4)"
        />

        <el-form label-position="top" autocomplete="off" class="wk-narrow">
          <el-form-item label="启用微信推送">
            <el-switch v-model="ppForm.enabled" />
          </el-form-item>
          <el-form-item label="用户令牌（Token）">
            <el-input
              v-model="ppForm.token"
              name="wukong-pushplus-token"
              autocomplete="off"
              placeholder="pushplus「我的凭证」里的那串 token；留空则保留已保存令牌"
            />
          </el-form-item>
          <el-form-item label="推送渠道">
            <el-select v-model="ppForm.channel" style="width: 100%">
              <el-option v-for="c in PUSHPLUS_CHANNELS" :key="c.value" :label="c.label" :value="c.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="告警合并窗口（分钟）">
            <el-input-number v-model="ppForm.merge_minutes" :min="1" :max="60" />
            <span class="wk-sub" style="margin-left: 10px">窗口内多条告警合并为一条，1~60 分钟</span>
          </el-form-item>
        </el-form>

        <div class="wk-set-actions">
          <el-button type="primary" :loading="pushplusSaving" @click="savePushplus">保存</el-button>
          <el-button :loading="pushplusTesting" @click="testPushplus">发送测试通知</el-button>
        </div>
      </WkCard>

      <!-- ================= 6. 修改密码 ================= -->
      <WkCard v-show="activeSection === 'security'" title="修改管理员密码" subtitle="bcrypt hash 写入 SQLite，重启后仍使用新密码">
        <el-form label-position="top" class="wk-narrow">
          <el-form-item label="当前密码">
            <el-input
              v-model="passwordForm.old_password"
              type="password"
              autocomplete="current-password"
              show-password
              placeholder="输入当前管理员密码"
            />
          </el-form-item>
          <el-form-item label="新密码">
            <el-input
              v-model="passwordForm.new_password"
              type="password"
              autocomplete="new-password"
              show-password
              placeholder="至少 8 位"
            />
          </el-form-item>
          <el-form-item label="确认新密码">
            <el-input
              v-model="passwordForm.confirm_password"
              type="password"
              autocomplete="new-password"
              show-password
              placeholder="再次输入新密码"
            />
          </el-form-item>
        </el-form>

        <div class="wk-set-actions">
          <el-button type="primary" :loading="passwordSaving" @click="changePassword">修改并固化密码</el-button>
        </div>
      </WkCard>

      <!-- ================= 7. 探针升级 ================= -->
      <WkCard v-show="activeSection === 'upgrade'" title="探针自动升级" subtitle="只读信息：升级由探针主动自检完成，无需手动配置目标版本">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="升级方式">自动升级（探针主动检查，无需干预）</el-descriptions-item>
          <el-descriptions-item label="检查频率">每 5 分钟</el-descriptions-item>
          <el-descriptions-item label="升级流程">下载新版本 → 备份当前 → 验签 → 原子替换 → systemd 重启</el-descriptions-item>
          <el-descriptions-item label="回滚机制">升级失败自动回滚到备份版本</el-descriptions-item>
          <el-descriptions-item label="当前目标版本">{{ upgradeForm.target_version || '主控自动维护' }}</el-descriptions-item>
        </el-descriptions>
      </WkCard>
    </div>
  </div>
</template>

<script setup lang="ts">
// ============ 系统设置页逻辑 ============
// 分区由 el-tabs 改为左侧竖排导航，但每个分区调用的接口、字段、校验规则一字未改：
//   主题 /api/theme ｜ 安装 /api/install-tokens ｜ ISP /api/isp-targets ｜
//   阈值 /api/alert-settings ｜ Telegram /api/telegram[/test] ｜ 密码 /api/auth/password
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import WkBadge from '@/components/WkBadge.vue'
import WkCard from '@/components/WkCard.vue'
import http from '@/utils/http'
import { useTheme, type ThemePreset } from '@/composables/useTheme'

// 当前激活分区（默认主题）
const activeSection = ref('theme')

// 主题单例：设置页改动要真正写进 CSS 变量，必须走它而不是自己拼 data-theme
const theme = useTheme()

// ===== 主题表单 =====
const themeForm = reactive({
  preset: 'dark' as ThemePreset,
  primary: '#5e6ad2',
  title: 'wukong 监控',
  footer_text: 'Powered by wukong',
  site_domain: '',
  agent_server_addr: '',
})
const saving = ref(false)

// 预设卡片：小色块预览各预设的画布/表面/主色搭配
const presetOptions: Array<{ label: string; value: ThemePreset; style: Record<string, string> }> = [
  {
    label: '暗黑',
    value: 'dark',
    style: { background: '#131316', borderColor: '#2a2a30', borderTop: '3px solid #5e6ad2' },
  },
  {
    label: '浅色',
    value: 'light',
    style: { background: '#ffffff', borderColor: '#e4e4e7', borderTop: '3px solid #4f5bd5' },
  },
]

// 常用主色板：避免用户面对空取色器无从下手
const primarySwatches = [
  '#5e6ad2',
  '#3b82f6',
  '#0ea5e9',
  '#10b981',
  '#f59e0b',
  '#ec4899',
]

// ===== 安装节点 =====
const generating = ref(false)
const installCommand = ref('')
const installReady = ref(false)
const installMessage = ref('')

// ===== 修改密码 =====
const passwordForm = reactive({ old_password: '', new_password: '', confirm_password: '' })
const passwordSaving = ref(false)

// ===== Telegram =====
const tgForm = reactive({ bot_token: '', chat_id: '', has_bot_token: false })
const telegramSaving = ref(false)
const telegramTesting = ref(false)

// ===== 微信推送（pushplus 中转） =====
// 只列出免费且无需额外 option 配置就能直达个人的渠道，与后端 pushplusChannelSet 保持一致
const PUSHPLUS_CHANNELS = [
  { value: 'clawbot', label: '微信 ClawBot（个人微信；每 10 条需主动激活一次）' },
  { value: 'wechat', label: '微信公众号（无条数激活限制，日常推荐）' },
  { value: 'cp', label: '企业微信应用' },
  { value: 'cmcc', label: '新消息 ClawBot（仅中国移动用户）' },
  { value: 'qq', label: 'QQ 机器人' },
  { value: 'mail', label: '邮件' },
]
const ppForm = reactive({
  enabled: false,
  token: '',
  has_token: false,
  channel: 'clawbot',
  merge_minutes: 5,
})
const pushplusSaving = ref(false)
const pushplusTesting = ref(false)

// ===== 告警阈值 =====
const thresholds = reactive<Record<string, number>>({
  cpu: 90,
  mem: 90,
  disk: 90,
  ping_latency: 200,
  ping_loss: 20,
  offline_seconds: 30,
  metric_duration_seconds: 60,
})
const thresholdSaving = ref(false)

// 滑杆型阈值（百分比语义）单独列出，渲染成统一的滑杆行
const sliderThresholds: Array<{ key: string; label: string }> = [
  { key: 'cpu', label: 'CPU (%)' },
  { key: 'mem', label: '内存 (%)' },
  { key: 'disk', label: '磁盘 (%)' },
  { key: 'ping_loss', label: 'Ping 丢包 (%)' },
]

// ===== 探针升级（只读展示，保留字段兼容） =====
const upgradeForm = reactive({ target_version: '', upgrade_url: '' })

// ===== Ping 运营商目标 =====
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
  // 作用域：all=全部节点，include=仅选中节点，exclude=排除选中节点
  scope: 'all',
  // include/exclude 模式下涉及的节点 ID
  agent_ids: [] as string[],
})

// 节点候选列表：只用于作用域选择，带探针自报的公网出口 IP 以便判断能否走 IPv6
const agentList = ref<any[]>([])

const agentOptions = computed(() =>
  agentList.value.map((node: any) => ({
    id: node.id,
    ip_v6: node.ip_v6 || '',
    label: node.name || node.hostname || `节点 ${String(node.id).slice(0, 8)}`,
  }))
)

// 目标地址含冒号就视为 IPv6，用于给出一键选择与风险提示
const isIPv6Target = computed(() => ispForm.ip.includes(':'))

// 作用域说明：把“为什么需要排除节点”直接写在表单下
const scopeHint = computed(() => {
  const base =
    ispForm.scope === 'include'
      ? '仅选中的节点会探测该线路，其余节点不下发。'
      : ispForm.scope === 'exclude'
        ? '选中节点不下发该线路，其余节点正常探测。'
        : '下发给所有探针探测。'
  if (ispForm.scope !== 'all' && isIPv6Target.value) {
    return `${base} 无公网 IPv6 出口的节点探测 IPv6 目标会全部失败并显示 100% 丢包。`
  }
  return base
})

// 一键把“有/无公网 IPv6 出口”的节点填进选择框：
// include 模式选有 IPv6 的节点，exclude 模式选没 IPv6 的节点
function pickIPv6Nodes() {
  const withV6 = agentList.value.filter((node: any) => node.ip_v6).map((node: any) => node.id)
  const withoutV6 = agentList.value.filter((node: any) => !node.ip_v6).map((node: any) => node.id)
  ispForm.agent_ids = ispForm.scope === 'include' ? withV6 : withoutV6
}

// 列表里的作用域摘要：把 ID 列表翻译成人能读的文案
function scopeText(row: any): string {
  const ids: string[] = Array.isArray(row?.agent_ids) ? row.agent_ids : []
  if (!row?.scope || row.scope === 'all' || ids.length === 0) return '全部节点'
  const names = ids.map(
    (id) => agentList.value.find((node: any) => node.id === id)?.name || String(id).slice(0, 8)
  )
  const suffix = row.scope === 'include' ? '仅' : '排除'
  return `${suffix} ${ids.length} 个：${names.join('、')}`
}

// 拉取节点列表供作用域选择使用
async function loadAgentOptions() {
  try {
    const res = await http.get(`/api/agents?_=${Date.now()}`)
    agentList.value = res.data || []
  } catch {
    agentList.value = []
  }
}

// 左侧分区导航配置：分组标题 + 条目 + 内联图标
const navGroups = [
  {
    title: '站点',
    items: [
      {
        key: 'theme',
        label: '主题与站点',
        icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75"><circle cx="13.5" cy="6.5" r="2.5"/><circle cx="19" cy="13" r="2"/><circle cx="6" cy="12" r="3"/><circle cx="10" cy="18.5" r="2.5"/><path d="M12 3a9 9 0 1 0 0 18c1.5 0 2-1 1.5-2s.5-2 2-2h2a3 3 0 0 0 3-3 8 8 0 0 0-9-9z"/></svg>',
      },
      {
        key: 'install',
        label: '安装节点',
        icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v12M7 10l5 5 5-5M4 21h16"/></svg>',
      },
    ],
  },
  {
    title: '监控',
    items: [
      {
        key: 'isp',
        label: 'Ping 运营商',
        icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a15 15 0 0 1 0 18M12 3a15 15 0 0 0 0 18"/></svg>',
      },
      {
        key: 'thresholds',
        label: '告警阈值',
        icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round"><path d="M4 18V9M10 18V4M16 18v-7M22 18h-20"/></svg>',
      },
    ],
  },
  {
    title: '通知与账户',
    items: [
      {
        key: 'telegram',
        label: 'Telegram',
        icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linejoin="round"><path d="M21 4L3 11l6 2 2 6 3-4 5 3z"/></svg>',
      },
      {
        key: 'pushplus',
        label: '微信推送',
        icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linejoin="round"><path d="M4 17l-2 4 4.5-1.5A9 9 0 1 0 4 17z"/></svg>',
      },
      {
        key: 'security',
        label: '修改密码',
        icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75"><rect x="4" y="10" width="16" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></svg>',
      },
      {
        key: 'upgrade',
        label: '探针升级',
        icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="M12 21V9M7 14l5-5 5 5M4 3h16"/></svg>',
      },
    ],
  },
]

// ===== 主题相关操作 =====
// 选择预设：立即应用，用户在设置页就能看到整套配色变化
function selectPreset(preset: ThemePreset) {
  themeForm.preset = preset
  theme.setPreset(preset)
}

// 选择主色：写入 --wk-primary，派生色与 EP 变量自动联动
function selectPrimary(color: string) {
  themeForm.primary = color
  theme.setPrimary(color)
}

function onPrimaryChange(color: string | null) {
  if (color) theme.setPrimary(color)
}

// 保存主题：接口不变（PUT /api/theme），保存后把最新标题/页脚同步到主题单例
async function saveTheme() {
  saving.value = true
  try {
    await http.put('/api/theme', themeForm)
    theme.state.title = themeForm.title
    theme.state.footer = themeForm.footer_text
    theme.setPreset(themeForm.preset)
    theme.setPrimary(themeForm.primary)
    ElMessage.success('主题和站点地址已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

// ===== 生成安装命令 =====
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

// ===== 复制安装命令 =====
async function copyCommand() {
  if (
    !installReady.value ||
    !installCommand.value ||
    installCommand.value.includes('<你的域名>')
  ) {
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
async function changePassword() {
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

// ===== Telegram =====
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
    await http.put('/api/telegram', { bot_token: tgForm.bot_token, chat_id: tgForm.chat_id })
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
    await http.post('/api/telegram/test', { bot_token: tgForm.bot_token, chat_id: tgForm.chat_id })
    ElMessage.success('测试通知已发送')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '测试通知发送失败')
  } finally {
    telegramTesting.value = false
  }
}

// ===== 微信推送（pushplus）=====
async function loadPushplus() {
  try {
    const res = await http.get(`/api/pushplus?_=${Date.now()}`)
    ppForm.enabled = Boolean(res.data.enabled)
    // 令牌不回显，永远从空开始；留空保存代表保留库里已有值
    ppForm.token = ''
    ppForm.has_token = Boolean(res.data.has_token)
    ppForm.channel = res.data.channel || 'clawbot'
    ppForm.merge_minutes = res.data.merge_minutes || 5
  } catch {}
}

async function savePushplus() {
  pushplusSaving.value = true
  try {
    await http.put('/api/pushplus', {
      enabled: ppForm.enabled,
      token: ppForm.token,
      channel: ppForm.channel,
      merge_minutes: ppForm.merge_minutes,
    })
    ppForm.token = ''
    await loadPushplus()
    ElMessage.success('微信推送配置已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存微信推送配置失败')
  } finally {
    pushplusSaving.value = false
  }
}

async function testPushplus() {
  pushplusTesting.value = true
  try {
    const res = await http.post('/api/pushplus/test', { token: ppForm.token, channel: ppForm.channel })
    // pushplus 接口是异步的，200 只代表“已受理”，所以直接展示后端那段提醒文本，开久一点
    ElMessage({ type: 'success', message: res.data?.message || '测试请求已发出', duration: 15000, showClose: true })
  } catch (e: any) {
    ElMessage({
      type: 'error',
      message: e.response?.data?.error || '测试通知发送失败',
      duration: 15000,
      showClose: true,
    })
  } finally {
    pushplusTesting.value = false
  }
}

// ===== 探针升级信息（只读） =====
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

// ===== 告警阈值 =====
async function loadThresholds() {
  try {
    const res = await http.get(`/api/alert-settings?_=${Date.now()}`)
    thresholds.cpu = res.data.cpu ?? thresholds.cpu
    thresholds.mem = res.data.mem ?? thresholds.mem
    thresholds.disk = res.data.disk ?? thresholds.disk
    thresholds.ping_latency = res.data.ping_latency ?? thresholds.ping_latency
    thresholds.ping_loss = res.data.ping_loss ?? thresholds.ping_loss
    thresholds.offline_seconds = res.data.offline_seconds ?? thresholds.offline_seconds
    thresholds.metric_duration_seconds =
      res.data.metric_duration_seconds ?? thresholds.metric_duration_seconds
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

// ===== Ping 运营商目标 CRUD =====
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
  ispForm.scope = 'all'
  ispForm.agent_ids = []
}

function editISPTarget(row: any) {
  ispForm.id = row.id
  ispForm.name = row.name || ''
  ispForm.ip = row.ip || ''
  ispForm.port = row.port || 80
  ispForm.mode = row.mode || 'auto'
  ispForm.enabled = Boolean(row.enabled)
  // 回填作用域；后端旧数据没有 scope 字段时回退为全部节点
  ispForm.scope = row.scope === 'include' || row.scope === 'exclude' ? row.scope : 'all'
  ispForm.agent_ids = Array.isArray(row.agent_ids) ? row.agent_ids.slice() : []
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
      scope: ispForm.scope,
      // all 模式下后端会忽略该列表，这里统一清洗避免残留
      agent_ids: ispForm.scope === 'all' ? [] : ispForm.agent_ids,
    }
    if (ispForm.scope !== 'all' && payload.agent_ids.length === 0) {
      ispSaving.value = false
      ElMessage.warning('请先选择至少一个节点')
      return
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

// ===== 挂载：读取全部配置 =====
onMounted(async () => {
  // 主题从后端读取（含 primary，登录态走 /api/theme 才能拿到完整字段）
  try {
    const res = await http.get(`/api/theme?_=${Date.now()}`)
    if (res.data.primary) themeForm.primary = res.data.primary
    if (res.data.title) themeForm.title = res.data.title
    if (res.data.footer_text) themeForm.footer_text = res.data.footer_text
    themeForm.site_domain = res.data.site_domain || ''
    themeForm.agent_server_addr = res.data.agent_server_addr || ''
    // 预设卡片以“当前实际生效的主题”为准：
    // 访问设置页不应该把用户刚在顶栏切的浅色又强制改回站点默认（无头浏览器实测发现会互相覆盖）
    themeForm.preset = theme.state.preset
    // 只同步标题/页脚/主色，不动 preset；主色必须重新注入，刷新后自定义色才会生效
    theme.state.title = themeForm.title
    theme.state.footer = themeForm.footer_text
    theme.setPrimary(themeForm.primary)
  } catch {}

  await Promise.all([
    loadTelegram(),
    loadPushplus(),
    loadThresholds(),
    loadISPTargets(),
    loadUpgradeSettings(),
    // 作用域选择需要节点列表，与其他配置并行拉取
    loadAgentOptions(),
  ])
})
</script>

<style scoped>
/* 设置页布局：左导航 220px + 右内容自适应 */
.wk-settings {
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
  gap: var(--wk-space-5);
  align-items: start;
}

.wk-settings-nav {
  position: sticky;
  top: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wk-space-4);
  padding: var(--wk-space-3);
  background: var(--wk-panel);
  border: 1px solid var(--wk-border);
  border-radius: var(--wk-radius-lg);
}

.wk-set-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 7px 10px;
  border: none;
  border-radius: var(--wk-radius-sm);
  background: transparent;
  color: var(--wk-text-secondary);
  font-size: var(--wk-fs-base);
  font-weight: 500;
  text-align: left;
  transition: background var(--wk-dur-fast) var(--wk-ease),
    color var(--wk-dur-fast) var(--wk-ease);
}

.wk-set-item:hover {
  background: color-mix(in srgb, var(--wk-text) 5%, transparent);
  color: var(--wk-text);
}

.wk-set-item.active {
  background: var(--wk-primary-soft);
  color: var(--wk-text);
  font-weight: 600;
}

.wk-set-ico {
  width: 17px;
  height: 17px;
  flex: 0 0 17px;
  display: inline-flex;
  color: var(--wk-primary);
}

.wk-set-ico :deep(svg) {
  width: 17px;
  height: 17px;
}

/* 表单双列栅格：窄屏自动单列 */
.wk-set-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--wk-space-4) var(--wk-space-5);
}

.wk-set-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.wk-set-field .wk-eyebrow {
  text-transform: none;
  letter-spacing: 0;
  font-size: var(--wk-fs-base);
  color: var(--wk-text-secondary);
}

.form-tip {
  font-size: var(--wk-fs-sm);
  color: var(--wk-text-muted);
  line-height: 1.5;
}

.wk-set-actions {
  display: flex;
  gap: var(--wk-space-2);
  margin-top: var(--wk-space-5);
  padding-top: var(--wk-space-4);
  border-top: 1px solid var(--wk-border);
}

.wk-narrow {
  max-width: 420px;
}

/* 预设选择卡 */
.wk-preset-picker {
  display: flex;
  gap: var(--wk-space-3);
}

.wk-preset {
  flex: 1;
  min-width: 118px;
  padding: var(--wk-space-3);
  border: 1px solid var(--wk-border);
  border-radius: var(--wk-radius-md);
  background: var(--wk-bg-recess);
  color: var(--wk-text-secondary);
  text-align: left;
  transition: border-color var(--wk-dur-fast) var(--wk-ease);
}

.wk-preset:hover {
  border-color: var(--wk-border-strong);
}

.wk-preset.active {
  border-color: var(--wk-primary);
  box-shadow: var(--wk-ring);
  color: var(--wk-text);
  font-weight: 600;
}

.wk-preset-swatch {
  display: block;
  width: 100%;
  height: 34px;
  border: 1px solid;
  border-radius: var(--wk-radius-xs);
  margin-bottom: 8px;
}

.wk-preset-label {
  font-size: var(--wk-fs-sm);
}

/* 主色板 */
.wk-swatches {
  display: flex;
  align-items: center;
  gap: var(--wk-space-2);
  flex-wrap: wrap;
}

.wk-swatch {
  width: 26px;
  height: 26px;
  border-radius: var(--wk-radius-sm);
  border: 2px solid transparent;
  transition: transform var(--wk-dur-fast) var(--wk-ease),
    border-color var(--wk-dur-fast) var(--wk-ease);
}

.wk-swatch:hover {
  transform: scale(1.08);
}

.wk-swatch.active {
  border-color: var(--wk-text);
}

/* 步骤列表 */
.wk-steps {
  display: flex;
  flex-direction: column;
  gap: var(--wk-space-2);
  margin: 0 0 var(--wk-space-4);
  padding-left: 18px;
  color: var(--wk-text-secondary);
  font-size: var(--wk-fs-base);
  line-height: 1.7;
}

/* 阈值滑杆行 */
.wk-slider-group {
  display: flex;
  flex-direction: column;
  gap: var(--wk-space-2);
  margin-top: var(--wk-space-5);
}

.wk-slider-row {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr);
  align-items: center;
  gap: var(--wk-space-4);
}

.wk-slider-label {
  font-size: var(--wk-fs-base);
  color: var(--wk-text-secondary);
}

/* ISP 内联表单：换行留白 */
.wk-isp-form {
  margin-bottom: var(--wk-space-2);
}

/* 作用域区：标签 + 下拉 + 节点多选 + 一键按钮同一行，窄屏自动换行 */
.wk-isp-scope {
  padding: var(--wk-space-3) 0 var(--wk-space-1);
  border-top: 1px dashed var(--wk-border);
  margin-top: var(--wk-space-2);
}

.wk-isp-scope-row {
  display: flex;
  align-items: center;
  gap: var(--wk-space-2);
  flex-wrap: wrap;
}

.wk-isp-scope-label {
  font-size: var(--wk-fs-base);
  font-weight: 500;
  color: var(--wk-text-secondary);
  width: 52px;
  flex-shrink: 0;
}

/* 下拉选项右侧的 IPv6/IPv4 标记：靠右弱化显示 */
.wk-opt-v6 {
  float: right;
  margin-left: var(--wk-space-3);
  font-size: var(--wk-fs-xs);
  color: var(--wk-text-muted);
}

.wk-isp-form :deep(.el-form-item) {
  margin-bottom: var(--wk-space-3);
}

@media (max-width: 1100px) {
  .wk-settings {
    grid-template-columns: 1fr;
  }

  .wk-settings-nav {
    position: static;
    flex-direction: row;
    flex-wrap: wrap;
  }
}

@media (max-width: 768px) {
  .wk-set-grid {
    grid-template-columns: 1fr;
  }

  .wk-slider-row {
    grid-template-columns: 1fr;
    gap: var(--wk-space-1);
  }
}
</style>
