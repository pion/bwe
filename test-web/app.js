async function fetchJSONL(url) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`Failed to load ${url}`);
  const text = await res.text();
  return text
    .split("\n")
    .filter(line => line.trim())
    .map(line => JSON.parse(line));
}

function showTest(name) {
  document.querySelectorAll(".tab").forEach(t => t.classList.remove("active"));
  document.querySelectorAll(".test-section").forEach(s => s.classList.remove("active"));
  document.querySelector(`.tab[data-test='${name}']`).classList.add("active");
  document.querySelector(`#test-${name}`).classList.add("active");

  // Load plots only once
  const section = document.querySelector(`#test-${name}`);
  if (!section.dataset.loaded) {
    loadAndPlotTest(name, section);
    section.dataset.loaded = "true";
  }
}

function createPlot(container) {
  const plotDiv = document.createElement("div");
  plotDiv.className = "plot";
  container.appendChild(plotDiv);
  return plotDiv;
}

// --- Metric computations ---
function computeRate(rtpLogs) {
  const out = rtpLogs.filter(p => p["vantage-point"] === "sender");
  const recv = rtpLogs.filter(p => p["vantage-point"] === "receiver");
  function binRates(packets) {
    const bins = {};
    for (const pkt of packets) {
      const t = Math.floor(new Date(pkt.time).getTime() / 1000);
      bins[t] = (bins[t] || 0) + pkt["payload-size"];
    }
    return Object.keys(bins)
      .map(Number)
      .sort((a, b) => a - b)
      .map(t => ({
        time: new Date(t * 1000),
        rate_mbps: (bins[t] * 8) / 1e6,
      }));
  }
  return { send: binRates(out), recv: binRates(recv) };
}

function computeDelays(rtpLogs) {
  const sendPackets = new Map();
  const delays = [];
  for (const p of rtpLogs) {
    if (p["vantage-point"] === "sender") {
      sendPackets.set(p["unwrapped-sequence-number"], new Date(p.time));
    } else if (p["vantage-point"] === "receiver") {
      const sentTime = sendPackets.get(p["unwrapped-sequence-number"]);
      if (sentTime) {
        const recvTime = new Date(p.time);
        delays.push({ time: recvTime, delay_ms: recvTime - sentTime });
      }
    }
  }
  return delays.sort((a, b) => a.time - b.time);
}

function computeLossRate(rtpLogs) {
  const senderPackets = rtpLogs.filter(p => p["vantage-point"] === "sender");
  const receiverPackets = rtpLogs.filter(p => p["vantage-point"] === "receiver");
  const receivedSeq = new Set(receiverPackets.map(p => p["unwrapped-sequence-number"]));

  const sentPerSecond = {};
  const lostPerSecond = {};

  for (const pkt of senderPackets) {
    const t = Math.floor(new Date(pkt.time).getTime() / 1000);
    sentPerSecond[t] = (sentPerSecond[t] || 0) + 1;
    if (!receivedSeq.has(pkt["unwrapped-sequence-number"])) {
      lostPerSecond[t] = (lostPerSecond[t] || 0) + 1;
    }
  }

  const times = Object.keys(sentPerSecond)
    .map(Number)
    .sort((a, b) => a - b);

  return times.map(sec => ({
    time: new Date(sec * 1000),
    lossRate: sentPerSecond[sec] > 0 ? (lostPerSecond[sec] || 0) / sentPerSecond[sec] : 0,
  }));
}

// --- Plot functions ---
function plotSendReceive(section, logs) {
  const rtp = logs.filter(e => e.msg === "rtp");
  const { send, recv } = computeRate(rtp);
  const targetLogs = logs.filter(e => e.msg === "setting codec target bitrate");
  const target = targetLogs.map(l => ({ time: new Date(l.time), rate_mbps: l.rate / 1e6 }));

  const traces = [
    { x: send.map(p => p.time), y: send.map(p => p.rate_mbps), type: "scatter", mode: "lines", name: "Send rate", line: { color: "blue" } },
    { x: recv.map(p => p.time), y: recv.map(p => p.rate_mbps), type: "scatter", mode: "lines", name: "Receive rate", line: { color: "green" } },
  ];
  if (target.length) {
    traces.push({
      x: target.map(p => p.time),
      y: target.map(p => p.rate_mbps),
      type: "scatter",
      mode: "lines",
      name: "Target rate",
      line: { color: "red", dash: "dot" },
    });
  }

  const div = createPlot(section);
  Plotly.newPlot(div, traces, {
    title: "Send / Receive Rate",
    xaxis: { title: "Time" },
    yaxis: { title: "Rate (Mbps)" },
    margin: { t: 40, b: 40, l: 60, r: 20 },
  });
}

function plotDelay(section, logs) {
  const delays = computeDelays(logs.filter(e => e.msg === "rtp"));
  const trace = {
    x: delays.map(d => d.time),
    y: delays.map(d => d.delay_ms),
    type: "scatter",
    mode: "lines+markers",
    name: "Delay (ms)",
    marker: { size: 3, color: "orange" },
    line: { color: "orange" },
  };
  const div = createPlot(section);
  Plotly.newPlot(div, [trace], {
    title: "End-to-End Delay",
    xaxis: { title: "Time" },
    yaxis: { title: "Delay (ms)" },
    margin: { t: 40, b: 40, l: 60, r: 20 },
  });
}

function plotLoss(section, logs) {
  const loss = computeLossRate(logs.filter(e => e.msg === "rtp"));
  const trace = {
    x: loss.map(p => p.time),
    y: loss.map(p => p.lossRate * 100),
    type: "scatter",
    mode: "lines+markers",
    name: "Loss (%)",
    marker: { size: 3, color: "red" },
    line: { color: "red" },
  };
  const div = createPlot(section);
  Plotly.newPlot(div, [trace], {
    title: "Packet Loss",
    xaxis: { title: "Time" },
    yaxis: { title: "Loss (%)" },
    margin: { t: 40, b: 40, l: 60, r: 20 },
  });
}

// --- Lazy loading per test ---
async function loadAndPlotTest(name, section) {
  const file = `${name}.jsonl`;
  const logs = await fetchJSONL(`logs/${file}`);
  plotSendReceive(section, logs);
  plotDelay(section, logs);
  plotLoss(section, logs);
}

// --- Main init ---
async function initDashboard() {
  const logFiles = await fetch("logs/index.json").then(r => r.json());
  const sidebar = document.getElementById("sidebar");
  const dashboard = document.getElementById("dashboard");

  for (const file of logFiles) {
    const testName = file.replace(".jsonl", "");

    const tab = document.createElement("button");
    tab.className = "tab";
    tab.textContent = testName;
    tab.dataset.test = testName;
    tab.onclick = () => showTest(testName);
    sidebar.appendChild(tab);

    const section = document.createElement("div");
    section.className = "test-section";
    section.id = `test-${testName}`;
    dashboard.appendChild(section);
  }

  const firstTest = logFiles[0].replace(".jsonl", "");
  showTest(firstTest);
}

initDashboard();

