<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { TaskService } from "../../bindings/github.com/equals-chan/ElectricHamster";
import type { TaskInput, TaskView } from "../../bindings/github.com/equals-chan/ElectricHamster";

const props = defineProps<{ refreshKey?: number }>();

const tasks = ref<TaskView[]>([]);
const error = ref("");
const busy = ref(false);
const showEditor = ref(false);
const editing = ref<TaskInput>(blank());

function blank(): TaskInput {
  return {
    id: "",
    name: "",
    sourceDir: "",
    destDir: "",
    codec: "zstd",
    level: 15,
    encrypt: true,
    headerEncrypt: true,
    splitSize: 0,
    passwordMode: "random",
    passwordLength: 20,
    passwordFixed: "",
    dedupRule: "content_sig",
    namePrefix: "",
    nameSuffix: "",
    seqStart: 10000,
    include: "",
    exclude: "",
  };
}

async function load() {
  tasks.value = (await TaskService.List()) ?? [];
}

function newTask() {
  editing.value = blank();
  error.value = "";
  showEditor.value = true;
}

function edit(t: TaskView) {
  editing.value = {
    id: t.id,
    name: t.name,
    sourceDir: t.sourceDir,
    destDir: t.destDir,
    codec: t.codec || "zstd",
    level: t.level,
    encrypt: t.encrypt,
    headerEncrypt: t.headerEncrypt,
    splitSize: t.splitSize,
    passwordMode: t.passwordMode || "random",
    passwordLength: t.passwordLength || 20,
    passwordFixed: "",
    dedupRule: t.dedupRule || "content_sig",
    namePrefix: t.namePrefix || "",
    nameSuffix: t.nameSuffix || "",
    seqStart: t.seqStart || 10000,
    include: t.include,
    exclude: t.exclude,
  };
  error.value = "";
  showEditor.value = true;
}

async function pick(field: "sourceDir" | "destDir") {
  try {
    const dir = await TaskService.PickDirectory(field === "sourceDir" ? "选择源目录" : "选择输出目录");
    if (dir) editing.value[field] = dir;
  } catch (e: any) {
    error.value = String(e?.message ?? e);
  }
}

async function save() {
  error.value = "";
  busy.value = true;
  try {
    await TaskService.Save(editing.value);
    showEditor.value = false;
    await load();
  } catch (e: any) {
    error.value = String(e?.message ?? e);
  } finally {
    busy.value = false;
  }
}

async function remove(id: string) {
  if (!confirm("确定删除该任务？（归档记录会保留）")) return;
  await TaskService.Delete(id);
  await load();
}

async function run(id: string) {
  error.value = "";
  try {
    await TaskService.Run(id);
  } catch (e: any) {
    error.value = String(e?.message ?? e);
  }
}

