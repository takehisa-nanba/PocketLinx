let projects = [];
let activeProjectIndex = 0;

async function refreshData() {
    try {
        const res = await fetch('/api/compose/projects');
        projects = await res.json();
        updateUI();
    } catch (e) { console.error(e); }
}

function updateUI() {
    renderContainerControls();
    renderNetworkMap();
}

function renderContainerControls() {
    const project = projects[activeProjectIndex];
    if (!project) return;

    const list = document.getElementById('control-list');
    list.innerHTML = project.containers.map(c => {
        const isRunning = c.status === "Running";

        // Windows側の個別IPを計算 (10.10.0.x -> 127.0.0.x)
        let winIP = "localhost";
        if (c.ip) {
            const parts = c.ip.split('.');
            if (parts.length === 4) {
                winIP = `127.0.0.${parts[3]}`;
            }
        }

        return `
        <div class="container-card ${isRunning ? 'card-running' : ''}">
            <div class="card-top">
                <div class="title-group">
                    <span class="card-name">${c.name || 'unnamed'}</span>
                    <span class="card-image">${c.image}</span>
                </div>
                <div style="font-size: 0.7rem; color: ${isRunning ? 'var(--accent-success)' : 'var(--text-muted)'}; font-weight: 700;">
                    ${c.status.toUpperCase()}
                </div>
            </div>
            
            <div class="stats-grid">
               <div class="progress-bg"><div class="progress-fill" style="width: ${isRunning ? 35 : 0}%; background: var(--accent-blue);"></div></div>
            </div>

            ${(c.ports || c.Ports) && (c.ports || c.Ports).length > 0 ? `
            <div class="access-urls">
                ${(c.ports || c.Ports).map(p => {
            const hostPort = p.host || p.Host;
            if (isRunning) {
                return `
                    <a href="http://${winIP}:${hostPort}" target="plx-${c.id}-${hostPort}" class="access-link">
                        🔗 ${winIP}:${hostPort}
                    </a>`;
            } else {
                return `
                    <span class="access-link disabled">
                        🚫 ${winIP}:${hostPort}
                    </span>`;
            }
        }).join('')}
            </div>
            ` : ''}

            <div class="actions">
                ${isRunning ?
                `<button class="btn btn-stop" onclick="stopContainer('${c.id}')">STOP</button>` :
                `<button class="btn btn-primary" onclick="startContainer('${c.id}')">START</button>`
            }
                <button class="btn" onclick="showLogs('${c.id}')">LOGS</button>
                <button class="btn" onclick="showConfig('${c.id}')">CONFIG</button>
                <button class="btn" onclick="cloneContainer('${c.id}')">CLONE</button>
                <button class="btn" onclick="editContainer('${c.id}')">EDIT</button>
                <button class="btn btn-danger" onclick="removeContainer('${c.id}')">RM</button>
            </div>
        </div>`;
    }).join('');
}

