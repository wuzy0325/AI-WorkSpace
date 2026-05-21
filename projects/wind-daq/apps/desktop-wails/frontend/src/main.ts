import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { useThemeStore } from './stores/themeStore'
import './styles.css'

const app = createApp(App)
app.use(createPinia())
useThemeStore().initializeTheme()
app.mount('#app')
