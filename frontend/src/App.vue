<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Events } from "@wailsio/runtime";
import { TaskService, VaultService } from "../bindings/github.com/equals-chan/ElectricHamster";
import type { ToolStatus, VaultStatus } from "../bindings/github.com/equals-chan/ElectricHamster";
import VaultPanel from "./components/VaultPanel.vue";
import TasksPanel from "./components/TasksPanel.vue";
import ArchivesPanel from "./components/ArchivesPanel.vue";
interface RunEvent {
  kind: string;
  taskId: string;
  folder: string;
  index: number;
  total: number;
  phase: string;
  percent: number;
  overallPercent: number;
  current: string;
  archivePath: string;
  error: string;
  done: number;
  skipped: number;
  failed: number;
  bytesIn: number;
  bytesOut: number;
  elapsedMs: number;
}

type JobStatus = "queued" | "running" | "done" | "failed" | "skipped";
interface JobState {
  folder: string;
  status: JobStatus;
  percent: number;
}

type Tab = "tasks" | "archives" | "vault";

const tab = ref<Tab>("tasks");
const vault = ref<VaultStatus>({ path: "", exists: false, unlocked: false });
const tool = ref<ToolStatus>({ available: false, path: "", version: "", codecs: [], error: "" });
const appVersion = ref("");
const running = ref(false);
const paused = ref(false);
const refreshKey = ref(0);

const overall = ref(0);
const stats = ref({ done: 0, skipped: 0, failed: 0 });
const jobs = ref<JobState[]>([]);
const log = ref<{ kind: string; text: string }[]>([]);

const sortedJobs = computed(() =>
  [...jobs.value].sort((a, b) => {
    const rank = (s: JobStatus) => (s === "running" ? 0 : s === "queued" ? 1 : s === "failed" ? 2 : s === "done" ? 3 : 4);
    const r = rank(a.status) - rank(b.status);
    return r !== 0 ? r : a.folder.localeCompare(b.folder);
  }),
);

function addLog(kind: string, text: string) {
  log.value.push({ kind, text });
  if (log.value.length > 300) log.value.splice(0, log.value.length - 300);
}

function upsertJob(folder: string, patch: Partial<JobState>) {
  const existing = jobs.value.find((j) => j.folder === folder);
  if (existing) Object.assign(existing, patch);
  else jobs.value.push({ folder, status: "queued", percent: 0, ...patch });
}

