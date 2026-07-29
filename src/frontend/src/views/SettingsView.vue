<script setup lang="ts">
import { nextTick, reactive, ref } from 'vue'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import type { FormInstanceFunctions, FormRules } from 'tdesign-vue-next'
import { Pencil, Plus, PlugZap, Trash2 } from '@lucide/vue'
import ConnectionTestDialog from '../components/ConnectionTestDialog.vue'
import InterfaceSettings from '../components/InterfaceSettings.vue'
import PlayStationIcon from '../components/PlayStationIcon.vue'
import { notifyError } from '../errorFeedback'
import { useProfilesStore } from '../stores/profiles'
import { useSystemStore } from '../stores/system'
import type { Profile } from '../types'

const profiles = useProfilesStore()
const system = useSystemStore()
const visible = ref(false)
const testVisible = ref(false)
const testingProfile = ref<Profile | null>(null)
const form = reactive<Partial<Profile>>({ name:'', host:'', port:2120, username:'anonymous', password:'', base_path:'/', preset:'zftpd' })
const profileForm = ref<FormInstanceFunctions>()
const profileRules: FormRules = {
  name: [
    { required: true, whitespace: true, message: '请输入配置名称', trigger: 'blur' },
    { max: 100, message: '名称不能超过 100 个字符', trigger: 'blur' },
  ],
  preset: [{ required: true, enum: ['zftpd', 'ftpsrv', 'custom'], message: '请选择服务器预设', trigger: 'change' }],
  host: [
    { required: true, whitespace: true, message: '请输入 IP 或主机名', trigger: 'blur' },
    { max: 255, message: '主机名不能超过 255 个字符', trigger: 'blur' },
    {
      validator: (value) => {
        if (typeof value !== 'string' || /[\s/\\[\]]/.test(value)) return false
        if (!value.includes(':')) return !value.includes('%')
        return value.split(':').length > 2 && /^[0-9a-f:.]+(?:%[a-z0-9._-]+)?$/i.test(value)
      },
      message: '请输入不带协议和端口的 IP 或主机名',
      trigger: 'blur',
    },
  ],
  port: [
    { required: true, message: '请输入端口', trigger: 'change' },
    { number: true, min: 1, max: 65535, message: '端口必须在 1 到 65535 之间', trigger: 'change' },
  ],
  username: [{ max: 255, message: '用户名不能超过 255 个字符', trigger: 'blur' }],
  password: [{ max: 1024, message: '密码不能超过 1024 个字符', trigger: 'blur' }],
  base_path: [
    { required: true, whitespace: true, message: '请输入基础目录', trigger: 'blur' },
    { max: 4096, message: '基础目录不能超过 4096 个字符', trigger: 'blur' },
    { validator: (value) => typeof value === 'string' && !value.replaceAll('\\', '/').split('/').includes('..'), message: '基础目录不能包含 .. 路径段', trigger: 'blur' },
  ],
}

function edit(profile?: Profile) {
  Object.assign(form, { id:'', name:'', host:'', port:2120, username:'anonymous', password:'', base_path:'/', preset:'zftpd' }, profile || {})
  form.password = ''
  visible.value = true
  void nextTick(() => profileForm.value?.clearValidate())
}
function presetChanged(value: string) { if (value === 'zftpd') form.port = 2120; if (value === 'ftpsrv') form.port = 2121 }
async function save() {
  if (await profileForm.value?.validate() !== true) return
  try { await profiles.save(form); visible.value = false; await MessagePlugin.success('PS5 配置已保存') }
  catch (error) { await notifyError(error) }
}
async function remove(profile: Profile) {
  const dialog = DialogPlugin.confirm({ header:'删除 PS5 配置？', body:`将删除 ${profile.name} 及其任务记录。`, onConfirm:async()=>{ await profiles.remove(profile.id); dialog.destroy() } })
}
function test(profile: Profile) {
  testingProfile.value = profile
  testVisible.value = true
}
</script>

