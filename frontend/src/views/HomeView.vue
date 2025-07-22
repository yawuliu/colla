<script setup lang="ts">
import { ref, onMounted, nextTick } from "vue";
import {
  GetDialogs,
  GetMessages,
  SendMessage,
  DeleteDialog,
} from "../../wailsjs/go/main/App";
import type { main } from "../../wailsjs/go/models";

// 使用时
type Dialog = main.Dialog;
type Message = main.Message;

/* ---------- 状态 ---------- */
const dialogs = ref<Dialog[]>([]);
const messages = ref<Message[]>([]);
const input = ref("");
const currentDID = ref<number>(0);

/* DOM 引用 */
const chatBox = ref<HTMLDivElement>();

/* ---------- 逻辑 ---------- */
const refreshDialogs = async () => {
  dialogs.value = await GetDialogs();
};

const loadDialog = async (id: number) => {
  currentDID.value = id;
  messages.value = await GetMessages(id);
  scrollBottom();
};

const newDialog = () => {
  currentDID.value = 0;
  messages.value = [];
};

const send = async () => {
  const text = input.value.trim();
  if (!text) return;

  const resp = await SendMessage(currentDID.value, text);
  currentDID.value = resp.newDid;
  messages.value = await GetMessages(resp.newDid);
  refreshDialogs();
  input.value = "";
  scrollBottom();
};

const deleteDialog = async (id: number) => {
  await DeleteDialog(id);
  refreshDialogs();
  if (id === currentDID.value) newDialog();
};

const scrollBottom = () =>
  nextTick(() => {
    if (chatBox.value) {
      chatBox.value.scrollTop = chatBox.value.scrollHeight;
    }
  });

/* ---------- 生命周期 ---------- */
onMounted(() => {
  refreshDialogs();
});
</script>

<template>
<div class="flex h-full bg-neutral-900 text-neutral-100">
        <!-- 左侧会话 -->
        <aside class="w-72 flex flex-col border-r border-neutral-700 ">
          <button
            @click="newDialog"
            class="h-12 px-4 bg-sky-600 hover:bg-sky-700 shrink-0"
          >
            新建会话
          </button>
          <ul class="flex-1 overflow-y-auto">
            <li
              v-for="d in dialogs"
              :key="d.ID"
              @click="loadDialog(d.ID)"
              @contextmenu.prevent="deleteDialog(d.ID)"
              :class="[
                'p-2 cursor-pointer truncate hover:bg-neutral-700',
                d.ID === currentDID && 'bg-neutral-600',
              ]"
            >
              {{ d.Title }}
            </li>
          </ul>
        </aside>

    <!-- 右侧聊天 -->
       <main class="flex flex-col h-full  w-full">
         <!-- 聊天内容 -->
         <section
           ref="chatBox"
           class="flex-1 overflow-y-auto p-4 bg-neutral-800 select-text"
         >
           <div v-for="(m, idx) in messages" :key="idx" class="mb-3">
             <div v-if="m.Role === 'user'" class="text-right">
               <span class="inline-block bg-sky-600 px-2 py-1 rounded">
                 {{ m.Content }}
               </span>
             </div>
             <div v-else>
               <span class="inline-block bg-neutral-600 px-2 py-1 rounded">
                 {{ m.Content }}
               </span>
             </div>
           </div>
         </section>

         <!-- 输入栏，固定高度 -->
         <footer class="h-22 srink-0 items-center px-2 bg-neutral-700 p-1">
           <textarea
             v-model="input"
             @keyup.enter="send"
             class="w-full h-20 px-3 rounded bg-neutral-900 text-neutral-100"
             placeholder="输入消息..."
             rows="5"
           ></textarea>
         </footer>
       </main>
  </div>
</template>

<style lang="scss">

</style>
