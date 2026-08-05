<template>
  <!-- 登录页根容器：使用全局 .wk-login-wrap 类，自带径向渐变背景与居中布局 -->
  <div class="wk-login-wrap">
    <!-- 主题切换按钮：固定在右上角，深色/浅色双主题切换 -->
    <button
      type="button"
      class="wk-icon-btn wk-theme-toggle"
      @click="toggleTheme"
      title="切换浅色/深色"
      aria-label="切换主题"
    >
      <!-- 深色模式下显示太阳图标（点击切换到浅色） -->
      <svg v-if="isDark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <circle cx="12" cy="12" r="5" /><path d="M12 1v3M12 20v3M4.22 4.22l2.12 2.12M17.66 17.66l2.12 2.12M1 12h3M20 12h3M4.22 19.78l2.12-2.12M17.66 6.34l2.12-2.12" />
      </svg>
      <!-- 浅色模式下显示月亮图标（点击切换到深色） -->
      <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
      </svg>
    </button>

    <!-- 登录卡片：使用全局 .wk-login-card 类，圆角卡片 + 阴影 -->
    <div class="wk-login-card">
      <!-- 品牌标志区：.wk-brand-mark 显示"悟"字方块 + "wukong 监控"文字 -->
      <div class="wk-login-brand">
        <span class="wk-brand-mark">悟</span>
        <span>wukong 监控</span>
      </div>
      <!-- 副标题：管理员登录提示 -->
      <div class="wk-login-sub">管理员登录</div>

      <!-- 登录表单：使用 Element Plus el-form，回车键提交 -->
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-position="top"
        @keyup.enter="handleLogin"
      >
        <!-- 用户名输入项 -->
        <el-form-item label="用户名" prop="username">
          <el-input
            v-model="form.username"
            placeholder="输入管理员用户名"
            :prefix-icon="UserIcon"
            size="large"
          />
        </el-form-item>

        <!-- 密码输入项：支持显示/隐藏密码 -->
        <el-form-item label="密码" prop="password">
          <el-input
            v-model="form.password"
            type="password"
            placeholder="输入密码"
            show-password
            :prefix-icon="LockIcon"
            size="large"
          />
        </el-form-item>

        <!-- TOTP 二步验证码：已启用二步验证时填写 -->
        <el-form-item label="二步验证码 (TOTP)" prop="totpCode">
          <el-input
            v-model="form.totpCode"
            placeholder="如已启用则填写"
            :prefix-icon="ShieldIcon"
            size="large"
          />
        </el-form-item>

        <!-- 登录按钮：全宽，加载中状态禁用重复点击 -->
        <el-form-item>
          <el-button
            type="primary"
            size="large"
            :loading="loading"
            class="wk-login-btn"
            @click="handleLogin"
          >
            登录
          </el-button>
        </el-form-item>
      </el-form>

      <!-- 错误信息提示：登录失败时展示后端返回的错误 -->
      <p v-if="error" class="wk-login-error">{{ error }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
// 引入 Vue 响应式 API
import { ref, reactive, onMounted } from 'vue'
// 引入路由，用于登录成功后跳转
import { useRoute, useRouter } from 'vue-router'
// 引入 Element Plus 消息提示组件
import { ElMessage } from 'element-plus'
// 引入 Element Plus 图标组件，用作输入框前缀图标
import { User, Lock, Key } from '@element-plus/icons-vue'
// 引入全局 axios 实例（自动附加 JWT、401 自动续期）
import http from '@/utils/http'

// 图标实例：传给 el-input 的 prefix-icon 属性
const UserIcon = User       // 用户名输入框图标
const LockIcon = Lock       // 密码输入框图标
const ShieldIcon = Key      // TOTP 验证码输入框图标

// 路由实例，用于页面跳转
const router = useRouter()
// 当前路由信息，用于读取 redirect 查询参数
const route = useRoute()

// 表单引用，用于调用 validate 校验
const formRef = ref()
// 登录请求中状态，控制按钮 loading
const loading = ref(false)
// 错误信息，登录失败时展示
const error = ref('')

// 主题状态：默认深色（与 MainLayout 保持一致，读取 localStorage）
const isDark = ref(localStorage.getItem('theme') !== 'light')

// 登录表单数据模型
const form = reactive({
  username: '',   // 用户名
  password: '',   // 密码
  totpCode: '',   // TOTP 二步验证码（可选）
})

// 表单校验规则：用户名和密码必填
const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

// 切换深色/浅色主题
function toggleTheme() {
  // 取反当前主题状态
  isDark.value = !isDark.value
  // 计算目标主题名称
  const theme = isDark.value ? 'dark' : 'light'
  // 设置 html 根元素的 data-theme 属性，驱动 CSS 变量切换
  document.documentElement.dataset.theme = theme
  // 同步切换 dark 类（兼容 Element Plus 深色模式）
  document.documentElement.classList.toggle('dark', theme === 'dark')
  // 持久化到 localStorage，刷新后保持主题
  localStorage.setItem('theme', theme)
}

// 处理登录提交
async function handleLogin() {
  // 先做表单校验，校验失败则中断
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  // 进入加载状态，清空之前的错误
  loading.value = true
  error.value = ''

  try {
    // 调用后端登录接口，提交用户名、密码、TOTP 验证码
    const res = await http.post('/api/auth/login', {
      username: form.username,
      password: form.password,
      totp_code: form.totpCode,
    })
    // 登录成功，保存 access token 和 refresh token 到本地存储
    localStorage.setItem('access_token', res.data.access_token)
    localStorage.setItem('refresh_token', res.data.refresh_token)
    // 提示登录成功
    ElMessage.success('登录成功')
    // 读取 redirect 参数，有则跳转目标页，否则跳转仪表盘
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard'
    router.push(redirect)
  } catch (e: any) {
    // 登录失败，展示后端返回的错误信息，无则显示默认失败提示
    error.value = e.response?.data?.error || '登录失败'
  } finally {
    // 无论成功失败，关闭加载状态
    loading.value = false
  }
}

// 组件挂载时初始化主题（登录页独立于 MainLayout，需自行设置根元素主题属性）
onMounted(() => {
  // 读取本地保存的主题，默认深色
  const savedTheme = localStorage.getItem('theme') || 'dark'
  // 设置根元素 data-theme 属性
  document.documentElement.dataset.theme = savedTheme
  // 同步 dark 类
  document.documentElement.classList.toggle('dark', savedTheme === 'dark')
  // 同步 isDark 响应式状态
  isDark.value = savedTheme === 'dark'
})
</script>

<style scoped>
/* 主题切换按钮：固定在页面右上角 */
.wk-theme-toggle {
  position: fixed;
  top: 20px;
  right: 20px;
  z-index: 100;
}

/* 登录按钮：占满表单宽度 */
.wk-login-btn {
  width: 100%;
}

/* 错误信息样式：红色居中提示 */
.wk-login-error {
  color: var(--wk-danger);
  text-align: center;
  margin-top: 12px;
  font-size: 13px;
}

/* 覆盖 Element Plus 表单项间距，使登录卡片更紧凑 */
:deep(.el-form-item) {
  margin-bottom: 18px;
}

/* 覆盖 Element Plus 标签字号与颜色，贴合设计令牌 */
:deep(.el-form-item__label) {
  font-size: 13px;
  font-weight: 600;
  color: var(--wk-text);
  padding-bottom: 4px;
}

/* 输入框圆角与背景适配双主题 */
:deep(.el-input__wrapper) {
  border-radius: 8px;
}
</style>