function renderNetworkMap() {
    const project = projects[activeProjectIndex];
    if (!project) return;

    const svg = document.getElementById('map-svg');
    const width = svg.clientWidth;
    const height = svg.clientHeight;
    if (width === 0) return;

    svg.innerHTML = '';
    const centerX = width / 2;
    const centerY = height / 2;
    const radius = Math.min(width, height) * 0.3;

    // Bridge Node (plx0) - NEON PRO Design
    const bridgeGroup = document.createElementNS("http://www.w3.org/2000/svg", "g");

    // Define a neon filter for the bridge
    const filter = document.createElementNS("http://www.w3.org/2000/svg", "filter");
    filter.setAttribute("id", "neon-glow");
    filter.innerHTML = `
        <feGaussianBlur stdDeviation="2.5" result="coloredBlur"/>
        <feMerge>
            <feMergeNode in="coloredBlur"/>
            <feMergeNode in="SourceGraphic"/>
        </feMerge>`;
    svg.appendChild(filter);

    // Bridge Path (Two towers, suspension cables)
    const bridgeSymbol = document.createElementNS("http://www.w3.org/2000/svg", "g");
    bridgeSymbol.setAttribute("filter", "url(#neon-glow)");

    // Main deck
    const deck = document.createElementNS("http://www.w3.org/2000/svg", "line");
    deck.setAttribute("x1", -30); deck.setAttribute("y1", 5);
    deck.setAttribute("x2", 30); deck.setAttribute("y2", 5);
    deck.setAttribute("stroke", "var(--accent-blue)");
    deck.setAttribute("stroke-width", "2");
    bridgeSymbol.appendChild(deck);

    // Towers
    const towers = document.createElementNS("http://www.w3.org/2000/svg", "path");
    towers.setAttribute("d", "M -18,5 L -18,-15 M 18,5 L 18,-15");
    towers.setAttribute("stroke", "var(--accent-blue)");
    towers.setAttribute("stroke-width", "2.5");
    bridgeSymbol.appendChild(towers);

    // Suspension Cables
    const cables = document.createElementNS("http://www.w3.org/2000/svg", "path");
    cables.setAttribute("d", "M -30,-5 Q -18,-25 -18,-15 Q 0,0 18,-15 Q 18,-25 30,-5");
    cables.setAttribute("fill", "none");
    cables.setAttribute("stroke", "var(--accent-cyan)");
    cables.setAttribute("stroke-width", "1.5");
    bridgeSymbol.appendChild(cables);

    // Vertical hangers
    const hangers = document.createElementNS("http://www.w3.org/2000/svg", "path");
    hangers.setAttribute("d", "M -10,3 L -10,-8 M 0,5 L 0,-10 M 10,3 L 10,-8");
    hangers.setAttribute("stroke", "var(--accent-cyan)");
    hangers.setAttribute("stroke-width", "0.8");
    bridgeSymbol.appendChild(hangers);

    // Pylons
    const pylons = document.createElementNS("http://www.w3.org/2000/svg", "path");
    pylons.setAttribute("d", "M -18,5 L -18,15 M -18,15 L -22,15 M -18,15 L -14,15 M 18,5 L 18,15 M 18,15 L 14,15 M 18,15 L 22,15");
    pylons.setAttribute("stroke", "var(--accent-blue)");
    pylons.setAttribute("stroke-width", "2");
    bridgeSymbol.appendChild(pylons);

    bridgeSymbol.setAttribute("transform", `translate(${centerX}, ${centerY})`);
    bridgeGroup.appendChild(bridgeSymbol);

    // Legend
    const bridgeLabel = document.createElementNS("http://www.w3.org/2000/svg", "text");
    bridgeLabel.setAttribute("x", centerX);
    bridgeLabel.setAttribute("y", centerY + 35);
    bridgeLabel.setAttribute("text-anchor", "middle");
    bridgeLabel.setAttribute("fill", "var(--accent-blue)");
    bridgeLabel.setAttribute("font-size", "9");
    bridgeLabel.setAttribute("font-weight", "800");
    bridgeLabel.setAttribute("style", "text-shadow: 0 0 5px var(--accent-blue);");
    bridgeLabel.textContent = "PLX0 NETWORK BRIDGE";
    bridgeGroup.appendChild(bridgeLabel);

    // Draw lines and containers
    project.containers.forEach((c, i) => {
        const angle = (i * (360 / project.containers.length) - 90) * (Math.PI / 180);
        const tx = centerX + radius * Math.cos(angle);
        const ty = centerY + radius * Math.sin(angle);
        const isRunning = c.status === "Running";

        const line = document.createElementNS("http://www.w3.org/2000/svg", "line");
        line.setAttribute("x1", centerX);
        line.setAttribute("y1", centerY);
        line.setAttribute("x2", tx);
        line.setAttribute("y2", ty);
        line.setAttribute("stroke", isRunning ? "var(--accent-blue)" : "var(--border-muted)");
        line.setAttribute("stroke-width", "1.5");
        if (isRunning) line.classList.add("connection-line");
        else line.setAttribute("opacity", "0.2");
        svg.appendChild(line);

        // Container bubble
        const g = document.createElementNS("http://www.w3.org/2000/svg", "g");
        const nodeBody = document.createElementNS("http://www.w3.org/2000/svg", "circle");
        nodeBody.setAttribute("cx", tx);
        nodeBody.setAttribute("cy", ty);
        nodeBody.setAttribute("r", 25);
        nodeBody.setAttribute("fill", "var(--bg-panel)");
        nodeBody.setAttribute("stroke", isRunning ? "var(--accent-blue)" : "var(--border-muted)");
        nodeBody.setAttribute("stroke-width", "2");
        g.appendChild(nodeBody);

        const label = document.createElementNS("http://www.w3.org/2000/svg", "text");
        label.setAttribute("x", tx);
        label.setAttribute("y", ty + 4);
        label.setAttribute("text-anchor", "middle");
        label.setAttribute("fill", "white");
        label.setAttribute("font-size", "8");
        label.textContent = (c.name || '...').toUpperCase();
        g.appendChild(label);

        svg.appendChild(g);
    });

    svg.appendChild(bridgeGroup);
}