<template>
  <div class="settings-page">
    <InterfaceSettings />

    <section class="settings-section">
      <header><h2>PS5 连接</h2><p>保存常用 PS5 的 FTP 地址和访问目录</p></header>
      <div class="settings-body">
        <div class="settings-action"><div><strong>连接配置</strong><small>支持 zftpd、ftpsrv 和自定义端口</small></div><t-button theme="primary" variant="outline" @click="edit()"><Plus :size="15" />添加 PS5</t-button></div>
        <div v-if="profiles.items.length" class="profile-list">
          <div v-for="profile in profiles.items" :key="profile.id" class="profile-row">
            <span class="profile-avatar"><PlayStationIcon :size="16" /></span>
            <div class="profile-info"><strong>{{ profile.name }}</strong><small>{{ profile.host }}:{{ profile.port }} · {{ profile.base_path }}</small></div>
            <span class="profile-type">{{ profile.preset }}</span>
            <div class="row-actions"><t-button variant="text" size="small" @click="test(profile)"><PlugZap :size="14" />测试连接</t-button><t-button variant="text" size="small" @click="edit(profile)"><Pencil :size="14" />编辑</t-button><t-button variant="text" theme="danger" size="small" @click="remove(profile)"><Trash2 :size="14" />删除</t-button></div>
          </div>
        </div>
        <div v-else class="plain-empty compact">尚未添加 PS5，添加后才能创建传输任务</div>
      </div>
    </section>

    <section class="settings-section">
      <header><h2>传输性能</h2><p>控制单个任务同时上传的文件数量</p></header>
      <div class="settings-body"><div class="settings-action"><div><strong>并行文件连接</strong><small>同一台 PS5 始终只运行一个任务；多台 PS5 可并行</small></div><span class="worker-value">{{ system.transferWorkers }} 路</span></div><t-slider v-model="system.transferWorkers" :min="1" :max="4" :marks="{1:'1',2:'2（推荐）',3:'3',4:'4'}" @change-end="system.saveWorkers(system.transferWorkers)" /></div>
    </section>

    <section class="settings-section">
      <header><h2>访问范围</h2><p>应用服务的网络安全说明</p></header>
      <div class="settings-body"><div class="security-notice"><strong>仅在可信局域网中使用</strong><p>应用不提供登录验证，并拥有 NAS 文件读取权限。请勿将 8100 端口暴露到互联网。</p></div></div>
    </section>
  </div>

  <t-dialog v-model:visible="visible" header="PS5 FTP 配置" confirm-btn="保存" width="620px" @confirm="save">
    <t-form ref="profileForm" :data="form" :rules="profileRules" required-mark label-align="top">
      <div class="form-grid">
        <t-form-item name="name" label="名称"><t-input v-model="form.name" placeholder="例如：客厅 PS5" /></t-form-item>
        <t-form-item name="preset" label="服务器预设"><t-select v-model="form.preset" :options="[{label:'zftpd',value:'zftpd'},{label:'ftpsrv',value:'ftpsrv'},{label:'自定义',value:'custom'}]" @change="presetChanged" /></t-form-item>
        <t-form-item name="host" label="IP / 主机名"><t-input v-model="form.host" placeholder="192.168.1.50" /></t-form-item>
        <t-form-item name="port" label="端口"><t-input-number v-model="form.port" :min="1" :max="65535" /></t-form-item>
        <t-form-item name="username" label="用户名"><t-input v-model="form.username" /></t-form-item>
        <t-form-item name="password" label="密码"><t-input v-model="form.password" type="password" placeholder="留空则保持原密码" /></t-form-item>
        <t-form-item class="span-two" name="base_path" label="基础目录"><t-input v-model="form.base_path" placeholder="/" /></t-form-item>
      </div>
    </t-form>
  </t-dialog>
  <ConnectionTestDialog v-model:visible="testVisible" :profile="testingProfile" />
</template>
