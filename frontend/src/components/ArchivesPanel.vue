<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { TaskService } from "../../bindings/github.com/equals-chan/ElectricHamster";
import type { ArchiveView } from "../../bindings/github.com/equals-chan/ElectricHamster";

const props = defineProps<{ refreshKey?: number }>();

const archives = ref<ArchiveView[]>([]);
const message = ref("");
const isError = ref(false);

async function load() {
  archives.value = (await TaskService.Archives("")) ?? [];
}

function notify(text: string, err = false) {
  message.value = text;
  isError.value = err;
}

async function verify(a: ArchiveView) {
  notify(`校验中：${a.folderName}…`);
  try {
    await TaskService.Verify(a.archivePath, a.passwordId);
    notify(`校验通过：${a.folderName}`);
  } catch (e: any) {
    notify(`校验失败 ${a.folderName}: ${String(e?.message ?? e)}`, true);
  }
}

async function extract(a: ArchiveView) {
  try {
    const dir = await TaskService.PickDirectory("选择解压目录");
    if (!dir) return;
    notify(`解压中：${a.folderName}…`);
    await TaskService.Extract(a.archivePath, dir, a.passwordId);
    notify(`已解压到 ${dir}`);
  } catch (e: any) {
    notify(`解压失败：${String(e?.message ?? e)}`, true);
  }
}

async function exportExcel() {
  try {
    const stamp = new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-");
    let path = await TaskService.PickSaveFile("导出 Excel", `passwords_${stamp}.xlsx`);
    if (!path) return;
    if (!path.toLowerCase().endsWith(".xlsx")) path += ".xlsx";
    notify("导出中…");
    const n = await TaskService.ExportExcel("", path);
    notify(`已导出 ${n} 条记录到 ${path}`);
  } catch (e: any) {
    notify(`导出失败：${String(e?.message ?? e)}`, true);
  }
}

function fmtBytes(n: number): string {
  if (!n) return "0 B";
  const u = ["B", "KB", "MB", "GB", "TB"];
  let i = 0, v = n;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${u[i]}`;
}

onMounted(load);
watch(() => props.refreshKey, load);
</script>

<template>
  <div class="panel">
    <div class="row" style="justify-content: space-between">
      <h2>归档记录</h2>
      <div class="row" style="gap: 8px">
        <button class="btn primary" @click="exportExcel">导出 Excel</button>
        <button class="btn" @click="load">刷新</button>
      </div>
    </div>

    <div v-if="message" class="badge" :class="isError ? 'err' : 'ok'" style="display: block; margin: 10px 0">
      {{ message }}
    </div>

    <table v-if="archives.length" style="margin-top: 8px">
      <thead>
        <tr><th>序号</th><th>文件夹</th><th>大小</th><th>文件数</th><th>时间</th><th>状态</th><th></th></tr>
      </thead>
      <tbody>
        <tr v-for="a in archives" :key="a.id">
          <td class="mono">{{ a.seq }}</td>
          <td>
            <div>{{ a.folderName }}</div>
            <div class="mono small muted">{{ a.archivePath }}</div>
          </td>
          <td>{{ fmtBytes(a.size) }}</td>
          <td>{{ a.fileCount }}</td>
          <td class="muted small">{{ a.createdAt }}</td>
          <td><span class="badge" :class="a.status === 'ok' ? 'ok' : 'err'">{{ a.status }}</span></td>
          <td class="row" style="gap: 6px">
            <button class="btn icon" @click="verify(a)">校验</button>
            <button class="btn icon" @click="extract(a)">解压</button>
          </td>
        </tr>
      </tbody>
    </table>
    <div v-else class="empty">暂无归档记录</div>
  </div>
</template>
