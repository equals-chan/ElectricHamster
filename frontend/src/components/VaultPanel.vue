<script setup lang="ts">
import { onMounted, ref } from "vue";
import { Events } from "@wailsio/runtime";
import { VaultService } from "../../bindings/github.com/equals-chan/ElectricHamster";
import type { ItemView, VaultStatus } from "../../bindings/github.com/equals-chan/ElectricHamster";

const status = ref<VaultStatus>({ path: "", exists: false, unlocked: false });
const items = ref<ItemView[]>([]);
const master = ref("");
const confirm = ref("");
const busy = ref(false);
const error = ref("");
const revealed = ref<Record<string, string>>({});
const genLength = ref(20);
const genSymbols = ref(false);
const generated = ref("");

const editingId = ref("");
const editLabel = ref("");
const editPassword = ref("");

async function load() {
  status.value = await VaultService.Status();
  if (status.value.unlocked) {
    items.value = (await VaultService.List()) ?? [];
  } else {
    items.value = [];
    revealed.value = {};
  }
}

async function create() {
  error.value = "";
  if (!master.value) return (error.value = "请输入主密码");
  if (master.value !== confirm.value) return (error.value = "两次输入不一致");
  busy.value = true;
  try {
    await VaultService.Create(master.value);
    master.value = confirm.value = "";
    await load();
  } catch (e: any) {
    error.value = String(e?.message ?? e);
  } finally {
    busy.value = false;
  }
}

async function unlock() {
  error.value = "";
  busy.value = true;
  try {
    await VaultService.Unlock(master.value);
    master.value = "";
    await load();
  } catch (e: any) {
    error.value = String(e?.message ?? e);
  } finally {
    busy.value = false;
  }
}

async function lock() {
  await VaultService.Lock();
  await load();
}

async function reveal(id: string) {
  if (revealed.value[id]) {
    const copy = { ...revealed.value };
    delete copy[id];
    revealed.value = copy;
    return;
  }
  try {
    revealed.value = { ...revealed.value, [id]: await VaultService.Reveal(id) };
  } catch (e: any) {
    error.value = String(e?.message ?? e);
  }
}

async function generate() {
  generated.value = await VaultService.Generate(genLength.value, genSymbols.value);
}

async function startEdit(it: ItemView) {
  error.value = "";
  editingId.value = it.id;
  editLabel.value = it.label;
  editPassword.value = revealed.value[it.id] ?? (await VaultService.Reveal(it.id));
}

function cancelEdit() {
  editingId.value = "";
  editLabel.value = "";
  editPassword.value = "";
}

async function saveEdit(it: ItemView) {
  error.value = "";
  if (!editPassword.value) return (error.value = "密码不能为空");
  try {
    await VaultService.Update(it.id, editLabel.value, editPassword.value);
    revealed.value = { ...revealed.value, [it.id]: editPassword.value };
    cancelEdit();
    await load();
  } catch (e: any) {
    error.value = String(e?.message ?? e);
  }
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    /* ignore */
  }
}

onMounted(() => {
  load();
  Events.On("vault:changed", () => load());
});
</script>

<template>
  <div class="panel">
    <h2>密码库</h2>
    <p class="small muted mono">{{ status.path }}</p>

    <div v-if="error" class="badge err" style="display: block; margin: 10px 0">{{ error }}</div>

    <div v-if="!status.exists" class="stack" style="max-width: 460px; margin-top: 12px">
      <p class="small muted">尚未创建密码库。设置一个主密码来加密保存所有归档密码。</p>
      <label class="field">主密码 <input type="password" v-model="master" /></label>
      <label class="field">确认主密码 <input type="password" v-model="confirm" /></label>
      <button class="btn primary" :disabled="busy" @click="create">创建密码库</button>
    </div>

    <div v-else-if="!status.unlocked" class="stack" style="max-width: 460px; margin-top: 12px">
      <p class="small muted">密码库已锁定，请输入主密码解锁。</p>
      <label class="field">主密码 <input type="password" v-model="master" @keyup.enter="unlock" /></label>
      <button class="btn primary" :disabled="busy" @click="unlock">解锁</button>
    </div>

    <div v-else>
      <div class="row" style="justify-content: space-between; margin-bottom: 12px">
        <h3 style="margin: 0">共 {{ items.length }} 条</h3>
        <button class="btn" @click="lock">锁定</button>
      </div>

      <table v-if="items.length">
        <thead>
          <tr><th>标签</th><th>密码</th><th>创建时间</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="it in items" :key="it.id">
            <td>
              <input v-if="editingId === it.id" v-model="editLabel" />
              <template v-else>{{ it.label }}</template>
            </td>
            <td class="mono">
              <input v-if="editingId === it.id" v-model="editPassword" />
              <template v-else-if="revealed[it.id]">{{ revealed[it.id] }}</template>
              <template v-else>••••••••</template>
            </td>
            <td class="muted small">{{ it.createdAt }}</td>
            <td class="row" style="gap: 6px">
              <template v-if="editingId === it.id">
                <button class="btn icon primary" @click="saveEdit(it)">保存</button>
                <button class="btn icon" @click="cancelEdit">取消</button>
              </template>
              <template v-else>
                <button class="btn icon" @click="reveal(it.id)">{{ revealed[it.id] ? "隐藏" : "显示" }}</button>
                <button v-if="revealed[it.id]" class="btn icon" @click="copy(revealed[it.id])">复制</button>
                <button class="btn icon" @click="startEdit(it)">改密</button>
              </template>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else class="empty">暂无条目</div>

      <div class="panel" style="margin-top: 16px; background: var(--panel-2)">
        <h3>密码生成器</h3>
        <div class="row">
          <label class="field" style="max-width: 120px">长度 <input type="number" v-model.number="genLength" min="4" max="128" /></label>
          <label class="row small" style="color: var(--muted)"><input type="checkbox" v-model="genSymbols" /> 含符号</label>
          <button class="btn" @click="generate">生成</button>
          <span v-if="generated" class="mono" style="margin-left: 8px">{{ generated }}</span>
          <button v-if="generated" class="btn icon" @click="copy(generated)">复制</button>
        </div>
      </div>
    </div>
  </div>
</template>
