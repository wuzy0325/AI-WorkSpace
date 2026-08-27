<template>
  <PageLayout>
    <header class="integrations-header">
      <div class="header-nav">
        <button
          class="back-btn"
          @click="goBack"
        >
          <el-icon><ArrowLeft /></el-icon>
        </button>
      </div>
      <div class="header-identity">
        <h1 class="header-title">
          连接工具
        </h1>
        <span class="header-sub">集成第三方服务，实现工作流自动化与双向同步</span>
      </div>
    </header>

    <div class="content-wrap">
      <!-- 关键集成 -->
      <section class="section">
        <div class="section-head">
          <h2>关键集成</h2>
          <p>将常用工具连接到系统，减少手动更新，保持数据同步</p>
        </div>
        <div class="integration-grid">
          <div
            v-for="card in keyIntegrations"
            :key="card.title"
            class="integration-card"
          >
            <div
              class="card-icon"
              :style="{ background: card.bg, color: card.color }"
            >
              <el-icon :size="24">
                <component :is="card.icon" />
              </el-icon>
            </div>
            <div class="card-body">
              <h3>{{ card.title }}</h3>
              <p>{{ card.description }}</p>
            </div>
            <div class="card-foot">
              <span class="card-action">{{ card.action }}</span>
              <el-icon><ArrowRight /></el-icon>
            </div>
          </div>
        </div>
      </section>

      <!-- 浏览所有集成 -->
      <section class="section">
        <div class="section-head">
          <h2>浏览所有集成</h2>
          <p>发现 150+ 可用连接</p>
        </div>
        <div
          class="directory-card"
          @click="openDirectory"
        >
          <div class="directory-icon">
            <el-icon :size="32">
              <Grid />
            </el-icon>
          </div>
          <div class="directory-body">
            <h3>集成目录</h3>
            <p>从技术支持工具（Intercom、Zendesk）创建缺陷，到设计探索（Figma）自动创建 Issue，一站式连接。</p>
          </div>
          <div class="directory-action">
            <el-icon><ArrowRight /></el-icon>
          </div>
        </div>
      </section>

      <!-- API -->
      <section class="section">
        <div class="section-head">
          <h2>Linear API</h2>
          <p>如果需要更自定义的集成，可以直接基于 Linear API（GraphQL）构建</p>
        </div>
        <div
          class="api-card"
          @click="openApiDocs"
        >
          <div class="api-icon">
            <el-icon :size="28">
              <Monitor />
            </el-icon>
          </div>
          <div class="api-body">
            <h3>开发者文档</h3>
            <p>查阅 Linear API 文档，了解如何构建自定义集成和自动化工作流。</p>
          </div>
          <div class="api-action">
            <span>查看文档</span>
            <el-icon><ArrowRight /></el-icon>
          </div>
        </div>
      </section>
    </div>
  </PageLayout>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { ArrowLeft, ArrowRight, Grid, Monitor } from '@element-plus/icons-vue'
import PageLayout from '@/components/common/PageLayout.vue'

const router = useRouter()

interface IntegrationCard {
  title: string
  description: string
  icon: unknown
  action: string
  color: string
  bg: string
}

const keyIntegrations: IntegrationCard[] = [
  {
    title: 'Slack',
    description: '从 Slack 消息创建 Issue 并同步线程讨论',
    icon: 'ChatSquare',
    action: '配置 Slack',
    color: '#4a154b',
    bg: 'rgba(74, 21, 75, 0.08)'
  },
  {
    title: 'GitHub / GitLab',
    description: '自动化 PR 与提交工作流，双向同步 Issue 状态',
    icon: 'Share',
    action: '配置代码仓库',
    color: '#24292f',
    bg: 'rgba(36, 41, 47, 0.08)'
  },
  {
    title: 'Agents',
    description: '部署 AI 代理作为团队成员，协同处理工作任务',
    icon: 'MagicStick',
    action: '配置代理',
    color: '#10b981',
    bg: 'rgba(16, 185, 129, 0.08)'
  }
]

function goBack(): void {
  router.push('/')
}

function openDirectory(): void {
  window.open('https://linear.app/integrations', '_blank')
}

function openApiDocs(): void {
  window.open('https://linear.app/developers', '_blank')
}
</script>

<style scoped lang="scss">
$font-sans: 'DM Sans', 'SF Pro Display', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
$mint: #10b981;
$slate-50: #f9fafb;
$slate-100: #f3f4f6;
$slate-200: #e5e7eb;
$slate-300: #d1d5db;
$slate-400: #9ca3af;
$slate-500: #6b7280;
$slate-600: #4b5563;
$slate-700: #374151;
$slate-800: #1f2937;
$slate-900: #111827;

.integrations-header {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-shrink: 0;
  height: 56px;
  padding: 0 24px;
  background: $slate-50;
  border-bottom: 1px solid $slate-200;
  font-family: $font-sans;
}