async function startContainer(id) {
    appendLog(`Starting container ${id}...`);
    await fetch(`/api/start?id=${id}`);
    refreshData();
}

async function stopContainer(id) {
    appendLog(`Stopping container ${id}...`);
    await fetch(`/api/stop?id=${id}`);
    refreshData();
}

async function removeContainer(id) {
    if (!confirm('本当に削除しますか？')) return;
    appendLog(`Removing container ${id}...`);
    await fetch(`/api/remove?id=${id}`);
    refreshData();
}

// Edit Mode State
let editingContainerId = null;

function editContainer(id) {
    let container = null;
    for (const p of projects) {
        const found = p.containers.find(c => c.id === id);
        if (found) { container = found; break; }
    }
    if (!container) return;

    editingContainerId = id;

    let cmd = container.command || '';
    if (container.config && container.config.Args) {
        cmd = container.config.Args.join(' ');
    }
    let portStr = '';
    let ports = container.ports || [];
    if (container.config && container.config.Ports) {
        ports = container.config.Ports;
    }
    if (ports && ports.length > 0) {
        const p = ports[0];
        const h = p.Host !== undefined ? p.Host : p.host;
        const c = p.Container !== undefined ? p.Container : p.container;
        portStr = `${h}:${c}`;
    }

    openRunModal({
        image: container.image,
        name: container.name || '',
        cmd: cmd,
        port: portStr
    });
}

function cloneContainer(id) {
    let container = null;
    for (const p of projects) {
        const found = p.containers.find(c => c.id === id);
        if (found) { container = found; break; }
    }
    if (!container) return;

    editingContainerId = null; // Ensure we are NOT in edit mode

    let cmd = container.command || '';
    if (container.config && container.config.Args) {
        cmd = container.config.Args.join(' ');
    }
    let portStr = '';
    let ports = container.ports || [];
    if (container.config && container.config.Ports) {
        ports = container.config.Ports;
    }
    if (ports && ports.length > 0) {
        const p = ports[0];
        const h = p.Host !== undefined ? p.Host : p.host;
        const c = p.Container !== undefined ? p.Container : p.container;
        portStr = `${h}:${c}`;
    }

    openRunModal({
        image: container.image,
        name: container.name ? `${container.name}-copy` : '',
        cmd: cmd,
        port: portStr
    });
}

function showConfig(id) {
    let container = null;
    for (const p of projects) {
        const found = p.containers.find(c => c.id === id);
        if (found) { container = found; break; }
    }
    if (!container) return;

    const json = JSON.stringify(container, null, 2);
    const logArea = document.getElementById('log-scroller');
    logArea.innerHTML = `<div style="padding:10px; font-family:var(--font-mono); font-size:11px; white-space:pre-wrap; color:var(--text-primary);"><strong>CONTAINER CONFIGURATION (Furniture List):</strong>\n\n${json}</div>`;
    document.querySelector('.activity-overlay').classList.add('active');
}

async function showLogs(id) {
    const res = await fetch(`/api/logs?id=${id}`);
    const text = await res.text();
    const logArea = document.getElementById('log-scroller');
    logArea.innerHTML = `<pre style="color: var(--text-main); font-size: 11px; white-space: pre-wrap;">${text}</pre>`;
    document.querySelector('.activity-overlay').classList.add('active');
}

function appendLog(msg) {
    const logArea = document.getElementById('log-scroller');
    const time = new Date().toLocaleTimeString();
    logArea.innerHTML += `<div style="font-size: 11px; color: var(--text-muted);">[${time}] ${msg}</div>`;
}

// Global loop
// --- Modal Logic ---
async function loadImages() {
    const res = await fetch('/api/images');
    const images = await res.json();
    const select = document.getElementById('run-image');
    // Save current selection if re-loading (though mostly we re-open)
    const current = select.value;
    select.innerHTML = images.map(img => `<option value="${img}">${img}</option>`).join('') || '<option value="">No images found</option>';
    if (current) select.value = current;
}

