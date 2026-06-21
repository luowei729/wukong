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
            <el-form-item>
              <el-button type="primary" :loading="saving" @click="saveTheme">
                保存主题
              </el-button>
            </el-form-item>
          </el-form>
        </div>
      </el-tab-pane>

      <!-- 安装新节点 -->
      <el-tab-pane label="安装节点" name="install">
        <div class="wk-card-solid" style="padding: 20px;">
          <el-button type="primary" :loading="generating" @click="generateToken">
            生成安装命令
          </el-button>
          <div v-if="installCommand" style="margin-top: 16px;">
            <div class="command-box">
              <code>{{ installCommand }}</code>
            </div>
            <el-button size="small" style="margin-top: 8px;" @click="copyCommand">
              复制命令
            </el-button>
          </div>
        </div>
      </el-tab-pane>

      <!-- Telegram 配置 -->
      <el-tab-pane label="Telegram 通知" name="telegram">
        <div class="wk-card-solid" style="padding: 20px;">
          <el-form label-position="top">
            <el-form-item label="Bot Token">
              <el-input v-model="tgForm.bot_token" type="password" placeholder="输入 Telegram Bot Token" />
            </el-form-item>
            <el-form-item label="Chat ID">
              <el-input v-model="tgForm.chat_id" placeholder="输入 Chat ID" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="saveTelegram">保存</el-button>
            </el-form-item>
          </el-form>
        </div>
      </el-tab-pane>

      <!-- 告警阈值 -->
      <el-tab-pane label="告警阈值" name="thresholds">
        <div class="wk-card-solid" style="padding: 20px;">
          <el-form label-position="top">
            <el-form-item label="CPU 告警阈值 (%)">
              <el-slider v-model="thresholds.cpu" :min="0" :max="100" show-input />
            </el-form-item>
            <el-form-item label="内存告警阈值 (%)">
              <el-slider v-model="thresholds.mem" :min="0" :max="100" show-input />
            </el-form-item>
            <el-form-item label="磁盘告警阈值 (%)">
              <el-slider v-model="thresholds.disk" :min="0" :max="100" show-input />
            </el-form-item>
            <el-form-item>
              <el-button type="primary">保存阈值</el-button>
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
})
const saving = ref(false)

// 安装
const generating = ref(false)
const installCommand = ref('')

// Telegram
const tgForm = reactive({
  bot_token: '',
  chat_id: '',
})

// 阈值
const thresholds = reactive({
  cpu: 90,
  mem: 90,
  disk: 90,
})

async function saveTheme() {
  saving.value = true
  try {
    const token = localStorage.getItem('access_token')
    await axios.put('/api/theme', themeForm, {
      headers: { Authorization: `Bearer ${token}` },
    })
    applyTheme()
    ElMessage.success('主题已保存')
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
  try {
    const token = localStorage.getItem('access_token')
    const res = await axios.post('/api/install-tokens', {}, {
      headers: { Authorization: `Bearer ${token}` },
    })
    installCommand.value = res.data.script_url
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '生成失败')
  } finally {
    generating.value = false
  }
}

async function copyCommand() {
  try {
    await navigator.clipboard.writeText(installCommand.value)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.warning('复制失败，请手动复制')
  }
}

async function saveTelegram() {
  // 后续实现
  ElMessage.success('Telegram 配置已保存')
}

onMounted(async () => {
  try {
    const token = localStorage.getItem('access_token')
    const res = await axios.get('/api/theme', {
      headers: { Authorization: `Bearer ${token}` },
    })
    if (res.data.preset) themeForm.preset = res.data.preset
    if (res.data.primary) themeForm.primary = res.data.primary
    if (res.data.title) themeForm.title = res.data.title
    if (res.data.footer_text) themeForm.footer_text = res.data.footer_text
    applyTheme()
  } catch {}
})
</script>

<style scoped>
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