import { createApp } from 'vue'
import { createPinia } from 'pinia'
import TDesign from 'tdesign-vue-next'
import 'tdesign-vue-next/es/style/index.css'
import './styles/app.less'
import App from './App.vue'
import router from './router'
import { useUiPreferencesStore } from './stores/uiPreferences'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia).use(router).use(TDesign)
useUiPreferencesStore(pinia).hydrate()
app.mount('#app')
