// ======= 接口约定（列目录 / 下载 / 验证码上传） =======
const PREFIX = "";  // 末尾不要加 /，例如 "/downloads"
const API = {
    LIST: PREFIX + "/__fs_list",
    DOWNLOAD: PREFIX + "/__fs_download",
    CHECK: PREFIX + "/__fs_checkcode",
    UPLOAD: PREFIX + "/__fs_upload"
};
// ======================================================================

const $ = (s) => document.querySelector(s);
const tbody = $("#tbody");
const crumbs = $("#crumbs");
const where = $("#where");
const q = $("#q");
const btnUp = $("#btnUp");
const btnRefresh = $("#btnRefresh");

const codeEl = $("#code");
const fileEl = $("#file");
const dropZone = document.getElementById("drop");
const fileNameEl = document.getElementById("fileName");
const drop = $("#drop");
const btnUpload = $("#btnUpload");
const prog = $("#prog");
const progText = $("#progText");
const statusEl = $("#status");

// 状态（服务端文件系统视角，不带前缀）
const ROOT = "/";
let currentDir = ROOT;
let allItems = [];

// --------- 工具函数 ---------
function fmtBytes(n) {
    if (!Number.isFinite(n)) return "";
    const u = ["B", "KB", "MB", "GB", "TB"]; let i = 0;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return n.toFixed(n < 10 && i > 0 ? 1 : 0) + " " + u[i];
}
function normalize(path) {
    const isDir = path.endsWith("/");
    const parts = path.split("/").filter(Boolean);
    const stack = [];
    for (const p of parts) {
        if (p === ".") continue;
        if (p === "..") { stack.pop(); continue; }
        stack.push(p);
    }
    let result = "/" + stack.join("/") + (isDir ? "/" : "");
    // 避免出现 "//"
    if (result === "//") result = "/";
    return result;
}
function safeJoin(base, seg) {
    return normalize((base.endsWith("/") ? base : base + "/") + seg);
}
function parentOf(path) {
    if (path === ROOT) return ROOT;
    const p = normalize(path).replace(/\/+$/, "").split("/").slice(0, -1).join("/") + "/";
    return p.startsWith(ROOT) ? p : ROOT;
}
function setStatus(html, cls = "") {
    statusEl.className = "status " + cls;
    statusEl.innerHTML = html || "";
}
function resetFilePicker() {
    fileEl.value = "";
    fileNameEl.textContent = "";
    dropZone.classList.remove("dragover");
}

// --------- 渲染面包屑 ---------
function renderCrumbs() {
    crumbs.innerHTML = "";
    const parts = normalize(currentDir).replace(/\/+$/, "").split("/").filter(Boolean);
    let acc = "/";
    const add = (name, href) => {
        const a = document.createElement("a");
        a.textContent = name; a.href = href;
        a.addEventListener("click", (e) => { e.preventDefault(); goto(href); });
        return a;
    };
    crumbs.append(add(".", "/"));
    for (let i = 0; i < parts.length; i++) {
        acc += parts[i] + "/";
        crumbs.append(document.createTextNode("/"));
        crumbs.append(add(parts[i], acc));
    }
    where.textContent = currentDir;
}

// --------- 加载目录 ---------
async function loadDir() {
    renderCrumbs();
    try {
        const u = new URL(API.LIST, location.origin);
        u.searchParams.set("dir", currentDir);          // 仅传逻辑目录，不带前缀
        const res = await fetch(u.toString(), { headers: { "Accept": "application/json" } });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        if (!data.ok) throw new Error(data.error || "列表获取失败");
        currentDir = data.cwd || currentDir;
        allItems = (data.items || []).slice();

        const kw = (q.value || "").trim().toLowerCase();
        let items = allItems;
        if (kw) items = items.filter(x => x.name.toLowerCase().includes(kw));

        items.sort((a, b) => (b.type === "dir") - (a.type === "dir") || a.name.localeCompare(b.name, "zh"));
        renderTable(items);
    } catch (e) {
        tbody.innerHTML = `<tr><td class="err" colspan="3">加载失败：${e.message}</td></tr>`;
    }
}

function renderTable(items) {
    if (!items.length) {
        tbody.innerHTML = `<tr><td class="muted" colspan="3">空目录</td></tr>`;
        return;
    }
    tbody.innerHTML = "";
    for (const it of items) {
        const tr = document.createElement("tr");

        const tdName = document.createElement("td");
        tdName.className = "name";
        const a = document.createElement("a");
        a.textContent = it.name + (it.type === "dir" ? "/" : "");
        if (it.type === "dir") {
            const dirPath = safeJoin(currentDir, it.name); // 纯逻辑目录，如 "/111/"
            a.dataset.dir = dirPath;
            a.href = "javascript:void(0)"; // 防止浏览器改成绝对 URL
        } else {
            a.href = new URL(API.DOWNLOAD, location.origin).toString() +
                "?path=" + encodeURIComponent(safeJoin(currentDir, it.name).replace(/\/$/, ""));
        }
        a.addEventListener("click", (e) => {
            e.preventDefault();
            if (it.type === "dir") goto(a.dataset.dir);
            else window.open(a.href, "_blank");
        });
        tdName.append(a);

        const tdTime = document.createElement("td");
        tdTime.className = "muted";
        tdTime.textContent = it.mtime || "";

        const tdSize = document.createElement("td");
        tdSize.className = "right muted";
        tdSize.textContent = it.type === "dir" ? "-" : (typeof it.size === "number" ? fmtBytes(it.size) : (it.size || ""));

        tr.append(tdName, tdTime, tdSize);
        tbody.append(tr);
    }
}

