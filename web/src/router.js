import { createRouter, createWebHashHistory } from 'vue-router'
import { store } from './store.js'
import LoginView from './views/LoginView.vue'
import ArtifactsView from './views/ArtifactsView.vue'
import MyLicensesView from './views/MyLicensesView.vue'
import ReviewView from './views/ReviewView.vue'
import PublicationsView from './views/PublicationsView.vue'
import EmailsView from './views/EmailsView.vue'

const routes = [
  { path: '/', redirect: '/artifacts' },
  { path: '/login', component: LoginView, meta: { public: true } },
  { path: '/artifacts', component: ArtifactsView },
  { path: '/my', component: MyLicensesView },
  { path: '/review', component: ReviewView, meta: { curator: true } },
  { path: '/publications', component: PublicationsView },
  { path: '/emails', component: EmailsView }
]

export const router = createRouter({
  history: createWebHashHistory(),
  routes
})

router.beforeEach((to) => {
  if (to.meta.public) return true
  if (!store.token) return '/login'
  if (to.meta.curator && !store.isCurator) return '/artifacts'
  return true
})
