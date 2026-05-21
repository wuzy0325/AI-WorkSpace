import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { useThemeStore } from './stores/themeStore'
import './styles.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
useThemeStore().initializeTheme()
app.mount('#app')
