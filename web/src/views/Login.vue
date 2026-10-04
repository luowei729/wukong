<template>
  <!-- ============ 登录页：左品牌区 + 右表单（窄屏折叠为单列） ============ -->
  <div class="wk-login-wrap">
    <!-- 主题切换：固定在右上角 -->
    <button
      type="button"
      class="wk-icon-btn wk-theme-toggle"
      :title="isDark ? '切换到浅色主题' : '切换到深色主题'"
      aria-label="切换主题"
      @click="toggleTheme"
    >
      <svg v-if="isDark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
        <circle cx="12" cy="12" r="4" />
        <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
      </svg>
      <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
        <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
      </svg>
    </button>

    <!-- ---------------- 左侧品牌区 ---------------- -->
    <aside class="wk-login-brand-side">
      <div class="wk-row" style="gap: 10px">
        <span class="wk-brand-mark" style="width: 32px; height: 32px; font-size: 15px">悟</span>
        <strong style="font-size: var(--wk-fs-md)">{{ title }}</strong>
      </div>

      <div>
        <div class="wk-login-brand-title">
          服务器状态<br />一眼看清
        </div>
        <p class="wk-login-brand-desc">
          秒级采集 CPU / 内存 / 磁盘 / 网络与运营商链路质量，异常自动告警到 Telegram。
        </p>
      </div>

      <!-- 三条能力要点：说明后台能做什么，比堆装饰更符合 SaaS 登录页习惯 -->
      <ul class="wk-login-points">
        <li v-for="point in points" :key="point">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M20 6L9 17l-5-5" />
          </svg>
          {{ point }}
        </li>
      </ul>
    </aside>

    <!-- ---------------- 右侧表单区 ---------------- -->
    <main class="wk-login-form-side">
      <div class="wk-login-card">
        <div class="wk-login-title">
          <h2>管理员登录</h2>
          <p>请输入账号密码进入监控后台</p>
        </div>

        <el-form
          ref="formRef"
          :model="form"
          :rules="rules"
          label-position="top"
          @keyup.enter="handleLogin"
        >
          <el-form-item label="用户名" prop="username">
            <el-input
              v-model="form.username"
              placeholder="输入管理员用户名"
              :prefix-icon="UserIcon"
              size="large"
              autocomplete="username"
            />
          </el-form-item>

          <el-form-item label="密码" prop="password">
            <el-input
              v-model="form.password"
              type="password"
              placeholder="输入密码"
              show-password
              :prefix-icon="LockIcon"
              size="large"
              autocomplete="current-password"
            />
          </el-form-item>

          <!-- TOTP：已启用二步验证时填写，未启用可留空 -->
          <el-form-item label="二步验证码 (TOTP)" prop="totpCode">
            <el-input
              v-model="form.totpCode"
              placeholder="如已启用二步验证则填写"
              :prefix-icon="ShieldIcon"
              size="large"
              autocomplete="one-time-code"
            />
          </el-form-item>

          <el-form-item>
            <el-button
              type="primary"
              size="large"
              class="wk-login-btn"
              :loading="loading"
              @click="handleLogin"
            >
              登录
            </el-button>
          </el-form-item>
        </el-form>

        <!-- 错误提示：图标 + 底色块，比一行红字更容易被注意 -->
        <p v-if="error" class="wk-login-error" role="alert">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" style="width: 16px; height: 16px">
            <circle cx="12" cy="12" r="9" />
            <path d="M12 8v5M12 16h.01" />
          </svg>
          {{ error }}
        </p>

        <!-- 返回公开状态页：未登录用户也能查看服务器状态 -->
        <div class="wk-sub" style="text-align: center">
          只想看服务器状态？<a @click="router.push('/')">访问公开状态页</a>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
// ============ 登录页逻辑 ============
// 登录接口、token 存储与 redirect 跳转逻辑保持不变，本次仅重构展示层与主题获取方式
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { User, Lock, Key } from '@element-plus/icons-vue'
import http from '@/utils/http'
import { useTheme } from '@/composables/useTheme'

// 图标实例：传给 el-input 的 prefix-icon
const UserIcon = User
const LockIcon = Lock
const ShieldIcon = Key

const router = useRouter()
const route = useRoute()

// 主题与站点标题来自 useTheme 单例（不在本页重复实现 data-theme 逻辑）
const { isDark, toggleTheme, title, load: loadTheme } = useTheme()

// 品牌区要点：说明后台核心能力，避免堆装饰文案
const points = [
  '探针秒级上报，多节点下依旧稳定写入',
  '运营商 Ping 延时与丢包 24 小时曲线',
  '阈值告警 + Telegram 机器人推送',
]

const formRef = ref()
const loading = ref(false)
const error = ref('')

const form = reactive({
  username: '',
  password: '',
  totpCode: '',
})

const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

// 登录提交：保持原接口与字段（username / password / totp_code）
async function handleLogin() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  error.value = ''

  try {
    const res = await http.post('/api/auth/login', {
      username: form.username,
      password: form.password,
      totp_code: form.totpCode,
    })
    localStorage.setItem('access_token', res.data.access_token)
    localStorage.setItem('refresh_token', res.data.refresh_token)
    ElMessage.success('登录成功')
    const redirect =
      typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard'
    router.push(redirect)
  } catch (e: any) {
    error.value = e.response?.data?.error || '登录失败'
  } finally {
    loading.value = false
  }
}

// 挂载时拉取站点主题：登录页也要显示后台配置的站点名
onMounted(() => {
  loadTheme()
})
</script>

<style scoped>
/* 品牌文字与说明之间的呼吸感由全局令牌控制，这里只处理登录页专属细节 */
.wk-login-card :deep(.el-form-item) {
  margin-bottom: var(--wk-space-4);
}

.wk-login-card :deep(.el-form-item__label) {
  padding-bottom: 4px;
}

/* 公开状态页链接 */
.wk-login-card a {
  cursor: pointer;
  font-weight: 500;
}
</style>
