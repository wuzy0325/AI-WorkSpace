<template>
  <PageLayout>
    <header class="instrument-header">
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
          导入数据
        </h1>
        <span class="header-sub">在系统间同步数据</span>
      </div>
    </header>

    <div class="content-scroll">
      <!-- 简介 -->
      <section class="hero">
        <div class="hero-icon">
          <el-icon class="hero-svg">
            <Upload />
          </el-icon>
        </div>
        <h2 class="hero-title">
          导入你的数据
        </h2>
        <p class="hero-desc">
          无论你是在探索 Cal1604 系统、运行试点项目，还是准备完成完整迁移，我们都能提供支持。
        </p>
      </section>

      <!-- 探索与评估 -->
      <section class="section-group">
        <h3 class="section-label">
          探索与评估
        </h3>

        <div class="card-grid">
          <div class="import-card">
            <div class="card-body">
              <div
                class="card-icon-box"
                style="--card-accent: #3b82f6"
              >
                <el-icon><Document /></el-icon>
              </div>
              <h4 class="card-title">
                了解标定系统
              </h4>
              <p class="card-desc">
                构建业务用例，建立组织认同，了解 Cal1604 系统功能全景。
              </p>
            </div>
            <div class="card-foot">
              <a
                class="card-link"
                href="#"
              >查看文档</a>
              <el-icon><ArrowRight /></el-icon>
            </div>
          </div>

          <div class="import-card">
            <div class="card-body">
              <div
                class="card-icon-box"
                style="--card-accent: #f59e0b"
              >
                <el-icon><Monitor /></el-icon>
              </div>
              <h4 class="card-title">
                运行试点项目
              </h4>
              <p class="card-desc">
                先在小团队中测试标定工作流，验证系统适配性后再推广至全部门。
              </p>
            </div>
            <div class="card-foot">
              <a
                class="card-link"
                href="#"
              >开始试点</a>
              <el-icon><ArrowRight /></el-icon>
            </div>
          </div>
        </div>
      </section>

      <!-- 数据迁移准备就绪 -->
      <section class="section-group">
        <h3 class="section-label">
          数据迁移准备就绪
        </h3>

        <div class="card-grid">
          <div class="import-card">
            <div class="card-body">
              <div
                class="card-icon-box"
                style="--card-accent: #10b981"
              >
                <el-icon><Files /></el-icon>
              </div>
              <h4 class="card-title">
                导入设备配置
              </h4>
              <p class="card-desc">
                从现有系统导入打压设备与计量设备配置，快速完成系统初始化。
              </p>
            </div>
            <div class="card-foot">
              <a
                class="card-link"
                href="#"
              >开始导入</a>
              <el-icon><ArrowRight /></el-icon>
            </div>
          </div>

          <div class="import-card">
            <div class="card-body">
              <div
                class="card-icon-box"
                style="--card-accent: #8b5cf6"
              >
                <el-icon><Reading /></el-icon>
              </div>
              <h4 class="card-title">
                导入标定模板
              </h4>
              <p class="card-desc">
                迁移标定流程模板、压力点配置与报告模板到新系统。
              </p>
            </div>
            <div class="card-foot">
              <a
                class="card-link"
                href="#"
              >开始导入</a>
              <el-icon><ArrowRight /></el-icon>
            </div>
          </div>

          <div class="import-card">
            <div class="card-body">
              <div
                class="card-icon-box"
                style="--card-accent: #ec4899"
              >
                <el-icon><FolderOpened /></el-icon>
              </div>
              <h4 class="card-title">
                从旧系统迁移
              </h4>
              <p class="card-desc">
                从 1604 旧版标定软件或历史会话数据中批量导入记录，保留完整审计追溯。
              </p>
            </div>
            <div class="card-foot">
              <a
                class="card-link"
                href="#"
              >迁移指南</a>
              <el-icon><ArrowRight /></el-icon>
            </div>
          </div>
        </div>
      </section>

      <!-- 技术支持 -->
      <section class="support-section">
        <div class="support-card">
          <div class="support-icon">
            <el-icon><ChatLineSquare /></el-icon>
          </div>
          <div class="support-body">
            <strong>遇到问题？</strong>
            按下键盘 <code>?</code> 键，选择「联系我们」获取技术支持。
          </div>
        </div>
      </section>
    </div>
  </PageLayout>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import {
  ArrowLeft, ArrowRight, Upload, Document, Monitor,
  Files, Reading, FolderOpened, ChatLineSquare
} from '@element-plus/icons-vue'
import PageLayout from '@/components/common/PageLayout.vue'

