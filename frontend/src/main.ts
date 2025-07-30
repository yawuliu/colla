import { createApp } from "vue";
import { createPinia } from "pinia";

import { Button } from 'ant-design-vue';
import App from "./App.vue";
import router from "./router";
import i18n from "./i18n";

import "./style.scss";
import 'ant-design-vue/dist/reset.css';

// import "./assets/main.css";

const app = createApp(App);

app.use(createPinia());
app.use(router);
app.use(i18n);
app.use(Button);

app.mount("#app");
