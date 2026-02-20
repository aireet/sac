<template>
  <n-config-provider :theme="darkTheme">
    <div class="auth-page">
      <div class="auth-card">
        <div class="logo-wrapper">
          <img :src="sacLogo" alt="SAC" class="logo" />
        </div>
        <p class="subtitle">Claude Code for Everyone</p>

        <n-form ref="formRef" :model="form" :rules="rules" @submit.prevent="handleLogin">
          <n-form-item path="username" label="Username or Email">
            <n-input v-model:value="form.username" placeholder="Enter username or email" size="large" />
          </n-form-item>

          <n-form-item path="password" label="Password">
            <n-input
              v-model:value="form.password"
              type="password"
              show-password-on="click"
              placeholder="Enter password"
              size="large"
              @keyup.enter="handleLogin"
            />
          </n-form-item>

          <n-button
            type="primary"
            block
            size="large"
            :loading="loading"
            @click="handleLogin"
            style="margin-top: 8px"
          >
            Sign In
          </n-button>
        </n-form>

        <div class="auth-footer" v-if="registrationOpen">
          <n-text depth="3">Don't have an account?</n-text>
          <router-link to="/register">
            <n-button text type="primary">Sign Up</n-button>
          </router-link>
        </div>
      </div>
    </div>
  </n-config-provider>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  NConfigProvider,
  NForm,
  NFormItem,
  NInput,
  NButton,
  NText,
  darkTheme,
  useMessage,
} from 'naive-ui'
import { useAuthStore } from '../stores/auth'
import api from '../services/api'
import sacLogo from '../assets/sac-logo.svg'
import { extractApiError } from '../utils/error'

const router = useRouter()
const authStore = useAuthStore()
const message = useMessage()

const formRef = ref()
const loading = ref(false)
const registrationOpen = ref(true)

const form = ref({
  username: '',
  password: '',
})

const rules = {
  username: { required: true, message: 'Please enter username or email', trigger: 'blur' },
  password: { required: true, message: 'Please enter password', trigger: 'blur' },
}

onMounted(async () => {
  try {
    const resp = await api.get('/auth/registration-mode')
    registrationOpen.value = resp.data.mode === 'open'
  } catch {
    registrationOpen.value = false
  }
})

const handleLogin = async () => {
  await formRef.value?.validate()
  loading.value = true
  try {
    await authStore.login(form.value.username, form.value.password)
    router.push('/')
  } catch (error) {
    message.error(extractApiError(error, 'Login failed'))
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.auth-page {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #18181c;
}

.auth-card {
  width: 400px;
  padding: 48px 40px;
  background: #242428;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.logo-wrapper {
  text-align: center;
  margin: 0 0 4px;
}

.logo {
  height: 40px;
}

.subtitle {
  text-align: center;
  color: #888;
  font-size: 14px;
  margin: 0 0 32px;
}

.auth-footer {
  margin-top: 24px;
  text-align: center;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}
</style>
