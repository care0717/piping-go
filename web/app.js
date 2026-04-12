// --- Utilities ---

function randomDigits(len) {
  const arr = new Uint32Array(len);
  crypto.getRandomValues(arr);
  return Array.from(arr, (n) => (n % 10).toString()).join("");
}

function readableBytes(bytes) {
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
  if (bytes < 1024 * 1024 * 1024)
    return (bytes / (1024 * 1024)).toFixed(1) + " MB";
  return (bytes / (1024 * 1024 * 1024)).toFixed(2) + " GB";
}

function getServerUrl() {
  return window.location.origin;
}

function getFilenameFromHeaders(headers) {
  const cd = headers.get("Content-Disposition");
  if (!cd) return null;
  const match = cd.match(/filename\*?=(?:UTF-8''|"?)([^";]+)"?/i);
  if (match) return decodeURIComponent(match[1]);
  return null;
}

function getReceiveMode() {
  var checked = document.querySelector('input[name="recv-mode"]:checked');
  return checked ? checked.value : "download";
}

function clearReceiveOutput() {
  recvOutput.hidden = true;
  recvContent.textContent = "";
  recvCopyBtn.disabled = true;
}

// --- DOM References ---

const sendPanel = document.getElementById("send-panel");
const recvPanel = document.getElementById("receive-panel");
const tabs = document.querySelectorAll(".tab");

const sendPath = document.getElementById("send-path");
const sendPathGen = document.getElementById("send-path-gen");
const dropZone = document.getElementById("drop-zone");
const fileInput = document.getElementById("file-input");
const fileListEl = document.getElementById("file-list");
const textInput = document.getElementById("text-input");
const sendBtn = document.getElementById("send-btn");
const sendStatus = document.getElementById("send-status");
const sendProgress = document.getElementById("send-progress");
const sendMessage = document.getElementById("send-message");

const recvPath = document.getElementById("recv-path");
const recvBtn = document.getElementById("recv-btn");
const recvStatus = document.getElementById("recv-status");
const recvProgress = document.getElementById("recv-progress");
const recvMessage = document.getElementById("recv-message");
const recvOutput = document.getElementById("recv-output");
const recvContent = document.getElementById("recv-content");
const recvCopyBtn = document.getElementById("recv-copy-btn");

const themeToggle = document.getElementById("theme-toggle");
const themeIcon = document.getElementById("theme-icon");

// --- State ---

let selectedFiles = [];
let lastReceivedText = null;

// --- Theme ---

