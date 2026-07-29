import { createApp } from 'vue'
import { createPinia } from 'pinia'
import TDesign, { MessagePlugin } from 'tdesign-vue-next'
import 'tdesign-vue-next/es/style/index.css'
import './styles/app.less'
import App from './App.vue'
import router from './router'
import { setApiErrorInterceptor } from './api'
import { createProfileErrorInterceptor } from './profileRecovery'
import { useProfilesStore } from './stores/profiles'
import { useSystemStore } from './stores/system'
import { useUiPreferencesStore } from './stores/uiPreferences'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia).use(router).use(TDesign)
const profiles = useProfilesStore(pinia)
const system = useSystemStore(pinia)
setApiErrorInterceptor(createProfileErrorInterceptor(profiles, router, (message) => MessagePlugin.warning(message)))
router.beforeEach((to) => {
  if (to.meta.requiresProfile && system.ready && !profiles.items.length) return '/settings'
})
useUiPreferencesStore(pinia).hydrate()
app.mount('#app')