async function importLegacy() {
  error.value = "";
  try {
    const cfg = await TaskService.PickLegacyConfig();
    if (!cfg) return;
    const summary = await TaskService.ImportLegacy(cfg);
    await load();
    error.value = summary;
  } catch (e: any) {
    error.value = String(e?.message ?? e);
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
      <h2>任务</h2>
      <div class="row" style="gap: 8px">
        <button class="btn" @click="importLegacy">导入旧数据</button>
        <button class="btn primary" @click="newTask">新建任务</button>
      </div>
    </div>

    <div v-if="error" class="badge err" style="display: block; margin: 10px 0">{{ error }}</div>

    <table v-if="tasks.length" style="margin-top: 8px">
      <thead>
        <tr><th>名称</th><th>源 → 输出</th><th>压缩</th><th>加密</th><th>命名</th><th>已归档</th><th></th></tr>
      </thead>
      <tbody>
        <tr v-for="t in tasks" :key="t.id">
          <td>{{ t.name }}</td>
          <td class="small muted">
            <div class="mono">{{ t.sourceDir }}</div>
            <div class="mono">→ {{ t.destDir }}</div>
          </td>
          <td><span class="badge">{{ t.codec }} · {{ t.level }}</span></td>
          <td>
            <span class="badge" :class="t.encrypt ? 'ok' : ''">{{ t.encrypt ? "AES-256" : "无" }}</span>
          </td>
          <td class="small mono muted">{{ t.namePrefix }}{{ t.seqStart }}{{ t.nameSuffix }}.7z</td>
          <td>{{ t.archiveCount }}</td>
          <td class="row" style="gap: 6px">
            <button class="btn icon primary" @click="run(t.id)">运行</button>
            <button class="btn icon" @click="edit(t)">编辑</button>
            <button class="btn icon danger" @click="remove(t.id)">删除</button>
          </td>
        </tr>
      </tbody>
    </table>
    <div v-else class="empty">还没有任务，点击「新建任务」开始。</div>

    <div v-if="showEditor" class="panel" style="margin-top: 16px; background: var(--panel-2)">
      <h3>{{ editing.id ? "编辑任务" : "新建任务" }}</h3>
      <div class="grid">
        <label class="field">任务名称 <input v-model="editing.name" placeholder="例如：动漫收藏" /></label>
        <label class="field">
          源目录
          <span class="row">
            <input v-model="editing.sourceDir" class="mono" />
            <button class="btn icon" @click="pick('sourceDir')">选择</button>
          </span>
        </label>
        <label class="field">
          输出目录
          <span class="row">
            <input v-model="editing.destDir" class="mono" />
            <button class="btn icon" @click="pick('destDir')">选择</button>
          </span>
        </label>
        <label class="field">压缩算法
          <select v-model="editing.codec">
            <option value="zstd">zstd</option>
            <option value="lzma2">lzma2</option>
          </select>
        </label>
        <label class="field">压缩等级 <input type="number" v-model.number="editing.level" min="0" max="22" /></label>
        <label class="field">分卷大小（字节，0=不分卷）<input type="number" v-model.number="editing.splitSize" min="0" /></label>
        <label class="row small" style="color: var(--muted); align-self: end">
          <input type="checkbox" v-model="editing.encrypt" /> 加密（AES-256）
        </label>
        <label class="row small" style="color: var(--muted); align-self: end">
          <input type="checkbox" v-model="editing.headerEncrypt" :disabled="!editing.encrypt" /> 加密文件名
        </label>
        <label class="field">密码策略
          <select v-model="editing.passwordMode" :disabled="!editing.encrypt">
            <option value="random">随机密码</option>
            <option value="fixed">固定密码</option>
          </select>
        </label>
        <label v-if="editing.passwordMode === 'random'" class="field">随机密码长度
          <input type="number" v-model.number="editing.passwordLength" min="8" max="128" :disabled="!editing.encrypt" />
        </label>
        <label v-if="editing.passwordMode === 'fixed'" class="field">固定密码
          <input v-model="editing.passwordFixed" :disabled="!editing.encrypt" />
        </label>
        <label class="field">判重方式
          <select v-model="editing.dedupRule">
            <option value="content_sig">内容签名（内容变化即重压）</option>
            <option value="folder_name">文件夹名（存档变动也跳过）</option>
            <option value="none">不判重（每次都压）</option>
          </select>
        </label>
        <label class="field">文件名前缀（可选）<input v-model="editing.namePrefix" placeholder="例如 game_" /></label>
        <label class="field">文件名后缀（可选）<input v-model="editing.nameSuffix" placeholder="例如 _01" /></label>
        <label class="field">起始编号 <input type="number" v-model.number="editing.seqStart" min="1" /></label>
        <label class="field" style="grid-column: 1 / -1">
          文件名预览：
          <span class="mono muted">{{ editing.namePrefix }}{{ editing.seqStart }}{{ editing.nameSuffix }}.7z</span>
          <span class="small muted">（每个任务独立编号）</span>
        </label>
        <label class="field">包含（glob，逗号分隔）<input v-model="editing.include" placeholder="留空=全部" /></label>
        <label class="field">排除（glob，逗号分隔）<input v-model="editing.exclude" placeholder="例如：temp*, *.bak" /></label>
      </div>
      <div class="row" style="margin-top: 14px">
        <button class="btn primary" :disabled="busy" @click="save">保存</button>
        <button class="btn" @click="showEditor = false">取消</button>
      </div>
    </div>
  </div>
</template>
