import { createRouter, createWebHistory } from 'vue-router'
import TransferView from './views/TransferView.vue'
import TasksView from './views/TasksView.vue'
import PS5FilesView from './views/PS5FilesView.vue'
import SettingsView from './views/SettingsView.vue'

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: TransferView, meta: { title: '新建传输' } },
    { path: '/tasks', component: TasksView, meta: { title: '任务中心' } },
    { path: '/files', component: PS5FilesView, meta: { title: 'PS5 文件', requiresProfile: true } },
    { path: '/settings', component: SettingsView, meta: { title: '设置' } },
  ],
})