function getPreferredTheme() {
  const saved = localStorage.getItem("theme");
  if (saved) return saved;
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

function applyTheme(theme) {
  document.documentElement.dataset.theme = theme;
  themeIcon.textContent = theme === "dark" ? "\u2600" : "\u263E";
  localStorage.setItem("theme", theme);
}

applyTheme(getPreferredTheme());
clearReceiveOutput();

themeToggle.addEventListener("click", function () {
  const current = document.documentElement.dataset.theme;
  applyTheme(current === "dark" ? "light" : "dark");
});

// --- Tabs ---

tabs.forEach(function (tab) {
  tab.addEventListener("click", function () {
    tabs.forEach(function (t) { t.classList.remove("active"); });
    tab.classList.add("active");

    var target = tab.dataset.tab;
    sendPanel.classList.toggle("active", target === "send");
    recvPanel.classList.toggle("active", target === "receive");
  });
});

// --- Secret Path ---

sendPath.value = randomDigits(4);

sendPathGen.addEventListener("click", function () {
  sendPath.value = randomDigits(4);
});

// --- File Selection ---

function updateFileList() {
  fileListEl.innerHTML = "";
  selectedFiles.forEach(function (file, i) {
    var item = document.createElement("div");
    item.className = "file-item";

    var span = document.createElement("span");
    span.textContent = file.name + " (" + readableBytes(file.size) + ")";
    item.appendChild(span);

    var removeBtn = document.createElement("button");
    removeBtn.className = "file-remove";
    removeBtn.textContent = "\u00d7";
    removeBtn.addEventListener("click", function () {
      selectedFiles.splice(i, 1);
      updateFileList();
    });
    item.appendChild(removeBtn);

    fileListEl.appendChild(item);
  });
}

fileInput.addEventListener("change", function () {
  if (fileInput.files) {
    selectedFiles = selectedFiles.concat(Array.from(fileInput.files));
    updateFileList();
    fileInput.value = "";
  }
});

dropZone.addEventListener("dragover", function (e) {
  e.preventDefault();
  dropZone.classList.add("dragover");
});

dropZone.addEventListener("dragleave", function () {
  dropZone.classList.remove("dragover");
});

dropZone.addEventListener("drop", function (e) {
  e.preventDefault();
  dropZone.classList.remove("dragover");
  if (e.dataTransfer && e.dataTransfer.files) {
    selectedFiles = selectedFiles.concat(Array.from(e.dataTransfer.files));
    updateFileList();
  }
});

// --- Send ---

sendBtn.addEventListener("click", function () {
  var path = sendPath.value.trim();
  if (!path) {
    alert("Secret path is required.");
    return;
  }

  var text = textInput.value;
  if (selectedFiles.length === 0 && !text) {
    alert("Select a file or enter text to send.");
    return;
  }

  var body, filename, contentType;

  if (selectedFiles.length === 1 && !text) {
    body = selectedFiles[0];
    filename = selectedFiles[0].name;
    contentType = selectedFiles[0].type || "application/octet-stream";
  } else if (selectedFiles.length === 0 && text) {
    body = new Blob([text], { type: "text/plain; charset=utf-8" });
    filename = "text.txt";
    contentType = "text/plain; charset=utf-8";
  } else {
    // Multiple files or file + text: send first file for now
    body = selectedFiles[0];
    filename = selectedFiles[0].name;
    contentType = selectedFiles[0].type || "application/octet-stream";
  }

  var url = getServerUrl() + "/" + encodeURIComponent(path);

  sendBtn.disabled = true;
  sendStatus.hidden = false;
  sendProgress.classList.add("indeterminate");
  sendProgress.style.width = "";
  sendMessage.textContent = "Waiting for receiver...";
  sendMessage.className = "";

  var xhr = new XMLHttpRequest();
  xhr.open("POST", url);
  xhr.setRequestHeader("Content-Type", contentType);
  xhr.setRequestHeader(
    "Content-Disposition",
    'attachment; filename="' + filename + '"'
  );

  xhr.upload.addEventListener("progress", function (e) {
    if (e.lengthComputable) {
      sendProgress.classList.remove("indeterminate");
      var pct = (e.loaded / e.total) * 100;
      sendProgress.style.width = pct + "%";
      sendMessage.textContent =
        "Sending... " +
        readableBytes(e.loaded) +
        " / " +
        readableBytes(e.total) +
        " (" +
        pct.toFixed(0) +
        "%)";
    }
  });

  var lastResponseLength = 0;
  xhr.addEventListener("readystatechange", function () {
    if (xhr.readyState >= 3 && xhr.responseText.length > lastResponseLength) {
      var newText = xhr.responseText.substring(lastResponseLength);
      lastResponseLength = xhr.responseText.length;
      if (newText.indexOf("Start piping") !== -1) {
        sendProgress.classList.remove("indeterminate");
        sendMessage.textContent = "Sending...";
      }
    }
  });

  xhr.addEventListener("load", function () {
    sendProgress.classList.remove("indeterminate");
    sendProgress.style.width = "100%";
    sendMessage.textContent = "Sent successfully!";
    sendMessage.className = "success";
    sendBtn.disabled = false;
  });

  xhr.addEventListener("error", function () {
    sendProgress.classList.remove("indeterminate");
    sendProgress.style.width = "0%";
    sendMessage.textContent = "Send failed. Check your connection.";
    sendMessage.className = "error";
    sendBtn.disabled = false;
  });

  xhr.send(body);
});

// --- Receive ---

recvCopyBtn.addEventListener("click", async function () {
  if (lastReceivedText === null) return;

  try {
    await navigator.clipboard.writeText(lastReceivedText);
    recvMessage.textContent = "Copied received text.";
    recvMessage.className = "success";
  } catch (err) {
    recvMessage.textContent = "Copy failed: " + (err.message || String(err));
    recvMessage.className = "error";
  }
});

recvBtn.addEventListener("click", async function () {
  var path = recvPath.value.trim();
  if (!path) {
    alert("Secret path is required.");
    return;
  }

  var url = getServerUrl() + "/" + encodeURIComponent(path);

  recvBtn.disabled = true;
  recvStatus.hidden = false;
  recvProgress.classList.add("indeterminate");
  recvProgress.style.width = "";
  recvMessage.textContent = "Waiting for sender...";
  recvMessage.className = "";
  lastReceivedText = null;
  clearReceiveOutput();

  try {
    var res = await fetch(url);
    if (!res.ok) {
      throw new Error("Server returned " + res.status + ": " + (await res.text()));
    }

    var contentLength = res.headers.get("Content-Length");
    var totalBytes = contentLength ? parseInt(contentLength, 10) : null;
    var filename = getFilenameFromHeaders(res.headers) || path;
    var receiveMode = getReceiveMode();

    recvProgress.classList.remove("indeterminate");
    recvMessage.textContent = "Receiving...";

    var reader = res.body.getReader();
    var chunks = [];
    var loaded = 0;

    while (true) {
      var result = await reader.read();
      if (result.done) break;
      chunks.push(result.value);
      loaded += result.value.byteLength;

      if (totalBytes) {
        var pct = (loaded / totalBytes) * 100;
        recvProgress.style.width = pct + "%";
        recvMessage.textContent =
          "Receiving... " +
          readableBytes(loaded) +
          " / " +
          readableBytes(totalBytes) +
          " (" +
          pct.toFixed(0) +
          "%)";
      } else {
        recvMessage.textContent = "Receiving... " + readableBytes(loaded);
      }
    }

    var ct = res.headers.get("Content-Type") || "application/octet-stream";
    var blob = new Blob(chunks, { type: ct });

    if (receiveMode === "screen") {
      var text = await blob.text();
      lastReceivedText = text;
      recvContent.textContent = text;
      recvCopyBtn.disabled = false;
      recvOutput.hidden = false;
      recvMessage.textContent = "Received " + readableBytes(loaded) + " on screen.";
    } else {
      var objUrl = URL.createObjectURL(blob);
      var a = document.createElement("a");
      a.href = objUrl;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(objUrl);
      recvMessage.textContent = "Received " + readableBytes(loaded) + " - " + filename;
    }

    recvProgress.style.width = "100%";
    recvMessage.className = "success";
  } catch (err) {
    recvProgress.classList.remove("indeterminate");
    recvProgress.style.width = "0%";
    recvMessage.textContent = "Error: " + (err.message || String(err));
    recvMessage.className = "error";
  } finally {
    recvBtn.disabled = false;
  }
});
