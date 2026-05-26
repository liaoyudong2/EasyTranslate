const loginView = document.querySelector("#loginView");
const appView = document.querySelector("#appView");
const loginForm = document.querySelector("#loginForm");
const codeInput = document.querySelector("#codeInput");
const loginError = document.querySelector("#loginError");
const logoutBtn = document.querySelector("#logoutBtn");
const dropZone = document.querySelector("#dropZone");
const fileInput = document.querySelector("#fileInput");
const fileList = document.querySelector("#fileList");
const emptyState = document.querySelector("#emptyState");
const refreshBtn = document.querySelector("#refreshBtn");
const progressWrap = document.querySelector("#progressWrap");
const progressBar = document.querySelector("#progressBar");
const toast = document.querySelector("#toast");

loginForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  loginError.textContent = "";
  const code = codeInput.value.trim();
  const res = await fetch("/api/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code }),
  });
  if (!res.ok) {
    const data = await readJSON(res);
    loginError.textContent = data.error || "登录失败";
    return;
  }
  showApp();
  await loadFiles();
});

logoutBtn.addEventListener("click", async () => {
  await fetch("/api/logout", { method: "POST" });
  loginView.classList.remove("hidden");
  appView.classList.add("hidden");
  logoutBtn.classList.add("hidden");
});

refreshBtn.addEventListener("click", loadFiles);
fileInput.addEventListener("change", () => uploadFiles(fileInput.files));

["dragenter", "dragover"].forEach((name) => {
  dropZone.addEventListener(name, (event) => {
    event.preventDefault();
    dropZone.classList.add("dragover");
  });
});

["dragleave", "drop"].forEach((name) => {
  dropZone.addEventListener(name, (event) => {
    event.preventDefault();
    dropZone.classList.remove("dragover");
  });
});

dropZone.addEventListener("drop", (event) => {
  uploadFiles(event.dataTransfer.files);
});

async function boot() {
  const res = await fetch("/api/files");
  if (res.ok) {
    showApp();
    await renderFiles(await res.json());
  }
}

function showApp() {
  loginView.classList.add("hidden");
  appView.classList.remove("hidden");
  logoutBtn.classList.remove("hidden");
}

async function loadFiles() {
  const res = await fetch("/api/files");
  if (res.status === 401) {
    loginView.classList.remove("hidden");
    appView.classList.add("hidden");
    logoutBtn.classList.add("hidden");
    return;
  }
  if (!res.ok) {
    showToast("读取文件列表失败");
    return;
  }
  await renderFiles(await res.json());
}

async function renderFiles(data) {
  const files = data.files || [];
  fileList.innerHTML = "";
  emptyState.classList.toggle("hidden", files.length > 0);
  for (const file of files) {
    const row = document.createElement("article");
    row.className = "file-row";
    row.innerHTML = `
      <div>
        <div class="file-name"></div>
        <div class="meta">${formatSize(file.size)} · ${formatDate(file.mtime)}</div>
      </div>
      <div class="actions">
        <a href="/api/files/${encodeURIComponent(file.id)}">下载</a>
        <button class="delete" type="button">删除</button>
      </div>
    `;
    row.querySelector(".file-name").textContent = file.name;
    row.querySelector(".delete").addEventListener("click", () => deleteFile(file.id));
    fileList.appendChild(row);
  }
}

function uploadFiles(files) {
  if (!files || files.length === 0) {
    return;
  }
  const formData = new FormData();
  for (const file of files) {
    formData.append("files", file);
  }

  const xhr = new XMLHttpRequest();
  xhr.open("POST", "/api/files");
  xhr.upload.addEventListener("progress", (event) => {
    if (!event.lengthComputable) {
      return;
    }
    progressWrap.classList.remove("hidden");
    progressBar.style.width = `${Math.round((event.loaded / event.total) * 100)}%`;
  });
  xhr.addEventListener("load", async () => {
    progressBar.style.width = "0";
    progressWrap.classList.add("hidden");
    fileInput.value = "";
    if (xhr.status >= 200 && xhr.status < 300) {
      showToast("上传完成");
      await loadFiles();
      return;
    }
    showToast("上传失败");
  });
  xhr.addEventListener("error", () => showToast("上传失败"));
  xhr.send(formData);
}

async function deleteFile(id) {
  const res = await fetch(`/api/files/${encodeURIComponent(id)}`, { method: "DELETE" });
  if (!res.ok) {
    showToast("删除失败");
    return;
  }
  await loadFiles();
}

function formatSize(size) {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = size;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function formatDate(value) {
  return new Date(value).toLocaleString();
}

async function readJSON(res) {
  try {
    return await res.json();
  } catch {
    return {};
  }
}

function showToast(message) {
  toast.textContent = message;
  toast.classList.remove("hidden");
  window.clearTimeout(showToast.timer);
  showToast.timer = window.setTimeout(() => toast.classList.add("hidden"), 2200);
}

boot();
