<template>
  <n-config-provider :theme="darkTheme">
    <div class="auth-page">
      <div class="auth-card">
        <div class="logo-wrapper">
          <img :src="sacLogo" alt="SAC" class="logo" />
        </div>
        <p class="subtitle">Create your account</p>

        <n-form ref="formRef" :model="form" :rules="rules" @submit.prevent="handleRegister">
          <n-form-item path="username" label="Username">
            <n-input v-model:value="form.username" placeholder="Choose a username" size="large" />
          </n-form-item>

          <n-form-item path="email" label="Email">
            <n-input v-model:value="form.email" placeholder="Enter email address" size="large" />
          </n-form-item>

          <n-form-item path="display_name" label="Display Name">
            <n-input v-model:value="form.display_name" placeholder="Your display name (optional)" size="large" />
          </n-form-item>

          <n-form-item path="password" label="Password">
            <n-input
              v-model:value="form.password"
              type="password"
              show-password-on="click"
              placeholder="At least 6 characters"
              size="large"
            />
          </n-form-item>

          <n-form-item path="confirm_password" label="Confirm Password">
            <n-input
              v-model:value="form.confirm_password"
              type="password"
              show-password-on="click"
              placeholder="Re-enter password"
              size="large"
              @keyup.enter="handleRegister"
            />
          </n-form-item>

          <n-button
            type="primary"
            block
            size="large"
            :loading="loading"
            @click="handleRegister"
            style="margin-top: 8px"
          >
            Create Account
          </n-button>
        </n-form>

        <div class="auth-footer">
          <n-text depth="3">Already have an account?</n-text>
          <router-link to="/login">
            <n-button text type="primary">Sign In</n-button>
          </router-link>
        </div>
      </div>
    </div>
  </n-config-provider>
</template>

<script setup lang="ts">
import { ref } from 'vue'
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
import sacLogo from '../assets/sac-logo.svg'
import { extractApiError } from '../utils/error'

const router = useRouter()
const authStore = useAuthStore()
const message = useMessage()

const formRef = ref()
const loading = ref(false)

const form = ref({
  username: '',
  email: '',
  display_name: '',
  password: '',
  confirm_password: '',
})

const rules = {
  username: { required: true, message: 'Please enter a username', trigger: 'blur' },
  email: [
    { required: true, message: 'Please enter an email', trigger: 'blur' },
    { type: 'email' as const, message: 'Please enter a valid email', trigger: 'blur' },
  ],
  password: [
    { required: true, message: 'Please enter a password', trigger: 'blur' },
    { min: 6, message: 'Password must be at least 6 characters', trigger: 'blur' },
  ],
  confirm_password: [
    { required: true, message: 'Please confirm your password', trigger: 'blur' },
    {
      validator(_rule: any, value: string) {
        if (value !== form.value.password) {
          return new Error('Passwords do not match')
        }
        return true
      },
      trigger: 'blur',
    },
  ],
}

const handleRegister = async () => {
  await formRef.value?.validate()
  loading.value = true
  try {
    await authStore.register({
      username: form.value.username,
      email: form.value.email,
      password: form.value.password,
      display_name: form.value.display_name || undefined,
    })
    message.success('Account created successfully!')
    router.push('/')
  } catch (error) {
    message.error(extractApiError(error, 'Registration failed'))
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