function applyPreset() {
    const preset = document.getElementById('run-preset').value;
    const cmdInput = document.getElementById('run-cmd');
    const portInput = document.getElementById('run-port');
    const imgSelect = document.getElementById('run-image');

    const selectImage = (name) => {
        for (let i = 0; i < imgSelect.options.length; i++) {
            if (imgSelect.options[i].value.includes(name)) {
                imgSelect.selectedIndex = i;
                return;
            }
        }
    };

    switch (preset) {
        case 'web':
            cmdInput.value = 'sh -c "while true; do printf \'HTTP/1.1 200 OK\\r\\nContent-Length: 21\\r\\n\\r\\nPocketLinx Web Server\\n\' | nc -l -p 80; done"';
            portInput.value = '8080:80';
            selectImage('alpine');
            break;
        case 'api':
            cmdInput.value = 'python3 -m http.server 8000';
            portInput.value = '8000:8000';
            selectImage('python');
            if (imgSelect.selectedIndex === -1) selectImage('alpine');
            break;
        case 'sleep':
            cmdInput.value = 'sleep 36000';
            portInput.value = '';
            selectImage('alpine');
            break;
        case 'shell':
            cmdInput.value = 'sh';
            portInput.value = '';
            selectImage('alpine');
            break;
        case 'custom':
        default:
            break;
    }
}

async function openRunModal(prefillData = null) {
    await loadImages();
    document.getElementById('run-modal').style.display = 'flex';

    const submitBtn = document.querySelector('#run-modal .btn-primary');
    const title = document.querySelector('#run-modal .panel-header');

    // Reset Preset
    const presetSelect = document.getElementById('run-preset');
    if (presetSelect) presetSelect.value = 'custom';

    if (prefillData) {
        document.getElementById('run-image').value = prefillData.image;
        document.getElementById('run-name').value = prefillData.name || '';
        document.getElementById('run-cmd').value = prefillData.cmd || '';
        document.getElementById('run-port').value = prefillData.port || '';

        if (editingContainerId) {
            if (title) title.innerText = `🛠 EDIT CONFIGURATION`;
            if (submitBtn) submitBtn.innerText = "SAVE CHANGES";
            appendLog(`Editing configuration for ${prefillData.name || editingContainerId}...`);
        } else {
            // Clone/Run copy
            if (title) title.innerText = "🚀 RUN NEW CONTAINER";
            if (submitBtn) submitBtn.innerText = "LAUNCH";
            appendLog(`Cloning configuration...`);
        }
    } else {
        editingContainerId = null;
        document.getElementById('run-name').value = '';
        if (title) title.innerText = "🚀 RUN NEW CONTAINER";
        if (submitBtn) submitBtn.innerText = "LAUNCH";
    }
}

function closeRunModal() {
    document.getElementById('run-modal').style.display = 'none';
    editingContainerId = null;
}

async function submitRun() {
    const image = document.getElementById('run-image').value;
    const name = document.getElementById('run-name').value;
    const portMapping = document.getElementById('run-port').value;
    const cmdInput = document.getElementById('run-cmd').value;

    if (!image) return alert('Select an image');

    // Capture ID because closeRunModal clears it
    const targetId = editingContainerId;
    closeRunModal();

    // Parse command respecting quotes
    const parseCommandArgs = (str) => {
        const args = [];
        let current = '';
        let quoteChar = null; // ' or " or null

        for (let i = 0; i < str.length; i++) {
            const char = str[i];

            if (quoteChar) {
                if (char === quoteChar) {
                    quoteChar = null; // Close quote
                } else {
                    current += char;
                }
            } else {
                if (char === '"' || char === "'") {
                    quoteChar = char; // Open quote
                } else if (char === ' ') {
                    if (current.length > 0) {
                        args.push(current);
                        current = '';
                    }
                } else {
                    current += char;
                }
            }
        }
        if (current.length > 0) args.push(current);
        return args;
    };

    const args = cmdInput.trim() ? parseCommandArgs(cmdInput) : [];

    const ports = [];
    if (portMapping) {
        const parts = portMapping.split(':');
        if (parts.length === 2) {
            ports.push({ host: parseInt(parts[0]), container: parseInt(parts[1]) });
        }
    }

    try {
        if (targetId) {
            appendLog(`Updating container ${targetId}...`);
            await fetch(`/api/update?id=${targetId}`, {
                method: 'POST',
                body: JSON.stringify({ image, name, args, ports })
            });
            appendLog(`Container updated. Please restart to apply changes.`);
        } else {
            appendLog(`Running new container: ${image}...`);
            await fetch('/api/run', {
                method: 'POST',
                body: JSON.stringify({ image, name, args, ports })
            });
        }
        refreshData();
    } catch (e) {
        appendLog(`Error: ${e.message}`);
    }
}

setInterval(refreshData, 3000);
refreshData();
window.onresize = renderNetworkMap;