.header-nav { display: flex; align-items: center; }

.back-btn {
  width: 32px; height: 32px;
  background: #fff;
  border: 1px solid $slate-200;
  border-radius: 8px;
  color: $slate-500;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  transition: background 0.15s ease, color 0.15s ease, border-color 0.15s ease;

  &:hover {
    background: #fff;
    color: $mint;
    border-color: $mint;
  }
}

.header-identity { display: flex; align-items: center; gap: 12px; }

.header-title {
  font-size: 20px;
  font-weight: 600;
  color: $slate-800;
  margin: 0;
  font-family: $font-sans;
}

.header-sub {
  font-size: 12px;
  color: $slate-400;
  font-weight: 400;
}

.content-wrap {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 24px 32px;
  display: flex;
  flex-direction: column;
  gap: 32px;
}

.section-head {
  margin-bottom: 16px;

  h2 {
    font-size: 16px;
    font-weight: 600;
    color: $slate-800;
    margin: 0 0 4px;
    font-family: $font-sans;
  }

  p {
    font-size: 13px;
    color: $slate-400;
    margin: 0;
    font-family: $font-sans;
  }
}

.integration-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}

.integration-card {
  background: #fff;
  border-radius: 10px;
  padding: 24px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06), 0 1px 2px rgba(0, 0, 0, 0.04);
  transition: all 0.2s ease;
  border: 1px solid transparent;

  &:hover {
    transform: translateY(-2px);
    box-shadow: 0 8px 16px rgba(0, 0, 0, 0.08);
    border-color: $slate-200;

    .card-action {
      color: $mint;
      gap: 8px;
    }
  }
}

.card-icon {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.card-body {
  flex: 1;

  h3 {
    font-size: 15px;
    font-weight: 600;
    color: $slate-800;
    margin: 0 0 6px;
    font-family: $font-sans;
  }

  p {
    font-size: 13px;
    color: $slate-500;
    line-height: 1.5;
    margin: 0;
    font-family: $font-sans;
  }
}

.card-foot {
  display: flex;
  align-items: center;
  gap: 4px;
  font-family: $font-sans;
}

.card-action {
  font-size: 13px;
  font-weight: 500;
  color: $slate-400;
  transition: all 0.2s ease;
}

.card-foot .el-icon {
  font-size: 14px;
  color: $slate-300;
  transition: all 0.2s ease;
}

.integration-card:hover .card-foot .el-icon {
  color: $mint;
}

.directory-card {
  background: #fff;
  border-radius: 10px;
  padding: 24px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06), 0 1px 2px rgba(0, 0, 0, 0.04);
  transition: all 0.2s ease;
  border: 1px solid transparent;

  &:hover {
    transform: translateY(-2px);
    box-shadow: 0 8px 16px rgba(0, 0, 0, 0.08);
    border-color: $slate-200;
  }
}

.directory-icon {
  width: 56px;
  height: 56px;
  border-radius: 12px;
  background: rgba(99, 102, 241, 0.08);
  color: #6366f1;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.directory-body {
  flex: 1;

  h3 {
    font-size: 15px;
    font-weight: 600;
    color: $slate-800;
    margin: 0 0 4px;
    font-family: $font-sans;
  }

  p {
    font-size: 13px;
    color: $slate-500;
    line-height: 1.5;
    margin: 0;
    font-family: $font-sans;
  }
}

.directory-action {
  color: $slate-300;
  font-size: 18px;
  flex-shrink: 0;
}

.api-card {
  background: #fff;
  border-radius: 10px;
  padding: 24px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06), 0 1px 2px rgba(0, 0, 0, 0.04);
  transition: all 0.2s ease;
  border: 1px solid transparent;

  &:hover {
    transform: translateY(-2px);
    box-shadow: 0 8px 16px rgba(0, 0, 0, 0.08);
    border-color: $slate-200;

    .api-action { color: $mint; }
  }
}

.api-icon {
  width: 56px;
  height: 56px;
  border-radius: 12px;
  background: rgba(16, 185, 129, 0.08);
  color: $mint;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.api-body {
  flex: 1;

  h3 {
    font-size: 15px;
    font-weight: 600;
    color: $slate-800;
    margin: 0 0 4px;
    font-family: $font-sans;
  }

  p {
    font-size: 13px;
    color: $slate-500;
    line-height: 1.5;
    margin: 0;
    font-family: $font-sans;
  }
}

.api-action {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 500;
  color: $slate-400;
  flex-shrink: 0;
  transition: color 0.2s ease;
  font-family: $font-sans;
}

@media (max-width: 860px) {
  .content-wrap { padding: 20px 24px; }
  .integration-grid { grid-template-columns: 1fr; }
}
</style>
