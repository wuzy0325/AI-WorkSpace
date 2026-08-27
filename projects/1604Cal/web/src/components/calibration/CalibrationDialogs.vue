<template>
  <!-- 报告模板选择对话框 -->
  <el-dialog
    v-model="showTemplateDialog"
    title="选择报告模板"
    width="380px"
  >
    <el-form label-width="70px">
      <el-form-item label="测点数">
        <el-input-number
          v-model="templatePoints"
          :min="2"
          :max="5"
        />
      </el-form-item>
      <el-form-item label="模式">
        <el-select v-model="templateMode">
          <el-option
            label="单程"
            value="single"
          />
          <el-option
            label="回程"
            value="return"
          />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="showTemplateDialog = false">
        取消
      </el-button>
      <el-button
        type="primary"
        @click="confirmTemplate"
      >
        确定
      </el-button>
    </template>
  </el-dialog>

  <div
    v-if="errorMessage"
    class="error-message"
  >
    <el-icon><Warning /></el-icon>
    {{ errorMessage }}
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Warning } from '@element-plus/icons-vue'
import { selectReportTemplate } from '@/api/calibration'

const showTemplateDialog = ref(false)
const templatePoints = ref(5)
const templateMode = ref<'single' | 'return'>('single')
const templateFilename = ref('')
const errorMessage = ref('')

function openTemplateDialog() {
  showTemplateDialog.value = true
}

async function confirmTemplate() {
  errorMessage.value = ''
  try {
    const result = await selectReportTemplate(templatePoints.value, templateMode.value)
    templateFilename.value = result.filename
    showTemplateDialog.value = false
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '模板选择失败'
  }
}

defineExpose({
  openTemplateDialog,
  templateFilename,
})
</script>

<style scoped lang="scss">
.error-message {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  color: var(--status-error);
  font-size: 13px;
  padding: var(--spacing-sm);
  background: var(--status-error-bg-subtle);
  border-radius: var(--radius-sm);
  flex-shrink: 0;

  .el-icon {
    font-size: 14px;
  }
}
</style>