const router = useRouter()

function goBack(): void {
  router.push('/')
}
</script>

<style scoped lang="scss">
$font-sans: 'DM Sans', 'SF Pro Display', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
$font-mono: 'JetBrains Mono', 'Fira Code', 'SF Mono', Consolas, monospace;
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

/* ═══ 头部 ═══ */
.instrument-header {
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

/* ═══ 内容滚动区 ═══ */
.content-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 32px 40px 40px;
  font-family: $font-sans;
}

/* ═══ 简介 ═══ */
.hero {
  text-align: center;
  margin-bottom: 40px;
}

.hero-icon {
  width: 56px;
  height: 56px;
  margin: 0 auto 16px;
  background: rgba(16, 185, 129, 0.08);
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: $mint;

  .hero-svg {
    font-size: 26px;
  }
}

.hero-title {
  font-size: 24px;
  font-weight: 700;
  color: $slate-800;
  margin: 0 0 10px;
  letter-spacing: -0.01em;
}

.hero-desc {
  font-size: 14px;
  color: $slate-500;
  margin: 0 auto;
  max-width: 520px;
  line-height: 1.6;
}

/* ═══ 分组 ═══ */
.section-group {
  margin-bottom: 36px;
}

.section-label {
  font-size: 11px;
  font-weight: 600;
  color: $slate-400;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  margin: 0 0 16px;
  padding-left: 2px;
  font-family: $font-sans;
}

/* ═══ 卡片网格 ═══ */
.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
}

.import-card {
  background: #ffffff;
  border: 1px solid $slate-200;
  border-radius: 12px;
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);

  &:hover {
    border-color: rgba($mint, 0.3);
    box-shadow:
      0 10px 15px -3px rgba(0, 0, 0, 0.08),
      0 4px 6px -4px rgba(0, 0, 0, 0.05),
      0 0 20px rgba(16, 185, 129, 0.06);
    transform: translateY(-2px);
  }
}

.card-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.card-icon-box {
  width: 38px;
  height: 38px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  color: var(--card-accent);
  background: rgba(var(--card-accent), 0.08);
  transition: transform 0.25s ease;

  .import-card:hover & {
    transform: scale(1.08);
  }
}

.card-title {
  font-size: 15px;
  font-weight: 600;
  color: $slate-800;
  margin: 0;
}

.card-desc {
  font-size: 13px;
  color: $slate-500;
  line-height: 1.55;
  margin: 0;
}

.card-foot {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: auto;
}

.card-link {
  font-size: 13px;
  font-weight: 500;
  color: $slate-400;
  text-decoration: none;
  transition: color 0.2s;

  .import-card:hover & {
    color: var(--card-accent, $mint);
  }
}

.card-foot .el-icon {
  font-size: 14px;
  color: $slate-300;
  transition: color 0.2s;

  .import-card:hover & {
    color: var(--card-accent, $mint);
  }
}

/* ═══ 技术支持 ═══ */
.support-section {
  margin-top: 12px;
}

.support-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 18px;
  background: $slate-50;
  border: 1px solid $slate-200;
  border-radius: 10px;
  font-size: 13px;
  color: $slate-500;
  font-family: $font-sans;
  line-height: 1.5;

  strong {
    color: $slate-700;
    font-weight: 600;
  }

  code {
    font-family: $font-mono;
    font-size: 12px;
    background: #fff;
    border: 1px solid $slate-200;
    border-radius: 4px;
    padding: 1px 6px;
    color: $slate-700;
  }
}

.support-icon {
  color: $slate-400;
  font-size: 18px;
  flex-shrink: 0;
  display: flex;
}

/* ═══ 响应式 ═══ */
@media (max-width: 768px) {
  .instrument-header {
    height: auto; flex-wrap: wrap; padding: 10px 16px;
  }
  .header-sub { display: none; }
  .content-scroll { padding: 20px 16px 24px; }
  .card-grid { grid-template-columns: 1fr; }
  .hero-title { font-size: 20px; }
}
</style>