// --------- 导航 ---------
function goto(dirPath) {
    // 兜底：有人传了绝对 URL，就取 pathname
    try {
        if (/^https?:/i.test(dirPath)) {
            const u = new URL(dirPath);
            dirPath = u.pathname; // 例如 "/downloads/111/"
        }
    } catch { }
    // 去掉代理前缀（如果设置了）
    if (PREFIX && dirPath.startsWith(PREFIX + "/")) {
        dirPath = dirPath.slice(PREFIX.length); // "/downloads/111/" -> "/111/"
    }
    currentDir = normalize(dirPath);   // e.g. "/sub/child/"
    resetFilePicker();
    loadDir();
}

btnUp.addEventListener("click", () => goto(parentOf(currentDir)));
btnRefresh.addEventListener("click", () => { resetFilePicker(); loadDir(); });
q.addEventListener("input", () => {
    const kw = (q.value || "").trim().toLowerCase();
    renderTable(allItems.filter(x => x.name.toLowerCase().includes(kw)));
});

// --------- 上传（两步式：先校验 code -> 再带 token 上传） ---------
fileEl.multiple = false;

drop.addEventListener("dragenter", e => { e.preventDefault(); drop.classList.add("drag"); });
drop.addEventListener("dragover", e => e.preventDefault());
drop.addEventListener("dragleave", () => drop.classList.remove("drag"));
drop.addEventListener("drop", e => {
    e.preventDefault(); drop.classList.remove("drag");
    if (e.dataTransfer?.files?.length) fileEl.files = e.dataTransfer.files;
});

// 显示文件名
function showFileName(file) {
    fileNameEl.textContent = file ? `已选择文件：${file.name}` : "";
}
fileEl.addEventListener("change", () => {
    showFileName(fileEl.files.length > 0 ? fileEl.files[0] : null);
});
dropZone.addEventListener("dragover", (e) => { e.preventDefault(); dropZone.classList.add("dragover"); });
dropZone.addEventListener("dragleave", () => { dropZone.classList.remove("dragover"); });
dropZone.addEventListener("drop", (e) => {
    e.preventDefault(); dropZone.classList.remove("dragover");
    if (e.dataTransfer.files.length > 0) {
        fileEl.files = e.dataTransfer.files;
        showFileName(e.dataTransfer.files[0]);
    }
});

function lockUpload(v) { btnUpload.disabled = v; codeEl.disabled = v; fileEl.disabled = v; }

btnUpload.addEventListener("click", async (e) => {
    e.preventDefault();
    setStatus("");
    const code = (codeEl.value || "").trim();
    const f = fileEl.files?.[0];
    if (!code) { setStatus("请输入验证码。", "err"); return; }
    if (!f) { setStatus("请选择要上传的文件。", "err"); return; }

    try {
        // Step 1: 验证码校验（不传前缀，前缀已体现在 API 路径上）
        const checkUrl = new URL(API.CHECK, location.origin);
        checkUrl.searchParams.set("code", code);
        checkUrl.searchParams.set("dir", currentDir);
        const res = await fetch(checkUrl);
        const data = await res.json();
        if (!res.ok || !data.ok) {
            setStatus(data.error || `验证码验证失败（HTTP ${res.status}）`, "err");
            return;
        }

        // Step 2: 带 token 上传文件
        const url = new URL(API.UPLOAD, location.origin);
        url.searchParams.set("dir", currentDir);
        url.searchParams.set("token", data.token);

        const fd = new FormData();
        fd.append("file", f);

        const xhr = new XMLHttpRequest();
        xhr.open("POST", url.toString());
        lockUpload(true); prog.hidden = false; prog.value = 0; progText.textContent = "";

        xhr.upload.onprogress = (ev) => {
            if (ev.lengthComputable) {
                const p = Math.round(ev.loaded * 100 / ev.total);
                prog.value = p;
                progText.textContent = `${p}% (${fmtBytes(ev.loaded)} / ${fmtBytes(ev.total)})`;
            }
        };
        xhr.onerror = () => { lockUpload(false); prog.hidden = true; setStatus("网络错误。", "err"); };
        xhr.onload = () => {
            lockUpload(false); prog.hidden = true;
            let upres = null;
            try { if ((xhr.getResponseHeader("content-type") || "").includes("application/json")) upres = JSON.parse(xhr.responseText); } catch { }
            if (xhr.status >= 200 && xhr.status < 300 && upres?.ok) {
                // 注意：后端返回的 url 没前缀，前端展示时要补上 PREFIX
                const dl = PREFIX + upres.url;
                setStatus(`上传成功：<a href="${dl}" target="_blank" rel="noopener">${upres.saved_as || "查看文件"}</a>`, "ok");
                resetFilePicker();
                loadDir();
                return;
            }
            setStatus((upres && upres.error) || `上传失败（HTTP ${xhr.status}）`, "err");
        };
        xhr.send(fd);
    } catch (err) {
        setStatus("请求出错：" + err.message, "err");
    }
});

// --------- 初始化 ---------
document.addEventListener("DOMContentLoaded", () => { resetFilePicker(); });
loadDir();