function fmtBytes(n: number): string {
  if (!n) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

async function refreshVault() {
  vault.value = await VaultService.Status();
}

async function refreshTool() {
  tool.value = await TaskService.ToolStatus();
}

function onRunEvent(e: RunEvent) {
  switch (e.kind) {
    case "job:started":
      running.value = true;
      paused.value = false;
      overall.value = 0;
      stats.value = { done: 0, skipped: 0, failed: 0 };
      jobs.value = [];
      addLog("d", `开始运行：共 ${e.total} 个文件夹`);
      break;
    case "job:paused":
      paused.value = true;
      addLog("s", "已暂停（当前文件夹完成后停止派发）");
      break;
    case "job:resumed":
      paused.value = false;
      addLog("d", "已继续");
      break;
    case "job:item_started":
      upsertJob(e.folder, { status: "running", percent: 0 });
      break;
    case "job:progress":
      overall.value = e.overallPercent;
      upsertJob(e.folder, { status: "running", percent: e.percent });
      break;
    case "job:item_skipped":
      stats.value.skipped++;
      upsertJob(e.folder, { status: "skipped", percent: 100 });
      break;
    case "job:item_done":
      if (e.error) {
        stats.value.failed++;
        upsertJob(e.folder, { status: "failed", percent: 100 });
        addLog("e", `失败 ${e.folder}: ${e.error}`);
      } else {
        stats.value.done++;
        upsertJob(e.folder, { status: "done", percent: 100 });
      }
      break;
    case "job:finished":
      running.value = false;
      paused.value = false;
      overall.value = 100;
      stats.value = { done: e.done, skipped: e.skipped, failed: e.failed };
      addLog(
        "d",
        `结束：完成 ${e.done}，跳过 ${e.skipped}，失败 ${e.failed}；` +
          `输入 ${fmtBytes(e.bytesIn)} / 输出 ${fmtBytes(e.bytesOut)}，耗时 ${e.elapsedMs} ms`,
      );
      refreshKey.value++;
      break;
    case "job:error":
      running.value = false;
      addLog("e", `运行错误：${e.error}`);
      break;
  }
}

onMounted(async () => {
  await Promise.all([refreshVault(), refreshTool()]);
  appVersion.value = await TaskService.AppVersion();
  Events.On("job:event", (ev: any) => onRunEvent(ev.data as RunEvent));
  Events.On("vault:changed", () => refreshVault());
});
</script>

<template>
  <div class="app">
    <header class="topbar">
      <div class="brand">Electric<span>Hamster</span><span class="ver" v-if="appVersion"> v{{ appVersion }}</span></div>
      <div class="grow"></div>
      <span class="pill" :class="tool.available ? 'ok' : 'err'">
        <i class="dot"></i>7-Zip {{ tool.available ? tool.version || "就绪" : "不可用" }}
      </span>
      <span class="pill" :class="vault.unlocked ? 'ok' : vault.exists ? 'warn' : 'err'">
        <i class="dot"></i>密码库 {{ vault.unlocked ? "已解锁" : vault.exists ? "已锁定" : "未创建" }}
      </span>
    </header>

    <nav class="tabs">
      <button :class="{ active: tab === 'tasks' }" @click="tab = 'tasks'">任务</button>
      <button :class="{ active: tab === 'archives' }" @click="tab = 'archives'">归档</button>
      <button :class="{ active: tab === 'vault' }" @click="tab = 'vault'">密码库</button>
    </nav>

    <main>
      <div v-show="running || jobs.length || log.length" class="panel" style="margin-bottom: 16px">
        <div class="row" style="justify-content: space-between">
          <h2>
            运行状态
            <span v-if="running" class="badge" :class="paused ? 'warn' : 'ok'">{{ paused ? "已暂停" : "进行中" }}</span>
          </h2>
          <div v-if="running" class="row" style="gap: 8px">
            <button v-if="!paused" class="btn" @click="TaskService.Pause()">暂停</button>
            <button v-else class="btn primary" @click="TaskService.Resume()">继续</button>
            <button class="btn danger" @click="TaskService.Cancel()">取消</button>
          </div>
        </div>

        <div class="row small muted" style="justify-content: space-between; margin-bottom: 6px">
          <span>整体进度</span>
          <span>{{ overall.toFixed(1) }}%</span>
        </div>
        <div class="progressbar"><i :style="{ width: overall + '%' }"></i></div>
        <div class="row small muted" style="margin-top: 8px; justify-content: space-between">
          <span>完成 {{ stats.done }} / 跳过 {{ stats.skipped }} / 失败 {{ stats.failed }}</span>
        </div>

        <div v-if="jobs.length" class="joblist">
          <div v-for="j in sortedJobs" :key="j.folder" class="job">
            <span class="badge" :class="j.status === 'failed' ? 'err' : j.status === 'done' || j.status === 'running' ? 'ok' : ''">
              {{ j.status }}
            </span>
            <span class="mono ellipsis">{{ j.folder }}</span>
            <span class="jobbar"><i :style="{ width: (j.status === 'running' ? j.percent : 100) + '%' }"></i></span>
            <span class="small muted">{{ j.status === "running" ? j.percent.toFixed(0) + "%" : "" }}</span>
          </div>
        </div>

        <div v-if="log.length" class="log" style="margin-top: 10px">
          <div v-for="(l, i) in log.slice(-100)" :key="i" :class="l.kind">{{ l.text }}</div>
        </div>
      </div>

      <TasksPanel v-show="tab === 'tasks'" :refresh-key="refreshKey" />
      <ArchivesPanel v-show="tab === 'archives'" :refresh-key="refreshKey" />
      <VaultPanel v-show="tab === 'vault'" />
    </main>
  </div>
</template>
