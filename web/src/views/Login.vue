<template>
  <!-- 登录页 -->
  <div class="login-container">
    <div class="login-card">
      <div class="login-header">
        <h1>🐒 wukong 监控</h1>
        <p>管理员登录</p>
      </div>
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-position="top"
        @keyup.enter="handleLogin"
      >
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" placeholder="输入管理员用户名" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input
            v-model="form.password"
            type="password"
            placeholder="输入密码"
            show-password
          />
        </el-form-item>
        <el-form-item label="二步验证码 (TOTP)" prop="totpCode">
          <el-input v-model="form.totpCode" placeholder="如已启用则填写" />
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            size="large"
            :loading="loading"
            style="width: 100%"
            @click="handleLogin"
          >
            登录
          </el-button>
        </el-form-item>
      </el-form>
      <p v-if="error" class="login-error">{{ error }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import http from '@/utils/http'

const router = useRouter()
const route = useRoute()

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
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard'
    router.push(redirect)
  } catch (e: any) {
    error.value = e.response?.data?.error || '登录失败'
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-container {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: var(--wk-bg);
}

.login-card {
  background: var(--wk-panel-solid);
  border: 1px solid var(--wk-border);
  border-radius: 12px;
  padding: 40px;
  width: 400px;
  max-width: 90vw;
}

.login-header {
  text-align: center;
  margin-bottom: 32px;

  h1 {
    color: var(--wk-primary);
    font-size: 24px;
    margin-bottom: 8px;
  }

  p {
    color: var(--wk-text-muted);
    font-size: 14px;
  }
}

.login-error {
  color: var(--wk-danger);
  text-align: center;
  margin-top: 16px;
  font-size: 14px;
}
</style>