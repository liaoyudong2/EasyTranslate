import { Call, Dialogs } from "/wails/runtime.js";

const stateText = document.querySelector("#stateText");
const stateSub = document.querySelector("#stateSub");
const toggleBtn = document.querySelector("#toggleBtn");
const openBtn = document.querySelector("#openBtn");
const saveBtn = document.querySelector("#saveBtn");
const pickDirBtn = document.querySelector("#pickDirBtn");
const urlList = document.querySelector("#urlList");
const accessCode = document.querySelector("#accessCode");
const qrImage = document.querySelector("#qrImage");
const portInput = document.querySelector("#portInput");
const dirInput = document.querySelector("#dirInput");
const codeInput = document.querySelector("#codeInput");
const autoStartInput = document.querySelector("#autoStartInput");
const rememberCodeInput = document.querySelector("#rememberCodeInput");
const toast = document.querySelector("#toast");

let snapshot = null;

toggleBtn.addEventListener("click", async () => {
  try {
    if (snapshot?.status?.running) {
      await call("StopService");
    } else {
      await saveConfig(false);
      await call("StartService");
    }
    await refresh();
  } catch (error) {
    showToast(error.message || "操作失败");
    await refresh();
  }
});

openBtn.addEventListener("click", async () => {
  try {
    await call("OpenBrowser");
  } catch (error) {
    showToast(error.message || "无法打开浏览器");
  }
});

saveBtn.addEventListener("click", () => saveConfig(true));

pickDirBtn.addEventListener("click", async () => {
  try {
    const directory = await call("PrepareDirectoryPicker", dirInput.value.trim());
    const selected = await Dialogs.OpenFile({
      Title: "选择保存目录",
      Message: "上传文件会保存到这个目录",
      ButtonText: "选择",
      Directory: directory || undefined,
      CanChooseDirectories: true,
      CanChooseFiles: false,
      CanCreateDirectories: true,
      ResolvesAliases: true,
    });
    if (!selected) {
      return;
    }
    dirInput.value = Array.isArray(selected) ? selected[0] : selected;
    await saveConfig(true);
  } catch (error) {
    showToast(error.message || "选择目录失败");
  }
});

async function refresh() {
  snapshot = await call("Snapshot");
  render(snapshot);
}

function render(data) {
  const { config, status } = data;
  const running = Boolean(status.running);
  stateText.textContent = status.message || (running ? "监听中" : "未启动");
  stateText.classList.toggle("danger", !running && status.message && status.message !== "未启动");
  stateSub.textContent = running ? `文件保存到 ${status.storageDir}` : "点击启动后，其他设备可通过浏览器访问。";
  toggleBtn.textContent = running ? "停止服务" : "启动服务";
  openBtn.disabled = !running;

  portInput.value = config.port || status.port || 8080;
  dirInput.value = config.storageDir || status.storageDir || "";
  codeInput.value = config.accessCode || status.accessCode || "";
  autoStartInput.checked = Boolean(config.autoStart);
  rememberCodeInput.checked = Boolean(config.rememberCode);
  accessCode.textContent = status.accessCode || "------";

  urlList.innerHTML = "";
  const urls = status.urls || [];
  if (urls.length === 0) {
    urlList.textContent = "服务启动后会显示访问地址";
  } else {
    for (const url of urls) {
      const item = document.createElement("div");
      item.className = "url-item";
      item.innerHTML = `<code></code><button class="ghost" type="button">复制</button>`;
      item.querySelector("code").textContent = url;
      item.querySelector("button").addEventListener("click", () => copyText(url));
      urlList.appendChild(item);
    }
  }

  if (running && urls.length > 0) {
    qrImage.src = `${urls[0]}/api/share/qr.png?url=${encodeURIComponent(urls[0])}`;
    qrImage.classList.remove("hidden");
  } else {
    qrImage.classList.add("hidden");
  }
}

async function saveConfig(showMessage) {
  const cfg = {
    port: Number(portInput.value || 8080),
    storageDir: dirInput.value.trim(),
    accessCode: codeInput.value.trim(),
    autoStart: autoStartInput.checked,
    rememberCode: rememberCodeInput.checked,
  };
  await call("SaveConfig", cfg);
  if (showMessage) {
    showToast("已保存");
  }
  await refresh();
}

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    showToast("已复制");
  } catch {
    showToast(text);
  }
}

async function call(method, ...args) {
  if (window.go?.desktop?.App?.[method]) {
    return window.go.desktop.App[method](...args);
  }
  return Call.ByName(`lan-transfer/internal/desktop.App.${method}`, ...args);
}

function showToast(message) {
  toast.textContent = message;
  toast.classList.remove("hidden");
  window.clearTimeout(showToast.timer);
  showToast.timer = window.setTimeout(() => toast.classList.add("hidden"), 2400);
}

refresh().catch((error) => showToast(error.message || "初始化失败"));
