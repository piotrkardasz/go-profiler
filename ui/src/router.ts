import { createRouter, createWebHistory } from 'vue-router'
import ProfileList from './views/ProfileList.vue'
import ProfileDetail from './views/ProfileDetail.vue'
import ProfileCompare from './views/ProfileCompare.vue'

const routes = [
  {
    path: '/_profiler/',
    name: 'profiles',
    component: ProfileList,
  },
  {
    path: '/_profiler/profile/:id',
    name: 'profile-detail',
    component: ProfileDetail,
    props: true,
  },
  {
    path: '/_profiler/compare/:idA/:idB',
    name: 'profile-compare',
    component: ProfileCompare,
    props: true,
  },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})
