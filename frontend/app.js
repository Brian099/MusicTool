// ==================== Music Toolkit 前端交互逻辑 ====================

const API = {
    authStatus: '/api/auth/status',
    authInit: '/api/auth/init',
    authLogin: '/api/auth/login',
    authLogout: '/api/auth/logout',
    systemStatus: '/api/system/status',
    dirStats: '/api/system/dir-stats',
    formatScan: '/api/format/scan',
    formatProgress: '/api/format/progress',
    formatRecords: '/api/format/records',
    formatActionSingle: '/api/format/action-single',
    formatActionBatch: '/api/format/action-batch',
    formatFixSingle: '/api/format/action-single',
    formatFixBatch: '/api/format/action-batch',
    formatExport: '/api/format/export',
    dedupCompute: '/api/dedup/compute',
    dedupProgress: '/api/dedup/progress',
    dedupCancel: '/api/dedup/cancel',
    dedupGroups: '/api/dedup/groups',
    dedupClean: '/api/dedup/clean',
    dedupCleanRecommended: '/api/dedup/clean-recommended',
    losslessScan: '/api/lossless/scan',
    losslessProgress: '/api/lossless/progress',
    losslessRecords: '/api/lossless/records',
    losslessExport: '/api/lossless/export',
    playlistParse: '/api/playlist/parse',
    playlistHistory: '/api/playlist/history',
    playlistHistoryDetail: '/api/playlist/history-detail',
    playlistHistoryDelete: '/api/playlist/history-delete',
    playlistHistoryClear: '/api/playlist/history-clear',
    playlistExport: '/api/playlist/export',
    feiniuConnect: '/api/feiniu/connect',
    feiniuStatus: '/api/feiniu/status',
    feiniuDisconnect: '/api/feiniu/disconnect',
    feiniuPlaylists: '/api/feiniu/playlists',
    feiniuPlaylistTracks: '/api/feiniu/playlist/tracks',
    feiniuPlaylistCreate: '/api/feiniu/playlist/create',
    feiniuPlaylistEdit: '/api/feiniu/playlist/edit',
    feiniuPlaylistDelete: '/api/feiniu/playlist/delete',
    feiniuPlaylistAddTracks: '/api/feiniu/playlist/add-tracks',
    feiniuPlaylistRemoveTracks: '/api/feiniu/playlist/remove-tracks',
    feiniuPlaylistPurge: '/api/feiniu/playlist/purge',
    feiniuPlaylistImport: '/api/feiniu/playlist/import',
    feiniuCover: '/api/feiniu/cover',
    audioStream: '/api/audio/stream'
};

// 拦截原生 fetch 以自动附加 Bearer 凭据并在 401 时提醒
const originalFetch = window.fetch;
window.fetch = async function(url, options = {}) {
    options = options || {};
    options.headers = options.headers || {};
    const token = localStorage.getItem('music_toolkit_token');
    if (token) {
        if (options.headers instanceof Headers) {
            if (!options.headers.has('Authorization')) {
                options.headers.set('Authorization', `Bearer ${token}`);
            }
        } else if (Array.isArray(options.headers)) {
            const hasAuth = options.headers.some(([k]) => k.toLowerCase() === 'authorization');
            if (!hasAuth) options.headers.push(['Authorization', `Bearer ${token}`]);
        } else {
            if (!options.headers['Authorization'] && !options.headers['authorization']) {
                options.headers['Authorization'] = `Bearer ${token}`;
            }
        }
    }
    const response = await originalFetch(url, options);
    if (response.status === 401 && typeof url === 'string' && !url.includes('/api/auth/')) {
        if (typeof checkAuthStatus === 'function') {
            checkAuthStatus();
        }
    }
    return response;
};

// 状态对象
const state = {
    authStatus: null,
    system: null,
    formatRecords: [],
    losslessRecords: [],
    playlistSongs: [],
    playlistRawResult: null,
    playlistHistory: [],
    feiniuStatus: null,
    feiniuPlaylists: [],
    currentFeiNiuPlaylist: null,
    formatPollingTimer: null,
    dedupPollingTimer: null,
    losslessPollingTimer: null,
    duplicateGroups: [],
    currentAudioPath: null
};

// 工具函数
function formatBytes(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function formatDuration(seconds) {
    if (!seconds || seconds <= 0) return '0:00';
    const m = Math.floor(seconds / 60);
    const s = Math.floor(seconds % 60);
    return `${m}:${s < 10 ? '0' : ''}${s}`;
}

function showToast(message, type = 'info') {
    const container = document.getElementById('toast-container');
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    toast.textContent = message;
    container.appendChild(toast);
    setTimeout(() => {
        toast.style.opacity = '0';
        toast.style.transform = 'translateX(30px)';
        setTimeout(() => toast.remove(), 300);
    }, 3500);
}

// ==================== 目录文件查询与统计 ====================

async function queryDirStats(dirPath, targetBadgeId) {
    const badge = document.getElementById(targetBadgeId);
    if (!badge) return;

    if (!dirPath || !dirPath.trim()) {
        badge.className = 'dir-stats-badge not-found';
        badge.innerHTML = '<span class="stats-loading">⚠️ 请输入有效目录路径</span>';
        return;
    }

    badge.className = 'dir-stats-badge';
    badge.innerHTML = '<span class="stats-loading">🔍 正在统计音乐文件...</span>';

    try {
        const res = await fetch(`${API.dirStats}?path=${encodeURIComponent(dirPath.trim())}`);
        const data = await res.json();

        if (!data.exists) {
            badge.className = 'dir-stats-badge not-found';
            badge.innerHTML = '<span>⚠️ 目录不存在或无法访问</span>';
            return;
        }

        let breakdownList = [];
        if (data.format_counts) {
            for (const [ext, cnt] of Object.entries(data.format_counts)) {
                breakdownList.push(`${ext.toUpperCase().replace('.', '')}: ${cnt}`);
            }
        }
        const breakdownStr = breakdownList.length > 0 ? ` (${breakdownList.slice(0, 4).join(', ')})` : '';

        badge.className = 'dir-stats-badge';
        badge.innerHTML = `
            <span>📁 检测到 <strong>${data.total_files}</strong> 首音频</span>
            <span>·</span>
            <span>${formatBytes(data.total_size)}</span>
            <span class="stats-breakdown">${breakdownStr}</span>
        `;
    } catch (e) {
        badge.className = 'dir-stats-badge not-found';
        badge.innerHTML = '<span>⚠️ 查询目录文件失败</span>';
    }
}

function initDirStatsWatchers() {
    const configs = [
        { inputId: 'fmt-music-dir', badgeId: 'fmt-dir-stats', btnId: 'btn-refresh-fmt-dir' },
        { inputId: 'dedup-music-dir', badgeId: 'dedup-dir-stats', btnId: 'btn-refresh-dedup-dir' },
        { inputId: 'lossless-music-dir', badgeId: 'lossless-dir-stats', btnId: 'btn-refresh-lossless-dir' }
    ];

    configs.forEach(cfg => {
        const input = document.getElementById(cfg.inputId);
        const btn = document.getElementById(cfg.btnId);
        if (!input) return;

        let timer = null;
        input.addEventListener('input', () => {
            clearTimeout(timer);
            timer = setTimeout(() => {
                queryDirStats(input.value, cfg.badgeId);
            }, 600);
        });

        if (btn) {
            btn.addEventListener('click', () => {
                queryDirStats(input.value, cfg.badgeId);
            });
        }
    });
}

// ==================== 模态框交互逻辑 ====================

let modalResolve = null;

function openActionModal({ title, desc, showOutputDir = false, showOptions = false, showDeleteOptions = false, defaultOutputDir = '' }) {
    const modal = document.getElementById('action-modal');
    const titleEl = document.getElementById('modal-title');
    const descEl = document.getElementById('modal-desc');
    const outputGroup = document.getElementById('modal-group-output-dir');
    const optionsGroup = document.getElementById('modal-group-options');
    const deleteGroup = document.getElementById('modal-delete-options');
    const outputInput = document.getElementById('modal-output-dir');

    titleEl.textContent = title;
    descEl.textContent = desc;
    outputInput.value = defaultOutputDir || (state.system && state.system.output_dir) || '/output';

    if (showOutputDir) outputGroup.classList.remove('hidden'); else outputGroup.classList.add('hidden');
    if (showOptions) optionsGroup.classList.remove('hidden'); else optionsGroup.classList.add('hidden');
    if (showDeleteOptions) deleteGroup.classList.remove('hidden'); else deleteGroup.classList.add('hidden');

    modal.classList.remove('hidden');

    return new Promise((resolve) => {
        modalResolve = resolve;
    });
}

function closeActionModal(result = null) {
    const modal = document.getElementById('action-modal');
    modal.classList.add('hidden');
    if (modalResolve) {
        modalResolve(result);
        modalResolve = null;
    }
}

function initActionModalEvents() {
    const modal = document.getElementById('action-modal');
    const closeBtn = document.getElementById('btn-modal-close');
    const cancelBtn = document.getElementById('btn-modal-cancel');
    const confirmBtn = document.getElementById('btn-modal-confirm');

    closeBtn.addEventListener('click', () => closeActionModal(null));
    cancelBtn.addEventListener('click', () => closeActionModal(null));
    modal.addEventListener('click', (e) => {
        if (e.target === modal) closeActionModal(null);
    });

    confirmBtn.addEventListener('click', () => {
        const outputDir = document.getElementById('modal-output-dir').value.trim();
        const keepStructure = document.getElementById('modal-keep-structure').checked;
        const fixExtension = document.getElementById('modal-fix-ext').checked;
        const deleteTypeRadio = document.querySelector('input[name="modal-delete-type"]:checked');
        const deleteType = deleteTypeRadio ? deleteTypeRadio.value : 'recycle';

        closeActionModal({
            confirmed: true,
            outputDir,
            keepStructure,
            fixExtension,
            deleteType
        });
    });
}

// ==================== 初始化与系统状态 ====================

async function fetchSystemStatus() {
    try {
        const res = await fetch(API.systemStatus);
        const data = await res.json();
        state.system = data;

        // 更新状态灯
        const ffmpegChip = document.getElementById('chip-ffmpeg');
        if (data.has_ffmpeg) ffmpegChip.classList.add('active');

        const chromaChip = document.getElementById('chip-chromaprint');
        if (data.has_chromaprint) chromaChip.classList.add('active');

        const musicLabel = document.getElementById('music-dir-label');
        const musicChip = document.getElementById('chip-music-dir');
        
        const dirs = data.music_dirs || [];
        const validDirs = dirs.filter(d => d.exists);
        const totalStats = data.total_music_stats || data.music_stats || { total_files: 0, total_size: 0, exists: false };

        if (validDirs.length > 0) {
            musicLabel.textContent = `已挂载 ${validDirs.length} 个目录`;
            musicChip.classList.add('active');
            const dirDetails = validDirs.map(d => `• ${d.path} (${d.total_files} 首, ${formatBytes(d.total_size)})`).join('\n');
            musicChip.title = `已挂载 ${validDirs.length} 个音乐目录 (共 ${totalStats.total_files} 首音频，占用 ${formatBytes(totalStats.total_size)}):\n${dirDetails}`;
        } else if (data.music_stats && data.music_stats.exists) {
            musicLabel.textContent = `已挂载 1 个目录`;
            musicChip.classList.add('active');
            musicChip.title = `挂载音乐库: ${data.music_dir} (共 ${data.music_stats.total_files} 首音频，占用 ${formatBytes(data.music_stats.total_size)})`;
        } else {
            musicLabel.textContent = '未挂载';
            musicChip.classList.remove('active');
            musicChip.title = '未检测到有效的音乐目录，请在设置中授权或检查挂载路径';
        }

        // 初始化/填充 3 个面板的下拉选择器
        setupDirectorySelects(data);

        // 检查是否有正在运行的任务
        if (data.tasks.format_scan && data.tasks.format_scan.status === 'running') {
            startFormatPolling();
        }
        if (data.tasks.dedup_compute && data.tasks.dedup_compute.status === 'running') {
            startDedupPolling();
        }
        if (data.tasks.lossless_scan && data.tasks.lossless_scan.status === 'running') {
            startLosslessPolling();
        }
    } catch (e) {
        console.error('获取系统状态失败:', e);
    }
}

// 统一配置与联动多目录下拉框
function setupDirectorySelects(systemData) {
    const panels = [
        { selectId: 'fmt-dir-select', inputId: 'fmt-music-dir', statsId: 'fmt-dir-stats', refreshId: 'btn-refresh-fmt-dir' },
        { selectId: 'dedup-dir-select', inputId: 'dedup-music-dir', statsId: 'dedup-dir-stats', refreshId: 'btn-refresh-dedup-dir' },
        { selectId: 'lossless-dir-select', inputId: 'lossless-music-dir', statsId: 'lossless-dir-stats', refreshId: 'btn-refresh-lossless-dir' }
    ];

    const dirs = systemData.music_dirs || [];
    const totalStats = systemData.total_music_stats || systemData.music_stats || { total_files: 0, total_size: 0 };
    const defaultDir = (dirs.length > 0 ? dirs[0].path : systemData.music_dir) || '/music';

    panels.forEach(p => {
        const sel = document.getElementById(p.selectId);
        const inp = document.getElementById(p.inputId);
        const refreshBtn = document.getElementById(p.refreshId);
        if (!sel || !inp) return;

        // 如果下拉框尚未填充过选项
        if (sel.getAttribute('data-initialized') !== 'true') {
            sel.innerHTML = '';

            // 1. 全部音乐库选项
            const optAll = document.createElement('option');
            optAll.value = '__ALL__';
            optAll.textContent = `🌐 全部音乐库 (合并扫描 - 共 ${totalStats.total_files} 首)`;
            sel.appendChild(optAll);

            // 2. 各个具体目录
            dirs.forEach(d => {
                const opt = document.createElement('option');
                opt.value = d.path;
                opt.textContent = `📁 ${d.path} (${d.total_files} 首 / ${formatBytes(d.total_size)})`;
                sel.appendChild(opt);
            });

            // 3. 自定义输入选项
            const optCustom = document.createElement('option');
            optCustom.value = '__CUSTOM__';
            optCustom.textContent = '✏️ 自定义路径 / 手动输入...';
            sel.appendChild(optCustom);

            // 默认选中：若只有1个目录则选中该目录，若有多个目录则默认选中第1个或全部
            if (dirs.length > 1) {
                sel.value = '__ALL__';
                inp.value = '__ALL__';
            } else if (dirs.length === 1) {
                sel.value = dirs[0].path;
                inp.value = dirs[0].path;
            } else {
                inp.value = defaultDir;
            }

            sel.setAttribute('data-initialized', 'true');

            // 下拉选择切换事件
            sel.addEventListener('change', () => {
                const val = sel.value;
                if (val === '__CUSTOM__') {
                    inp.focus();
                    inp.select();
                } else {
                    inp.value = val;
                }
                syncAllPanelsDir(val, inp.value);
                queryDirStats(inp.value, p.statsId);
            });

            // 输入框修改事件
            inp.addEventListener('input', () => {
                const curVal = inp.value.trim();
                // 检查是否匹配某个预设
                let matched = false;
                for (let i = 0; i < sel.options.length; i++) {
                    if (sel.options[i].value === curVal) {
                        sel.selectedIndex = i;
                        matched = true;
                        break;
                    }
                }
                if (!matched) {
                    sel.value = '__CUSTOM__';
                }
            });

            inp.addEventListener('change', () => {
                queryDirStats(inp.value, p.statsId);
            });

            if (refreshBtn) {
                refreshBtn.addEventListener('click', () => {
                    queryDirStats(inp.value, p.statsId);
                });
            }
        }

        // 触发初次目录统计显示
        queryDirStats(inp.value || defaultDir, p.statsId);
    });
}

// 跨面板同步目录选择状态
function syncAllPanelsDir(selectedVal, inputVal) {
    const panels = [
        { selectId: 'fmt-dir-select', inputId: 'fmt-music-dir', statsId: 'fmt-dir-stats' },
        { selectId: 'dedup-dir-select', inputId: 'dedup-music-dir', statsId: 'dedup-dir-stats' },
        { selectId: 'lossless-dir-select', inputId: 'lossless-music-dir', statsId: 'lossless-dir-stats' }
    ];

    panels.forEach(p => {
        const sel = document.getElementById(p.selectId);
        const inp = document.getElementById(p.inputId);
        if (sel && sel.value !== selectedVal) {
            sel.value = selectedVal;
        }
        if (inp && inp.value !== inputVal && selectedVal !== '__CUSTOM__') {
            inp.value = inputVal;
            queryDirStats(inputVal, p.statsId);
        }
    });
}

// 查询指定路径音频文件统计并渲染徽章
async function queryDirStats(dirPath, badgeElementId) {
    const badge = document.getElementById(badgeElementId);
    if (!badge) return;

    dirPath = (dirPath || '').trim();
    if (!dirPath) {
        badge.innerHTML = '<span class="stats-loading">⚠️ 请先指定或选择待扫描的目录</span>';
        return;
    }

    badge.innerHTML = '<span class="stats-loading">🔍 正在查询目录音频文件...</span>';

    try {
        const res = await fetch(`${API.dirStats}?path=${encodeURIComponent(dirPath)}`);
        if (!res.ok) {
            badge.innerHTML = '<span class="stats-err">❌ 目录不可访问或无读取权限</span>';
            return;
        }
        const data = await res.json();
        if (!data.exists) {
            badge.innerHTML = `<span class="stats-err" title="${data.path}">⚠️ 目录不存在或无法访问</span>`;
            return;
        }

        const sizeStr = formatBytes(data.total_size);
        if (data.total_files === 0) {
            badge.innerHTML = `<span class="stats-empty">📭 目录内未发现支持的音频文件</span>`;
        } else {
            const extSummary = Object.entries(data.format_counts || {})
                .sort((a, b) => b[1] - a[1])
                .slice(0, 4)
                .map(([ext, count]) => `${ext.replace('.', '')}: ${count}`)
                .join(', ');
            
            badge.innerHTML = `<span class="stats-ok" title="${extSummary ? '格式分布: ' + extSummary : ''}">📁 已检测到 <strong>${data.total_files}</strong> 首音频 (约 ${sizeStr})</span>`;
        }
    } catch (e) {
        badge.innerHTML = '<span class="stats-err">⚠️ 查询目录状态失败</span>';
    }
}

// ==================== 选项卡切换 ====================

function initNavTabs() {
    const tabs = document.querySelectorAll('.nav-tab');
    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            const isUnlocked = state.authStatus && state.authStatus.unlocked;
            const targetId = tab.getAttribute('data-tab');

            // 如果系统未解锁且点击的是受保护工具Tab
            if (!isUnlocked && targetId !== 'tab-feiniu') {
                showToast('系统尚未解锁：请先登录本地账号或连接飞牛音乐', 'warning');
                showAuthPortal();
                return;
            }

            tabs.forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));

            hideAuthPortal();

            tab.classList.add('active');
            const targetEl = document.getElementById(targetId);
            if (targetEl) targetEl.classList.add('active');

            if (targetId === 'tab-dedup') {
                loadDuplicateGroups();
            } else if (targetId === 'tab-format') {
                loadFormatRecords();
            } else if (targetId === 'tab-lossless') {
                loadLosslessRecords();
            } else if (targetId === 'tab-playlist') {
                loadPlaylistHistory();
            } else if (targetId === 'tab-feiniu') {
                checkFeiNiuStatus();
                loadFeiNiuPlaylists();
            }
        });
    });
}

// ==================== TAB 1: 格式检查器 ====================

function initFormatChecker() {
    const form = document.getElementById('form-format-scan');
    const workersSlider = document.getElementById('fmt-workers');
    const workersVal = document.getElementById('fmt-workers-val');

    workersSlider.addEventListener('input', () => {
        workersVal.textContent = workersSlider.value;
    });

    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const payload = {
            music_dir: document.getElementById('fmt-music-dir').value.trim() || undefined,
            workers: parseInt(workersSlider.value, 10)
        };

        try {
            const res = await fetch(API.formatScan, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });
            if (!res.ok) {
                const err = await res.json();
                showToast(err.detail || '启动格式扫描失败', 'error');
                return;
            }
            showToast('格式扫描任务已启动...', 'info');
            startFormatPolling();
        } catch (err) {
            showToast('请求格式扫描失败: ' + err.message, 'error');
        }
    });

    // 搜索过滤
    document.getElementById('fmt-filter-query').addEventListener('input', (e) => {
        renderFormatTable(e.target.value);
    });

    // 导出报告
    document.getElementById('btn-export-csv').addEventListener('click', () => {
        window.open(API.formatExport, '_blank');
    });

    // 全选/反选
    const checkAll = document.getElementById('fmt-check-all');
    checkAll.addEventListener('change', () => {
        const checkboxes = document.querySelectorAll('.fmt-item-check');
        checkboxes.forEach(cb => cb.checked = checkAll.checked);
    });

    // 1. 批量修正后缀 (修改)
    document.getElementById('btn-batch-fix-rename').addEventListener('click', async () => {
        const selected = Array.from(document.querySelectorAll('.fmt-item-check:checked')).map(cb => cb.value);
        if (selected.length === 0) {
            showToast('请先勾选需要修正后缀的文件', 'warn');
            return;
        }

        if (!confirm(`确定要直接原地重命名修正选中的 ${selected.length} 个文件后缀吗？`)) return;

        try {
            const res = await fetch(API.formatActionBatch, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ file_paths: selected, action: 'rename_fix' })
            });
            const data = await res.json();
            showToast(`批量修正完成：处理了 ${data.total} 个文件`, 'success');
            loadFormatRecords();
        } catch (e) {
            showToast('批量修正失败: ' + e.message, 'error');
        }
    });

    // 2. 批量复制到目录
    document.getElementById('btn-batch-copy').addEventListener('click', async () => {
        const selected = Array.from(document.querySelectorAll('.fmt-item-check:checked')).map(cb => cb.value);
        if (selected.length === 0) {
            showToast('请先勾选需要复制的文件', 'warn');
            return;
        }

        const modalRes = await openActionModal({
            title: '批量复制异常音频',
            desc: `已选择 ${selected.length} 个文件，将复制到指定目录`,
            showOutputDir: true,
            showOptions: true
        });
        if (!modalRes || !modalRes.confirmed) return;

        try {
            const res = await fetch(API.formatActionBatch, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    file_paths: selected,
                    action: 'copy',
                    output_dir: modalRes.outputDir,
                    keep_structure: modalRes.keepStructure,
                    fix_extension: modalRes.fixExtension
                })
            });
            const data = await res.json();
            showToast(`批量复制完成：处理了 ${data.total} 个文件`, 'success');
            loadFormatRecords();
        } catch (e) {
            showToast('批量复制失败: ' + e.message, 'error');
        }
    });

    // 3. 批量移动到目录
    document.getElementById('btn-batch-move').addEventListener('click', async () => {
        const selected = Array.from(document.querySelectorAll('.fmt-item-check:checked')).map(cb => cb.value);
        if (selected.length === 0) {
            showToast('请先勾选需要移动的文件', 'warn');
            return;
        }

        const modalRes = await openActionModal({
            title: '批量移动异常音频',
            desc: `已选择 ${selected.length} 个文件，将从原曲库移出到指定目录`,
            showOutputDir: true,
            showOptions: true
        });
        if (!modalRes || !modalRes.confirmed) return;

        try {
            const res = await fetch(API.formatActionBatch, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    file_paths: selected,
                    action: 'move',
                    output_dir: modalRes.outputDir,
                    keep_structure: modalRes.keepStructure,
                    fix_extension: modalRes.fixExtension
                })
            });
            const data = await res.json();
            showToast(`批量移动完成：处理了 ${data.total} 个文件`, 'success');
            loadFormatRecords();
        } catch (e) {
            showToast('批量移动失败: ' + e.message, 'error');
        }
    });

    // 4. 批量删除 / 回收
    document.getElementById('btn-batch-delete').addEventListener('click', async () => {
        const selected = Array.from(document.querySelectorAll('.fmt-item-check:checked')).map(cb => cb.value);
        if (selected.length === 0) {
            showToast('请先勾选需要清理的文件', 'warn');
            return;
        }

        const modalRes = await openActionModal({
            title: '批量清理异常文件',
            desc: `已选择 ${selected.length} 个文件，请选择清理方式：`,
            showDeleteOptions: true
        });
        if (!modalRes || !modalRes.confirmed) return;

        if (modalRes.deleteType === 'delete') {
            if (!confirm(`【高危警告】确定彻底删除选中的 ${selected.length} 个文件吗？数据无法找回！`)) return;
        }

        try {
            const res = await fetch(API.formatActionBatch, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    file_paths: selected,
                    action: modalRes.deleteType
                })
            });
            const data = await res.json();
            showToast(`批量清理完成：共处理 ${data.total} 个文件`, 'success');
            loadFormatRecords();
        } catch (e) {
            showToast('批量清理失败: ' + e.message, 'error');
        }
    });
}

function startFormatPolling() {
    const progressContainer = document.getElementById('format-progress-container');
    const startBtn = document.getElementById('btn-start-format');
    progressContainer.classList.remove('hidden');
    startBtn.disabled = true;

    if (state.formatPollingTimer) clearInterval(state.formatPollingTimer);

    state.formatPollingTimer = setInterval(async () => {
        try {
            const res = await fetch(API.formatProgress);
            const p = await res.json();

            const total = p.total || 0;
            const scanned = p.scanned || 0;
            const percent = total > 0 ? Math.round((scanned / total) * 100) : 0;

            document.getElementById('fmt-progress-percent').textContent = `${percent}% (${scanned}/${total})`;
            document.getElementById('fmt-progress-bar').style.width = `${percent}%`;
            document.getElementById('fmt-current-file').textContent = p.current_file ? `当前处理: ${p.current_file}` : '就绪';

            document.getElementById('stat-fmt-total').textContent = scanned;
            document.getElementById('stat-fmt-ok').textContent = Math.max(0, scanned - p.mismatched);
            document.getElementById('stat-fmt-mismatch').textContent = p.mismatched;
            document.getElementById('stat-fmt-fake').textContent = p.fake_or_corrupt;

            if (p.status === 'done' || p.status === 'idle') {
                clearInterval(state.formatPollingTimer);
                state.formatPollingTimer = null;
                startBtn.disabled = false;
                progressContainer.classList.add('hidden');
                showToast(`扫描完成！共检查 ${scanned} 首音频，发现 ${p.mismatched} 项异常`, 'success');
                loadFormatRecords();
            }
        } catch (e) {
            console.error('轮询格式扫描进度失败:', e);
        }
    }, 800);
}

async function loadFormatRecords() {
    try {
        const res = await fetch(`${API.formatRecords}?mismatch_only=false`);
        const data = await res.json();
        state.formatRecords = data.records || [];
        renderFormatTable();
    } catch (e) {
        console.error('拉取格式记录失败:', e);
    }
}

function renderFormatTable(query = '') {
    const tbody = document.getElementById('fmt-records-body');
    const countEl = document.getElementById('fmt-record-count');

    let records = state.formatRecords.filter(r => r.is_mismatch === 1);
    if (query.trim()) {
        const q = query.toLowerCase();
        records = records.filter(r => r.file_name.toLowerCase().includes(q) || r.details.toLowerCase().includes(q));
    }

    countEl.textContent = `共发现 ${records.length} 项异常`;

    if (records.length === 0) {
        tbody.innerHTML = `
            <tr>
                <td colspan="8" class="empty-state">
                    <div class="empty-icon">🎉</div>
                    <p>曲库健康！未发现任何格式不一致或伪装音频</p>
                </td>
            </tr>
        `;
        return;
    }

    tbody.innerHTML = records.map(r => {
        let fmtBadgeClass = `fmt-${r.detected_format}`;
        if (!r.is_audio) fmtBadgeClass = 'fmt-corrupt';

        const encPath = encodeURIComponent(r.file_path);
        const encName = encodeURIComponent(r.file_name);

        return `
            <tr>
                <td><input type="checkbox" class="fmt-item-check" value="${escapeHtml(r.file_path)}"></td>
                <td>
                    <div class="song-main-info">
                        <span class="song-filename">${escapeHtml(r.file_name)}</span>
                        <span class="song-path">${escapeHtml(r.file_path)}</span>
                    </div>
                </td>
                <td><span class="badge-fmt">${r.current_ext || '无'}</span></td>
                <td><span class="badge-fmt ${fmtBadgeClass}">${r.detected_format.toUpperCase()}</span></td>
                <td><strong style="color: var(--color-success); font-family: var(--font-mono);">${r.suggested_ext}</strong></td>
                <td style="font-size: 12px; color: var(--text-muted);">${escapeHtml(r.details)}</td>
                <td><span style="font-size: 11px; color: var(--text-sub);">${escapeHtml(r.status || '-')}</span></td>
                <td class="action-cell">
                    <div class="action-btn-group">
                        <button class="btn btn-success btn-sm" onclick="handleSingleFormatAction('${encPath}', 'rename_fix', '${r.suggested_ext}')" title="原地修改/修正后缀">
                            ✏️ 修正
                        </button>
                        <button class="btn btn-secondary btn-sm" onclick="handleSingleFormatAction('${encPath}', 'copy', '${r.suggested_ext}')" title="复制到输出目录">
                            📋 复制
                        </button>
                        <button class="btn btn-secondary btn-sm" onclick="handleSingleFormatAction('${encPath}', 'move', '${r.suggested_ext}')" title="移动到输出目录">
                            📦 移动
                        </button>
                        <button class="btn btn-danger-outline btn-sm" onclick="handleSingleFormatAction('${encPath}', 'recycle')" title="安全移入回收站">
                            ♻️
                        </button>
                        ${r.is_audio ? `
                        <button class="btn btn-secondary btn-sm" onclick="playAudio('${encPath}', decodeURIComponent('${encName}'))" title="试听音频">
                            ▶
                        </button>` : ''}
                    </div>
                </td>
            </tr>
        `;
    }).join('');
}

// 单项操作处理器
window.handleSingleFormatAction = async function(encodedPath, action, suggestedExt) {
    const filePath = decodeURIComponent(encodedPath);

    if (action === 'rename_fix') {
        if (!confirm(`确定要将文件后缀直接更正为 ${suggestedExt} 吗？\n${filePath}`)) return;
        try {
            const res = await fetch(API.formatActionSingle, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ file_path: filePath, action: 'rename_fix', suggested_ext: suggestedExt })
            });
            const data = await res.json();
            if (!res.ok) {
                showToast(data.detail || '修正失败', 'error');
                return;
            }
            showToast('后缀修正成功！', 'success');
            loadFormatRecords();
        } catch (e) {
            showToast('修正失败: ' + e.message, 'error');
        }
    } else if (action === 'copy') {
        const modalRes = await openActionModal({
            title: '复制异常音频',
            desc: `文件：${filePath}`,
            showOutputDir: true,
            showOptions: true
        });
        if (!modalRes || !modalRes.confirmed) return;

        try {
            const res = await fetch(API.formatActionSingle, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    file_path: filePath,
                    action: 'copy',
                    output_dir: modalRes.outputDir,
                    keep_structure: modalRes.keepStructure,
                    fix_extension: modalRes.fixExtension,
                    suggested_ext: suggestedExt
                })
            });
            const data = await res.json();
            if (!res.ok) {
                showToast(data.detail || '复制失败', 'error');
                return;
            }
            showToast('已成功复制到目标目录！', 'success');
            loadFormatRecords();
        } catch (e) {
            showToast('复制失败: ' + e.message, 'error');
        }
    } else if (action === 'move') {
        const modalRes = await openActionModal({
            title: '移动异常音频',
            desc: `文件将从原目录移出：${filePath}`,
            showOutputDir: true,
            showOptions: true
        });
        if (!modalRes || !modalRes.confirmed) return;

        try {
            const res = await fetch(API.formatActionSingle, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    file_path: filePath,
                    action: 'move',
                    output_dir: modalRes.outputDir,
                    keep_structure: modalRes.keepStructure,
                    fix_extension: modalRes.fixExtension,
                    suggested_ext: suggestedExt
                })
            });
            const data = await res.json();
            if (!res.ok) {
                showToast(data.detail || '移动失败', 'error');
                return;
            }
            showToast('已成功移动到目标目录！', 'success');
            loadFormatRecords();
        } catch (e) {
            showToast('移动失败: ' + e.message, 'error');
        }
    } else if (action === 'recycle') {
        if (!confirm(`确定将该异常文件移入安全回收站吗？\n${filePath}`)) return;
        try {
            const res = await fetch(API.formatActionSingle, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ file_path: filePath, action: 'recycle' })
            });
            const data = await res.json();
            if (!res.ok) {
                showToast(data.detail || '移入回收站失败', 'error');
                return;
            }
            showToast('已移入安全回收站', 'success');
            loadFormatRecords();
        } catch (e) {
            showToast('清理失败: ' + e.message, 'error');
        }
    }
};

window.fixSingleFormat = async function(encodedPath, suggestedExt) {
    window.handleSingleFormatAction(encodedPath, 'rename_fix', suggestedExt);
};

// ==================== TAB 2: 指纹去重 (Songloft 引擎) ====================

function initDeduplicator() {
    const toleranceSlider = document.getElementById('dedup-tolerance');
    const toleranceVal = document.getElementById('dedup-tolerance-val');
    const workersSlider = document.getElementById('dedup-workers');
    const workersVal = document.getElementById('dedup-workers-val');

    toleranceSlider.addEventListener('input', () => {
        toleranceVal.textContent = `${toleranceSlider.value}s`;
        loadDuplicateGroups();
    });

    workersSlider.addEventListener('input', () => {
        workersVal.textContent = workersSlider.value;
    });

    // 触发去重计算
    async function triggerCompute(mode) {
        const payload = {
            mode: mode,
            music_dir: document.getElementById('dedup-music-dir').value.trim() || undefined,
            workers: parseInt(workersSlider.value, 10)
        };
        try {
            const res = await fetch(API.dedupCompute, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });
            if (!res.ok) {
                const err = await res.json();
                showToast(err.detail || '启动指纹任务失败', 'error');
                return;
            }
            showToast(`已启动指纹计算 (${mode === 'missing' ? '计算缺失' : mode === 'recompute_all' ? '全量重算' : '重试失败'})...`, 'info');
            startDedupPolling();
        } catch (e) {
            showToast('启动指纹任务失败: ' + e.message, 'error');
        }
    }

    document.getElementById('btn-dedup-missing').addEventListener('click', () => triggerCompute('missing'));
    document.getElementById('btn-dedup-recompute').addEventListener('click', () => {
        if (confirm('全量重新计算将清空已有缓存指纹，曲库较大时可能耗费较多 CPU 时间。确定继续吗？')) {
            triggerCompute('recompute_all');
        }
    });
    document.getElementById('btn-dedup-retry').addEventListener('click', () => triggerCompute('retry_failed'));

    // 取消计算
    document.getElementById('btn-dedup-cancel').addEventListener('click', async () => {
        try {
            await fetch(API.dedupCancel, { method: 'POST' });
            showToast('正在中止指纹计算...', 'warn');
        } catch (e) {
            console.error(e);
        }
    });

    // 刷新重复列表
    document.getElementById('btn-refresh-groups').addEventListener('click', loadDuplicateGroups);

    // 快捷勾选工具
    const selectRedundantBtn = document.getElementById('btn-dedup-select-redundant');
    if (selectRedundantBtn) {
        selectRedundantBtn.addEventListener('click', () => {
            const checks = document.querySelectorAll('.dedup-item-check');
            checks.forEach(cb => {
                const tr = cb.closest('tr');
                cb.checked = tr ? !tr.classList.contains('is-recommended') : false;
            });
            showToast('已选中所有标记为冗余的音频', 'info');
        });
    }

    const selectNoneBtn = document.getElementById('btn-dedup-select-none');
    if (selectNoneBtn) {
        selectNoneBtn.addEventListener('click', () => {
            const checks = document.querySelectorAll('.dedup-item-check');
            checks.forEach(cb => cb.checked = false);
            showToast('已清空所有勾选', 'info');
        });
    }

    // 顶部操作栏：智能一键清理与批量清理
    const cleanRecycleBtn = document.getElementById('btn-clean-recycle');
    if (cleanRecycleBtn) {
        cleanRecycleBtn.addEventListener('click', () => cleanRecommended('recycle'));
    }

    const cleanBatchRecycleBtn = document.getElementById('btn-dedup-batch-recycle-selected');
    if (cleanBatchRecycleBtn) {
        cleanBatchRecycleBtn.addEventListener('click', () => cleanSelectedDuplicates('recycle'));
    }

    const cleanDeleteBtn = document.getElementById('btn-clean-delete');
    if (cleanDeleteBtn) {
        cleanDeleteBtn.addEventListener('click', () => {
            if (confirm('【高危提醒】这将永久彻底删除所有重复组中被判定为冗余的音频文件，无法撤销！建议使用移入回收站。确定彻底删除吗？')) {
                cleanRecommended('delete');
            }
        });
    }
}

function startDedupPolling() {
    const progressContainer = document.getElementById('dedup-progress-container');
    const cancelBtn = document.getElementById('btn-dedup-cancel');
    progressContainer.classList.remove('hidden');
    cancelBtn.classList.remove('hidden');

    if (state.dedupPollingTimer) clearInterval(state.dedupPollingTimer);

    state.dedupPollingTimer = setInterval(async () => {
        try {
            const res = await fetch(API.dedupProgress);
            const p = await res.json();

            const total = p.total || 0;
            const done = p.computed + p.failed;
            const percent = total > 0 ? Math.round((done / total) * 100) : 0;

            document.getElementById('dedup-progress-percent').textContent = `${percent}% (${done}/${total})`;
            document.getElementById('dedup-progress-bar').style.width = `${percent}%`;
            document.getElementById('dedup-current-file').textContent = p.current_file ? `正在处理: ${p.current_file} (成功: ${p.computed}, 失败: ${p.failed})` : '正在提取声学指纹...';

            if (p.status === 'done' || p.status === 'cancelled' || p.status === 'idle') {
                clearInterval(state.dedupPollingTimer);
                state.dedupPollingTimer = null;
                progressContainer.classList.add('hidden');
                cancelBtn.classList.add('hidden');
                showToast(`指纹计算完毕！成功提取 ${p.computed} 首音频指纹`, 'success');
                loadDuplicateGroups();
            }
        } catch (e) {
            console.error('轮询指纹进度失败:', e);
        }
    }, 1000);
}

async function loadDuplicateGroups() {
    const tolerance = document.getElementById('dedup-tolerance').value;
    try {
        const res = await fetch(`${API.dedupGroups}?tolerance=${tolerance}`);
        const data = await res.json();
        state.duplicateGroups = data.groups || [];

        // 更新统计
        document.getElementById('stat-dedup-groups').textContent = data.group_count;
        document.getElementById('stat-dedup-songs').textContent = data.total_duplicate_songs;
        document.getElementById('stat-dedup-wasted').textContent = formatBytes(data.total_wasted_size);
        document.getElementById('badge-groups-count').textContent = `${data.group_count} 组重复`;

        renderDuplicateGroups();
    } catch (e) {
        console.error('加载重复分组失败:', e);
    }
}

function renderDuplicateGroups() {
    const container = document.getElementById('duplicate-groups-list');
    const groups = state.duplicateGroups;

    if (!groups || groups.length === 0) {
        container.innerHTML = `
            <div class="empty-state glass-panel">
                <div class="empty-icon">✨</div>
                <h3>曲库无重复音频！</h3>
                <p>基于 AcoustID 声学指纹与 30s 时长容差比对，未发现相同歌曲的不同副本。</p>
            </div>
        `;
        return;
    }

    container.innerHTML = groups.map((g, idx) => {
        return `
            <div class="group-card" id="${g.group_id}">
                <div class="group-header">
                    <div class="group-title">
                        <span>#${idx + 1} 重复歌曲组 (${g.songs.length} 首)</span>
                        <span class="group-fp-hash" title="AcoustID Base64 指纹: ${g.fingerprint}">指纹: ${g.fingerprint.slice(0, 16)}...</span>
                    </div>
                    <div class="group-meta">
                        <span>总占用: <strong>${formatBytes(g.total_size)}</strong></span>
                        <span class="wasted-tag">可节省: ${formatBytes(g.wasted_size)}</span>
                    </div>
                </div>

                <table class="group-songs-table">
                    <tbody>
                        ${g.songs.map(s => {
                            const isLossless = ['flac', 'wav', 'ape', 'alac', 'dsf', 'dff'].includes(s.format.toLowerCase());
                            const kbps = s.bitrate > 0 ? `${Math.round(s.bitrate / 1000)} kbps` : '-';
                            const hz = s.sample_rate > 0 ? `${(s.sample_rate / 1000).toFixed(1)} kHz` : '-';

                            return `
                                <tr class="${s.is_recommended_keep ? 'is-recommended' : ''}">
                                    <td width="30">
                                        <input type="checkbox" class="dedup-item-check" data-group="${g.group_id}" value="${escapeHtml(s.file_path)}" ${!s.is_recommended_keep ? 'checked' : ''}>
                                    </td>
                                    <td>
                                        <div class="song-main-info">
                                            <div class="song-filename">
                                                <span>${escapeHtml(s.file_name)}</span>
                                                ${isLossless ? '<span class="tag-lossless">无损</span>' : ''}
                                                ${s.is_recommended_keep ? '<span class="tag-recommended">⭐ 推荐保留 (最佳音质)</span>' : ''}
                                            </div>
                                            <div class="song-path">${escapeHtml(s.file_path)}</div>
                                        </div>
                                    </td>
                                    <td>
                                        <div class="song-audio-specs">
                                            <span class="badge-fmt fmt-${s.format}">${s.format.toUpperCase()}</span>
                                            <span class="spec-bitrate">${kbps}</span>
                                            <span>${hz}</span>
                                            <span>${formatDuration(s.duration)}</span>
                                            <span>${formatBytes(s.file_size)}</span>
                                        </div>
                                    </td>
                                    <td class="action-cell">
                                        <div class="action-btn-group">
                                            <button class="btn btn-secondary btn-sm" onclick="playAudio('${encodeURIComponent(s.file_path)}', '${escapeHtml(s.file_name)}')" title="试听音频">
                                                ▶ 试听
                                            </button>
                                            <button class="btn btn-danger-outline btn-sm" onclick="cleanSingleFile('${encodeURIComponent(s.file_path)}')" title="移入回收站">
                                                ♻️ 回收
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            `;
                        }).join('')}
                    </tbody>
                </table>
            </div>
        `;
    }).join('');
}

window.cleanSingleFile = async function(encodedPath) {
    const filePath = decodeURIComponent(encodedPath);
    if (!confirm(`确定要将文件移入回收站吗？\n${filePath}`)) return;

    try {
        const res = await fetch(API.dedupClean, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ file_paths: [filePath], action: 'recycle' })
        });
        showToast('已移入回收站', 'success');
        loadDuplicateGroups();
    } catch (e) {
        showToast('清理失败: ' + e.message, 'error');
    }
};

async function cleanRecommended(action = 'recycle') {
    const tolerance = document.getElementById('dedup-tolerance').value;
    try {
        const res = await fetch(`${API.dedupCleanRecommended}?action=${action}&tolerance=${tolerance}`, {
            method: 'POST'
        });
        const data = await res.json();
        showToast(`智能清理完成！共清理 ${data.total} 个冗余文件`, 'success');
        loadDuplicateGroups();
    } catch (e) {
        showToast('智能清理失败: ' + e.message, 'error');
    }
}

async function cleanSelectedDuplicates(action = 'recycle') {
    const selected = Array.from(document.querySelectorAll('.dedup-item-check:checked')).map(cb => cb.value);
    if (selected.length === 0) {
        showToast('请先勾选需要清理的重复文件', 'warn');
        return;
    }

    if (action === 'delete') {
        if (!confirm(`【高危警告】确定要彻底删除选中的 ${selected.length} 个文件吗？数据无法恢复！`)) return;
    } else {
        if (!confirm(`确定将选中的 ${selected.length} 个文件移入安全回收站吗？`)) return;
    }

    try {
        const res = await fetch(API.dedupClean, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ file_paths: selected, action: action })
        });
        const data = await res.json();
        showToast(`批量清理完成！共处理 ${data.total} 个文件`, 'success');
        loadDuplicateGroups();
    } catch (e) {
        showToast('批量清理失败: ' + e.message, 'error');
    }
}

// ==================== 音频试听播放器 ====================

window.playAudio = function(encodedPath, fileName) {
    const filePath = decodeURIComponent(encodedPath);
    const playerBar = document.getElementById('audio-player-bar');
    const trackName = document.getElementById('player-track-name');
    const trackPath = document.getElementById('player-track-path');
    const audioEl = document.getElementById('global-audio-element');

    trackName.textContent = fileName || '音频试听';
    trackPath.textContent = filePath;

    audioEl.src = `${API.audioStream}?path=${encodeURIComponent(filePath)}`;
    playerBar.classList.remove('hidden');
    audioEl.play().catch(e => console.log('自动播放失败，等待手动触发:', e));
};

document.getElementById('btn-close-player').addEventListener('click', () => {
    const playerBar = document.getElementById('audio-player-bar');
    const audioEl = document.getElementById('global-audio-element');
    audioEl.pause();
    playerBar.classList.add('hidden');
});

function escapeHtml(val) {
    if (val === null || val === undefined) return '';
    if (typeof val === 'object') {
        val = val.name || val.title || val.message || JSON.stringify(val);
    }
    const str = String(val);
    return str.replace(/&/g, '&amp;')
              .replace(/</g, '&lt;')
              .replace(/>/g, '&gt;')
              .replace(/"/g, '&quot;')
              .replace(/'/g, '&#039;');
}

function formatDuration(dur) {
    if (!dur || isNaN(dur)) return '--:--';
    let sec = Number(dur);
    if (sec > 1000) {
        sec = Math.floor(sec / 1000);
    }
    const m = Math.floor(sec / 60);
    const s = Math.floor(sec % 60);
    return `${m}:${s < 10 ? '0' : ''}${s}`;
}

function initThemeToggle() {
    const btn = document.getElementById('btn-toggle-theme');
    if (!btn) return;

    const savedTheme = localStorage.getItem('music_toolkit_theme') || 'dark';
    applyTheme(savedTheme);

    btn.addEventListener('click', () => {
        const isLight = document.body.classList.contains('light-theme');
        const nextTheme = isLight ? 'dark' : 'light';
        applyTheme(nextTheme);
        localStorage.setItem('music_toolkit_theme', nextTheme);
    });
}

function applyTheme(theme) {
    const btn = document.getElementById('btn-toggle-theme');
    if (theme === 'dark') {
        document.body.classList.remove('light-theme');
        document.body.classList.add('dark-theme');
        if (btn) btn.textContent = '🌙';
    } else {
        document.body.classList.remove('dark-theme');
        document.body.classList.add('light-theme');
        if (btn) btn.textContent = '☀️';
    }
}

// ==================== TAB 3: 真假无损鉴别 (FLAC/APE 频谱分析) ====================

function initLosslessChecker() {
    const form = document.getElementById('form-lossless-scan');
    const workersSlider = document.getElementById('lossless-workers');
    const workersVal = document.getElementById('lossless-workers-val');

    if (!form) return;

    workersSlider.addEventListener('input', () => {
        workersVal.textContent = workersSlider.value;
    });

    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const payload = {
            music_dir: document.getElementById('lossless-music-dir').value.trim() || undefined,
            workers: parseInt(workersSlider.value, 10)
        };

        try {
            const res = await fetch(API.losslessScan, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });
            if (!res.ok) {
                const err = await res.json();
                showToast(err.detail || '启动无损检测失败', 'error');
                return;
            }
            showToast('已启动真假无损频谱分析任务...', 'info');
            startLosslessPolling();
        } catch (err) {
            showToast('启动无损检测失败: ' + err.message, 'error');
        }
    });

    // 筛选
    document.getElementById('lossless-filter-select').addEventListener('change', () => {
        renderLosslessTable();
    });

    // 搜索
    document.getElementById('lossless-filter-query').addEventListener('input', () => {
        renderLosslessTable();
    });

    // 导出 CSV 报告
    document.getElementById('btn-export-lossless-csv').addEventListener('click', () => {
        window.open(API.losslessExport, '_blank');
    });
}

function startLosslessPolling() {
    const progressContainer = document.getElementById('lossless-progress-container');
    const startBtn = document.getElementById('btn-start-lossless');
    progressContainer.classList.remove('hidden');
    startBtn.disabled = true;

    if (state.losslessPollingTimer) clearInterval(state.losslessPollingTimer);

    state.losslessPollingTimer = setInterval(async () => {
        try {
            const res = await fetch(API.losslessProgress);
            const p = await res.json();

            const total = p.total || 0;
            const scanned = p.scanned || 0;
            const percent = total > 0 ? Math.round((scanned / total) * 100) : 0;

            document.getElementById('lossless-progress-percent').textContent = `${percent}% (${scanned}/${total})`;
            document.getElementById('lossless-progress-bar').style.width = `${percent}%`;
            document.getElementById('lossless-current-file').textContent = p.current_file ? `正在分析频谱: ${p.current_file} (真无损: ${p.true_lossless || 0}, 假无损: ${p.fake_lossless || 0})` : '正在进行 FFT 高频截断与泛音分析...';

            document.getElementById('stat-lossless-total').textContent = scanned;
            document.getElementById('stat-lossless-true').textContent = p.true_lossless || 0;
            document.getElementById('stat-lossless-fake').textContent = p.fake_lossless || 0;
            document.getElementById('stat-lossless-bad').textContent = p.failed || 0;

            if (p.status === 'done' || p.status === 'idle') {
                clearInterval(state.losslessPollingTimer);
                state.losslessPollingTimer = null;
                startBtn.disabled = false;
                progressContainer.classList.add('hidden');
                showToast(`无损分析完毕！共检测 ${scanned} 首音频，发现 ${p.fake_lossless || 0} 首疑似假无损`, 'success');
                loadLosslessRecords();
            }
        } catch (e) {
            console.error('轮询无损检测进度失败:', e);
        }
    }, 800);
}

async function loadLosslessRecords() {
    try {
        const res = await fetch(`${API.losslessRecords}?filter=all`);
        const data = await res.json();
        state.losslessRecords = data.records || [];
        renderLosslessTable();
    } catch (e) {
        console.error('拉取无损检测记录失败:', e);
    }
}

function renderLosslessTable() {
    const tbody = document.getElementById('lossless-records-body');
    const countEl = document.getElementById('lossless-record-count');
    const filter = document.getElementById('lossless-filter-select').value;
    const query = (document.getElementById('lossless-filter-query').value || '').trim().toLowerCase();

    let records = state.losslessRecords;
    if (filter === 'fake_only') {
        records = records.filter(r => r.grade === 'fake_320k' || r.grade === 'fake_low_bitrate');
    } else if (filter === 'true_only') {
        records = records.filter(r => r.grade === 'true_lossless' || r.grade === 'true_hires');
    }

    if (query) {
        records = records.filter(r => r.file_name.toLowerCase().includes(query) || r.details.toLowerCase().includes(query));
    }

    countEl.textContent = `共 ${records.length} 首音频`;

    if (records.length === 0) {
        tbody.innerHTML = `
            <tr>
                <td colspan="7" class="empty-state">
                    <div class="empty-icon">🎼</div>
                    <p>暂无符合条件的无损检测记录</p>
                </td>
            </tr>
        `;
        return;
    }

    tbody.innerHTML = records.map(r => {
        let gradeBadge = '';
        let cutoffStyle = 'color: var(--color-success); font-weight: 700;';

        if (r.grade === 'true_hires') {
            gradeBadge = '<span class="badge tag-quality-lossless">🏆 真 Hi-Res</span>';
        } else if (r.grade === 'true_lossless') {
            gradeBadge = '<span class="badge tag-quality-lossless">💎 真无损 (CD)</span>';
        } else if (r.grade === 'fake_320k') {
            gradeBadge = '<span class="badge tag-quality-fake">⚠️ 假无损 (320k)</span>';
            cutoffStyle = 'color: var(--color-warning); font-weight: 700;';
        } else if (r.grade === 'fake_low_bitrate') {
            gradeBadge = '<span class="badge tag-quality-low">🚫 劣质假无损 (128k)</span>';
            cutoffStyle = 'color: var(--color-danger); font-weight: 700;';
        } else {
            gradeBadge = '<span class="badge">未知</span>';
            cutoffStyle = 'color: var(--text-sub);';
        }

        const srKhz = r.sample_rate > 0 ? `${(r.sample_rate / 1000).toFixed(1)} kHz` : '-';
        const kbps = r.bitrate > 0 ? `${Math.round(r.bitrate / 1000)} kbps` : '-';
        const cutoffKhz = r.cutoff_freq_hz > 0 ? `${(r.cutoff_freq_hz / 1000).toFixed(1)} kHz` : '-';

        return `
            <tr>
                <td>
                    <div class="song-main-info">
                        <span class="song-filename">${escapeHtml(r.file_name)}</span>
                        <span class="song-path">${escapeHtml(r.file_path)}</span>
                    </div>
                </td>
                <td>
                    <div class="song-audio-specs">
                        <span class="badge-fmt fmt-${r.format}">${r.format.toUpperCase()}</span>
                        <span>${srKhz}</span>
                        <span class="spec-bitrate">${kbps}</span>
                    </div>
                </td>
                <td>${gradeBadge}</td>
                <td><span style="${cutoffStyle}; font-family: var(--font-mono);">${cutoffKhz}</span></td>
                <td><strong style="font-family: var(--font-mono);">${r.confidence}%</strong></td>
                <td style="font-size: 12px; color: var(--text-muted);">${escapeHtml(r.details)}</td>
                <td class="action-cell">
                    <div class="action-btn-group">
                        <button class="btn btn-secondary btn-sm" onclick="playAudio('${encodeURIComponent(r.file_path)}', '${escapeHtml(r.file_name)}')" title="试听音频">
                            ▶ 试听
                        </button>
                        <button class="btn btn-danger-outline btn-sm" onclick="cleanSingleFile('${encodeURIComponent(r.file_path)}')" title="移入回收站">
                            ♻️ 回收
                        </button>
                    </div>
                </td>
            </tr>
        `;
    }).join('');
}

window.cleanSingleFile = async function(encodedPath) {
    const filePath = decodeURIComponent(encodedPath);
    if (!confirm(`确定将此音频文件移入回收站目录吗？\n${filePath}`)) {
        return;
    }
    try {
        const res = await fetch(API.dedupClean, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ file_paths: [filePath], action: 'recycle' })
        });
        if (!res.ok) {
            const err = await res.json();
            showToast(err.detail || '移入回收站失败', 'error');
            return;
        }
        showToast('已成功移入回收站目录', 'success');
        loadLosslessRecords();
    } catch (err) {
        showToast('移入回收站失败: ' + err.message, 'error');
    }
};

// ==================== TAB 4: 歌单文本提取器 ====================

function initPlaylistExtractor() {
    const form = document.getElementById('form-playlist-parse');
    const urlInput = document.getElementById('playlist-url');
    const formatSelect = document.getElementById('playlist-format');
    const detailedCheck = document.getElementById('playlist-detailed');
    const reverseCheck = document.getElementById('playlist-reverse');
    const saveHistoryCheck = document.getElementById('playlist-save-history');
    const submitBtn = document.getElementById('btn-start-playlist');
    const progressContainer = document.getElementById('playlist-progress-container');

    // 表单提交解析
    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const url = urlInput.value.trim();
        if (!url) {
            showToast('请输入歌单分享链接或分享文案', 'warn');
            return;
        }

        submitBtn.disabled = true;
        progressContainer.classList.remove('hidden');

        try {
            const res = await fetch(API.playlistParse, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    url: url,
                    detailed: detailedCheck.checked,
                    format: formatSelect.value,
                    order: reverseCheck.checked ? 'reverse' : 'default',
                    save_history: saveHistoryCheck.checked
                })
            });

            const data = await res.json();
            if (!res.ok) {
                showToast(data.detail || '解析歌单失败，请检查链接是否正确', 'error');
                return;
            }

            state.playlistRawResult = data;
            state.playlistSongs = data.songs || [];

            // 更新统计卡片
            const platformNames = {
                'netease': '🔴 网易云音乐',
                'qq': '🟢 QQ音乐',
                'qishui': '🥤 汽水音乐'
            };
            document.getElementById('stat-pl-platform').textContent = platformNames[data.platform] || data.platform;
            document.getElementById('stat-pl-count').textContent = data.song_count;
            document.getElementById('stat-pl-time').textContent = `${data.parse_time.toFixed(2)}s`;

            // 更新标题
            document.getElementById('playlist-result-title').textContent = data.title || '歌单歌曲列表';
            document.getElementById('playlist-result-subtitle').textContent = `共解析出 ${data.song_count} 首歌曲 (${platformNames[data.platform] || data.platform})`;

            renderPlaylistSongs();
            showToast(`成功解析歌单「${data.title}」，共 ${data.song_count} 首歌曲！`, 'success');

            // 刷新历史
            loadPlaylistHistory();
        } catch (err) {
            showToast('请求解析歌单异常: ' + err.message, 'error');
        } finally {
            submitBtn.disabled = false;
            progressContainer.classList.add('hidden');
        }
    });

    // 搜索过滤
    document.getElementById('pl-filter-query').addEventListener('input', (e) => {
        renderPlaylistSongs(e.target.value);
    });

    // 复制纯文本
    document.getElementById('btn-pl-copy-all').addEventListener('click', async () => {
        if (!state.playlistSongs || state.playlistSongs.length === 0) {
            showToast('当前没有可复制的歌曲列表', 'warn');
            return;
        }
        const text = state.playlistSongs.map(s => s.full_text).join('\n');
        try {
            await navigator.clipboard.writeText(text);
            showToast(`已成功复制 ${state.playlistSongs.length} 首歌曲文本到剪贴板！可以直接粘贴到迁移工具`, 'success');
        } catch (err) {
            // 降级复制
            const ta = document.createElement('textarea');
            ta.value = text;
            document.body.appendChild(ta);
            ta.select();
            document.execCommand('copy');
            document.body.removeChild(ta);
            showToast(`已成功复制 ${state.playlistSongs.length} 首歌曲文本！`, 'success');
        }
    });

    // 导出 TXT
    document.getElementById('btn-pl-export-txt').addEventListener('click', () => {
        if (!state.playlistSongs || state.playlistSongs.length === 0) {
            showToast('当前没有歌曲可导出', 'warn');
            return;
        }
        const title = (state.playlistRawResult && state.playlistRawResult.title) || 'playlist';
        const songs = state.playlistSongs.map(s => s.full_text);
        exportPlaylistFile(title, 'txt', songs);
    });

    // 导出 CSV
    document.getElementById('btn-pl-export-csv').addEventListener('click', () => {
        if (!state.playlistSongs || state.playlistSongs.length === 0) {
            showToast('当前没有歌曲可导出', 'warn');
            return;
        }
        const title = (state.playlistRawResult && state.playlistRawResult.title) || 'playlist';
        const songs = state.playlistSongs.map(s => s.full_text);
        exportPlaylistFile(title, 'csv', songs);
    });

    // 刷新历史
    document.getElementById('btn-pl-refresh-history').addEventListener('click', loadPlaylistHistory);

    // 清空历史
    document.getElementById('btn-pl-clear-history').addEventListener('click', async () => {
        if (!confirm('确定要清空所有本地歌单提取历史记录吗？')) return;
        try {
            await fetch(API.playlistHistoryClear, { method: 'POST' });
            showToast('已清空本地历史记录', 'success');
            loadPlaylistHistory();
        } catch (e) {
            showToast('清空历史失败: ' + e.message, 'error');
        }
    });
}

function exportPlaylistFile(title, format, songs) {
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = API.playlistExport;
    form.target = '_blank';

    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = 'payload';

    fetch(API.playlistExport, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title, format, songs })
    }).then(res => res.blob()).then(blob => {
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `${title}.${format}`;
        document.body.appendChild(a);
        a.click();
        a.remove();
        window.URL.revokeObjectURL(url);
    }).catch(err => {
        showToast('导出失败: ' + err.message, 'error');
    });
}

function renderPlaylistSongs(query = '') {
    const tbody = document.getElementById('pl-songs-body');
    let songs = state.playlistSongs || [];

    if (query.trim()) {
        const q = query.toLowerCase();
        songs = songs.filter(s => s.song_name.toLowerCase().includes(q) || s.artist.toLowerCase().includes(q) || s.full_text.toLowerCase().includes(q));
    }

    if (songs.length === 0) {
        tbody.innerHTML = `
            <tr>
                <td colspan="5" class="empty-state">
                    <p>未找到匹配的歌曲</p>
                </td>
            </tr>
        `;
        return;
    }

    tbody.innerHTML = songs.map(s => {
        const encText = encodeURIComponent(s.full_text);
        return `
            <tr>
                <td style="font-family: var(--font-mono); color: var(--text-muted);">${s.index}</td>
                <td><strong style="color: var(--text-main);">${escapeHtml(s.song_name)}</strong></td>
                <td><span style="color: var(--accent-primary);">${escapeHtml(s.artist || '-')}</span></td>
                <td><span style="font-family: var(--font-mono); font-size: 12px; color: var(--text-sub);">${escapeHtml(s.full_text)}</span></td>
                <td class="action-cell">
                    <button class="btn btn-secondary btn-sm" onclick="copySingleSong('${encText}')" title="复制单曲名称与歌手">
                        📋 复制
                    </button>
                </td>
            </tr>
        `;
    }).join('');
}

window.copySingleSong = function(encodedText) {
    const text = decodeURIComponent(encodedText);
    navigator.clipboard.writeText(text).then(() => {
        showToast(`已复制: ${text}`, 'info');
    });
};

async function loadPlaylistHistory() {
    try {
        const res = await fetch(`${API.playlistHistory}?limit=30`);
        const data = await res.json();
        state.playlistHistory = data.history || [];

        document.getElementById('stat-pl-history-count').textContent = data.total || 0;
        renderPlaylistHistory();
    } catch (e) {
        console.error('拉取歌单历史失败:', e);
    }
}

function renderPlaylistHistory() {
    const tbody = document.getElementById('pl-history-body');
    const history = state.playlistHistory || [];

    if (history.length === 0) {
        tbody.innerHTML = `
            <tr>
                <td colspan="5" class="empty-state">
                    <p>暂无历史提取记录</p>
                </td>
            </tr>
        `;
        return;
    }

    const platformBadges = {
        'netease': '<span class="badge-fmt fmt-corrupt">网易云</span>',
        'qq': '<span class="badge-fmt fmt-flac">QQ音乐</span>',
        'qishui': '<span class="badge-fmt fmt-wav">汽水音乐</span>'
    };

    tbody.innerHTML = history.map(h => {
        const dateStr = new Date(h.created_at * 1000).toLocaleString('zh-CN', {
            month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'
        });

        return `
            <tr>
                <td>${platformBadges[h.platform] || h.platform}</td>
                <td>
                    <div class="song-main-info">
                        <strong style="color: var(--text-main); font-size: 13px;">${escapeHtml(h.title || '未命名歌单')}</strong>
                        <span class="song-path" style="max-width: 320px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">${escapeHtml(h.source_url)}</span>
                    </div>
                </td>
                <td><span style="font-family: var(--font-mono); font-weight: 600;">${h.song_count} 首</span></td>
                <td><span style="font-size: 12px; color: var(--text-muted);">${dateStr}</span></td>
                <td class="action-cell">
                    <div class="action-btn-group">
                        <button class="btn btn-primary btn-sm" onclick="loadHistoryDetail(${h.id})" title="展开查看此歌单歌曲">
                            📂 载入
                        </button>
                        <button class="btn btn-danger-outline btn-sm" onclick="deleteHistoryItem(${h.id})" title="删除此记录">
                            🗑️
                        </button>
                    </div>
                </td>
            </tr>
        `;
    }).join('');
}

window.loadHistoryDetail = async function(id) {
    try {
        const res = await fetch(`${API.playlistHistoryDetail}?id=${id}`);
        if (!res.ok) {
            showToast('载入历史歌单详情失败', 'error');
            return;
        }
        const data = await res.json();
        state.playlistRawResult = data;
        state.playlistSongs = data.songs || [];

        const platformNames = {
            'netease': '🔴 网易云音乐',
            'qq': '🟢 QQ音乐',
            'qishui': '🥤 汽水音乐'
        };
        document.getElementById('stat-pl-platform').textContent = platformNames[data.platform] || data.platform;
        document.getElementById('stat-pl-count').textContent = data.song_count;
        document.getElementById('stat-pl-time').textContent = '已缓存';

        document.getElementById('playlist-result-title').textContent = data.title || '歌单歌曲列表';
        document.getElementById('playlist-result-subtitle').textContent = `历史记录共 ${data.song_count} 首歌曲 (${platformNames[data.platform] || data.platform})`;

        renderPlaylistSongs();
        showToast(`已成功载入历史歌单「${data.title}」`, 'success');

        // 平滑滚动到歌曲卡片
        document.getElementById('playlist-result-title').scrollIntoView({ behavior: 'smooth' });
    } catch (e) {
        showToast('载入历史失败: ' + e.message, 'error');
    }
};

window.deleteHistoryItem = async function(id) {
    try {
        const res = await fetch(API.playlistHistoryDelete, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id })
        });
        if (res.ok) {
            showToast('已删除历史记录', 'info');
            loadPlaylistHistory();
        }
    } catch (e) {
        showToast('删除失败: ' + e.message, 'error');
    }
};

// ==================== TAB 5: 飞牛音乐歌单管理器 ====================

async function checkFeiNiuStatus() {
    try {
        const res = await fetch(API.feiniuStatus);
        if (!res.ok) return;
        const data = await res.json();
        state.feiniuStatus = data;

        const chip = document.getElementById('chip-feiniu');
        const chipLabel = document.getElementById('feiniu-chip-label');
        const authBadge = document.getElementById('fn-auth-status-badge');
        const statUser = document.getElementById('stat-fn-user');
        const statSession = document.getElementById('stat-fn-session-status');

        if (data.connected) {
            if (chip) {
                chip.classList.add('active');
            }
            if (chipLabel) chipLabel.textContent = `飞牛: ${data.username || '已连接'}`;
            if (authBadge) {
                authBadge.className = 'status-indicator-badge connected';
                authBadge.textContent = '🟢 已连接';
            }
            if (statUser) statUser.textContent = data.username || '已登录';
            if (statSession) {
                statSession.textContent = '已保活';
                statSession.style.color = '#10b981';
            }

            // 自动填充已保存的地址和用户名
            const serverInput = document.getElementById('fn-server-url');
            const userInput = document.getElementById('fn-username');
            if (serverInput) {
                if (data.server_url) {
                    serverInput.value = data.server_url;
                } else if (!serverInput.value) {
                    serverInput.value = 'http://172.17.0.1:5666';
                }
            }
            if (userInput && !userInput.value && data.username) userInput.value = data.username;
        } else {
            if (chip) {
                chip.classList.remove('active');
            }
            if (chipLabel) chipLabel.textContent = '飞牛NAS';
            if (authBadge) {
                authBadge.className = 'status-indicator-badge';
                authBadge.textContent = data.error ? '🔴 连接失败' : '未连接';
            }
            if (statUser) statUser.textContent = data.username ? `${data.username} (离线)` : '-';
            if (statSession) {
                statSession.textContent = '未连接';
                statSession.style.color = 'var(--text-muted)';
            }
            const serverInput = document.getElementById('fn-server-url');
            if (serverInput && !serverInput.value) {
                serverInput.value = data.server_url || 'http://172.17.0.1:5666';
            }
        }
    } catch (e) {
        console.error('检查飞牛状态失败:', e);
    }
}

function initFeiNiuManager() {
    const connectForm = document.getElementById('form-feiniu-connect');
    const disconnectBtn = document.getElementById('btn-fn-disconnect');
    const createPlForm = document.getElementById('form-fn-create-playlist');
    const refreshPlBtn = document.getElementById('btn-fn-refresh-playlists');
    const filterInput = document.getElementById('fn-filter-query');
    const closeTracksBtn = document.getElementById('btn-fn-close-tracks');
    const purgeInvalidBtn = document.getElementById('btn-fn-purge-invalid');

    // 连接表单提交
    if (connectForm) {
        connectForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const btn = document.getElementById('btn-fn-connect');
            btn.disabled = true;
            btn.textContent = '⏳ 正在连接...';

            const payload = {
                server_url: document.getElementById('fn-server-url').value.trim(),
                username: document.getElementById('fn-username').value.trim(),
                password: document.getElementById('fn-password').value,
                access_code: document.getElementById('fn-access-code').value.trim()
            };

            try {
                const res = await fetch(API.feiniuConnect, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                const result = await res.json();
                if (result.success) {
                    showToast(`飞牛 NAS 连接成功！欢迎 ${result.username}`, 'success');
                    await checkFeiNiuStatus();
                    await checkAuthStatus();
                    loadFeiNiuPlaylists();
                } else {
                    showToast(`连接失败: ${result.error || '未知错误'}`, 'error');
                    checkFeiNiuStatus();
                    checkAuthStatus();
                }
            } catch (err) {
                showToast(`请求异常: ${err.message}`, 'error');
            } finally {
                btn.disabled = false;
                btn.textContent = '⚡ 连接 / 重新登录';
            }
        });
    }

    // 断开连接
    if (disconnectBtn) {
        disconnectBtn.addEventListener('click', async () => {
            if (!confirm('确认断开飞牛 NAS 连接并清除本地存储的密码与 Token 凭据？')) return;
            try {
                await fetch(API.feiniuDisconnect, { method: 'POST' });
                showToast('已断开飞牛 NAS 连接', 'info');
                state.feiniuPlaylists = [];
                renderFeiNiuPlaylists([]);
                await checkFeiNiuStatus();
                await checkAuthStatus();
            } catch (err) {
                showToast('断开失败: ' + err.message, 'error');
            }
        });
    }

    // 快捷新建歌单
    if (createPlForm) {
        createPlForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const input = document.getElementById('fn-new-pl-name');
            const name = input.value.trim();
            if (!name) return;

            try {
                const res = await fetch(API.feiniuPlaylistCreate, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name })
                });
                const result = await res.json();
                if (result.success) {
                    showToast(`已成功创建飞牛歌单「${name}」`, 'success');
                    input.value = '';
                    loadFeiNiuPlaylists();
                } else {
                    showToast(`创建歌单失败: ${result.error}`, 'error');
                }
            } catch (err) {
                showToast(`创建异常: ${err.message}`, 'error');
            }
        });
    }

    // 刷新歌单列表
    if (refreshPlBtn) {
        refreshPlBtn.addEventListener('click', () => {
            loadFeiNiuPlaylists();
        });
    }

    // 筛选歌单
    if (filterInput) {
        filterInput.addEventListener('input', () => {
            const q = filterInput.value.trim().toLowerCase();
            if (!q) {
                renderFeiNiuPlaylists(state.feiniuPlaylists);
                return;
            }
            const filtered = state.feiniuPlaylists.filter(p => (p.name || '').toLowerCase().includes(q));
            renderFeiNiuPlaylists(filtered);
        });
    }

    // 关闭歌曲明细回到歌单列表
    if (closeTracksBtn) {
        closeTracksBtn.addEventListener('click', () => {
            document.getElementById('fn-track-detail-panel').classList.add('hidden');
            document.getElementById('fn-playlist-panel').classList.remove('hidden');
        });
    }

    // 清理失效歌曲
    if (purgeInvalidBtn) {
        purgeInvalidBtn.addEventListener('click', async () => {
            if (!state.currentFeiNiuPlaylist) return;
            if (!confirm(`确认清理歌单「${state.currentFeiNiuPlaylist.name}」中的失效歌曲？`)) return;

            try {
                const res = await fetch(API.feiniuPlaylistPurge, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ guid: state.currentFeiNiuPlaylist.guid })
                });
                const result = await res.json();
                if (result.success) {
                    showToast('清理失效歌曲完成', 'success');
                    window.viewFeiNiuPlaylist(state.currentFeiNiuPlaylist.guid, state.currentFeiNiuPlaylist.name);
                    loadFeiNiuPlaylists();
                } else {
                    showToast('清理失败: ' + result.error, 'error');
                }
            } catch (err) {
                showToast('清理请求失败: ' + err.message, 'error');
            }
        });
    }

    // 初始化重命名模态框事件
    initFeiNiuRenameModal();
}

async function loadFeiNiuPlaylists() {
    const grid = document.getElementById('fn-playlist-grid');
    if (!grid) return;

    try {
        const res = await fetch(`${API.feiniuPlaylists}?page=1&size=100`);
        const result = await res.json();
        if (!result.success) {
            grid.innerHTML = `
                <div class="empty-state" style="grid-column: 1 / -1;">
                    <p style="color: var(--color-danger);">拉取歌单失败: ${escapeHtml(result.error || '未连接飞牛 NAS')}</p>
                </div>
            `;
            return;
        }

        const data = result.data || {};
        state.feiniuPlaylists = data.list || [];

        // 更新统计卡片
        const countEl = document.getElementById('stat-fn-playlist-count');
        const trackCountEl = document.getElementById('stat-fn-track-count');
        if (countEl) countEl.textContent = state.feiniuPlaylists.length;
        if (trackCountEl) {
            const totalTracks = state.feiniuPlaylists.reduce((sum, p) => sum + (p.trackCount || 0), 0);
            trackCountEl.textContent = totalTracks;
        }

        renderFeiNiuPlaylists(state.feiniuPlaylists);

        // 异步精准校准每个歌单的实际歌曲总数（防止飞牛初始返回 trackCount=0）
        Promise.all(state.feiniuPlaylists.map(async (p) => {
            try {
                const tr = await fetch(`${API.feiniuPlaylistTracks}?guid=${encodeURIComponent(p.guid)}&page=1&size=1`);
                const td = await tr.json();
                if (td.success && td.data && typeof td.data.total === 'number') {
                    p.trackCount = td.data.total;
                    const badge = document.getElementById(`fn-badge-${p.guid}`);
                    if (badge) badge.textContent = `🎵 ${p.trackCount} 首`;
                }
            } catch (e) {}
        })).then(() => {
            if (trackCountEl) {
                const totalTracks = state.feiniuPlaylists.reduce((sum, p) => sum + (p.trackCount || 0), 0);
                trackCountEl.textContent = totalTracks;
            }
        });
    } catch (e) {
        grid.innerHTML = `
            <div class="empty-state" style="grid-column: 1 / -1;">
                <p>无法连接飞牛服务: ${escapeHtml(e.message)}</p>
            </div>
        `;
    }
}

function renderFeiNiuPlaylists(playlists) {
    const grid = document.getElementById('fn-playlist-grid');
    if (!grid) return;

    if (!playlists || playlists.length === 0) {
        grid.innerHTML = `
            <div class="empty-state" style="grid-column: 1 / -1;">
                <p>暂无歌单，您可以在左侧创建新歌单或从外部歌单提取后一键导入</p>
            </div>
        `;
        return;
    }

    grid.innerHTML = playlists.map(p => {
        const safeName = escapeHtml(p.name || '未命名歌单');
        const trackCount = p.trackCount || 0;
        const timeStr = p.updatedAt ? new Date(p.updatedAt * 1000).toLocaleDateString() : '';

        // 封面图构建
        let coverHtml = `<div class="playlist-cover-placeholder">🎵</div>`;
        if (p.coverId) {
            const coverUrl = `${API.feiniuCover}?coverId=${encodeURIComponent(p.coverId)}&size=300`;
            coverHtml = `<img src="${coverUrl}" class="playlist-cover-img" alt="${safeName}" onerror="this.parentElement.innerHTML='<div class=\\'playlist-cover-placeholder\\'>🎵</div>'">`;
        }

        return `
            <div class="playlist-card" onclick="viewFeiNiuPlaylist('${escapeHtml(p.guid)}', '${safeName}')">
                <div class="playlist-cover-box">
                    ${coverHtml}
                    <div class="playlist-track-badge" id="fn-badge-${escapeHtml(p.guid)}">🎵 ${trackCount} 首</div>
                </div>
                <div class="playlist-card-content">
                    <div class="playlist-card-title" title="${safeName}">${safeName}</div>
                    <div class="playlist-card-time">${timeStr ? '更新于 ' + timeStr : ''}</div>
                    <div class="playlist-card-actions" onclick="event.stopPropagation();">
                        <button class="btn-card-action" onclick="viewFeiNiuPlaylist('${escapeHtml(p.guid)}', '${safeName}')" title="查看歌曲">📂</button>
                        <button class="btn-card-action" onclick="editFeiNiuPlaylist('${escapeHtml(p.guid)}', '${safeName}')" title="重命名">✏️</button>
                        <button class="btn-card-action" style="color: var(--color-danger);" onclick="deleteFeiNiuPlaylist('${escapeHtml(p.guid)}', '${safeName}')" title="删除歌单">🗑️</button>
                    </div>
                </div>
            </div>
        `;
    }).join('');
}

window.viewFeiNiuPlaylist = async function(guid, name) {
    state.currentFeiNiuPlaylist = { guid, name };

    const playlistPanel = document.getElementById('fn-playlist-panel');
    const trackPanel = document.getElementById('fn-track-detail-panel');
    const nameEl = document.getElementById('fn-detail-playlist-name');
    const subEl = document.getElementById('fn-detail-playlist-sub');
    const tbody = document.getElementById('fn-tracks-body');

    playlistPanel.classList.add('hidden');
    trackPanel.classList.remove('hidden');

    nameEl.textContent = name || '歌单曲目明细';
    subEl.textContent = '正在加载歌曲...';
    tbody.innerHTML = `<tr><td colspan="7" class="empty-state"><p>正在加载歌曲列表...</p></td></tr>`;

    try {
        const res = await fetch(`${API.feiniuPlaylistTracks}?guid=${encodeURIComponent(guid)}&page=1&size=300`);
        const result = await res.json();
        if (!result.success) {
            tbody.innerHTML = `<tr><td colspan="7" class="empty-state"><p style="color: var(--color-danger);">${escapeHtml(result.error)}</p></td></tr>`;
            return;
        }

        const tracks = (result.data && result.data.list) || [];
        const totalCount = (result.data && typeof result.data.total === 'number') ? result.data.total : tracks.length;
        subEl.textContent = `共 ${totalCount} 首歌曲`;

        // 动态同步更新卡片角标与缓存
        const targetPl = (state.feiniuPlaylists || []).find(x => x.guid === guid);
        if (targetPl) targetPl.trackCount = totalCount;
        const badgeEl = document.getElementById(`fn-badge-${guid}`);
        if (badgeEl) badgeEl.textContent = `🎵 ${totalCount} 首`;
        const trackCountEl = document.getElementById('stat-fn-track-count');
        if (trackCountEl) {
            const totalTracks = (state.feiniuPlaylists || []).reduce((sum, p) => sum + (p.trackCount || 0), 0);
            trackCountEl.textContent = totalTracks;
        }

        if (tracks.length === 0) {
            tbody.innerHTML = `<tr><td colspan="7" class="empty-state"><p>此歌单内暂无歌曲</p></td></tr>`;
            return;
        }

        tbody.innerHTML = tracks.map((t, idx) => {
            const safeTitle = escapeHtml(t.title || '未知歌曲');
            const artists = (t.artists || []).map(a => (typeof a === 'object' ? a.name : a)).filter(Boolean).join(' / ') || '未知歌手';
            let albumName = '-';
            if (t.album) {
                if (typeof t.album === 'object') {
                    albumName = t.album.name || t.album.title || '-';
                } else {
                    albumName = String(t.album);
                }
            }
            const album = escapeHtml(albumName);
            const format = (t.audioSpec && t.audioSpec.format ? t.audioSpec.format.toUpperCase() : 'AUDIO');
            const bitrate = (t.audioSpec && t.audioSpec.bitrate) ? `${Math.round(t.audioSpec.bitrate / 1000)}k` : '';
            const duration = formatDuration(t.duration || (t.audioSpec && t.audioSpec.duration));

            return `
                <tr>
                    <td><span style="font-family: var(--font-mono);">${idx + 1}</span></td>
                    <td>
                        <div class="song-meta">
                            <strong style="color: var(--text-main); font-size: 13px;">${safeTitle}</strong>
                            <span class="song-path" style="max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">${escapeHtml(t.audioSpec ? t.audioSpec.path : '')}</span>
                        </div>
                    </td>
                    <td><span>${escapeHtml(artists)}</span></td>
                    <td><span style="color: var(--text-muted);">${album}</span></td>
                    <td><span class="badge-format">${format} ${bitrate}</span></td>
                    <td><span style="font-family: var(--font-mono);">${duration}</span></td>
                    <td class="action-cell">
                        <div class="action-btn-group">
                            <button class="btn btn-danger-outline btn-sm" onclick="removeFeiNiuTrack('${escapeHtml(guid)}', '${escapeHtml(t.guid)}')" title="从歌单中移除此曲目">
                                移除
                            </button>
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
    } catch (e) {
        tbody.innerHTML = `<tr><td colspan="7" class="empty-state"><p>加载失败: ${escapeHtml(e.message)}</p></td></tr>`;
    }
};

window.editFeiNiuPlaylist = function(guid, name) {
    const modal = document.getElementById('fn-rename-modal');
    document.getElementById('fn-rename-guid').value = guid;
    document.getElementById('fn-rename-name').value = name;
    modal.classList.remove('hidden');
};

function initFeiNiuRenameModal() {
    const modal = document.getElementById('fn-rename-modal');
    const closeBtn = document.getElementById('btn-fn-rename-close');
    const cancelBtn = document.getElementById('btn-fn-rename-cancel');
    const confirmBtn = document.getElementById('btn-fn-rename-confirm');

    const closeModal = () => modal.classList.add('hidden');
    if (closeBtn) closeBtn.addEventListener('click', closeModal);
    if (cancelBtn) cancelBtn.addEventListener('click', closeModal);

    if (confirmBtn) {
        confirmBtn.addEventListener('click', async () => {
            const guid = document.getElementById('fn-rename-guid').value;
            const name = document.getElementById('fn-rename-name').value.trim();
            if (!name) return;

            try {
                const res = await fetch(API.feiniuPlaylistEdit, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ guid, name })
                });
                const result = await res.json();
                if (result.success) {
                    showToast('歌单已成功重命名', 'success');
                    closeModal();
                    loadFeiNiuPlaylists();
                } else {
                    showToast('修改失败: ' + result.error, 'error');
                }
            } catch (err) {
                showToast('请求失败: ' + err.message, 'error');
            }
        });
    }
}

window.deleteFeiNiuPlaylist = async function(guid, name) {
    if (!confirm(`确定要从飞牛 NAS 中删除歌单「${name}」吗？（不会删除音乐源文件）`)) return;

    try {
        const res = await fetch(API.feiniuPlaylistDelete, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ guid })
        });
        const result = await res.json();
        if (result.success) {
            showToast(`已删除歌单「${name}」`, 'info');
            loadFeiNiuPlaylists();
        } else {
            showToast('删除失败: ' + result.error, 'error');
        }
    } catch (e) {
        showToast('删除请求失败: ' + e.message, 'error');
    }
};

window.removeFeiNiuTrack = async function(playlistGuid, trackGuid) {
    try {
        const res = await fetch(API.feiniuPlaylistRemoveTracks, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                guid: playlistGuid,
                trackGUIDs: [trackGuid]
            })
        });
        const result = await res.json();
        if (result.success) {
            showToast('已从歌单移除歌曲', 'info');
            window.viewFeiNiuPlaylist(playlistGuid, state.currentFeiNiuPlaylist ? state.currentFeiNiuPlaylist.name : '');
            loadFeiNiuPlaylists();
        } else {
            showToast('移除失败: ' + result.error, 'error');
        }
    } catch (e) {
        showToast('请求失败: ' + e.message, 'error');
    }
};

// ==================== 飞牛歌单一键导入模态框 ====================

function initFeiNiuImportModal() {
    const importBtn = document.getElementById('btn-pl-import-feiniu');
    const modal = document.getElementById('fn-import-modal');
    const closeBtn = document.getElementById('btn-fn-import-modal-close');
    const cancelBtn = document.getElementById('btn-fn-import-cancel');
    const confirmBtn = document.getElementById('btn-fn-import-confirm');
    const copyUnmatchedBtn = document.getElementById('btn-copy-unmatched');

    const formArea = document.getElementById('fn-import-form-area');
    const runningArea = document.getElementById('fn-import-running-area');
    const resultArea = document.getElementById('fn-import-result-area');

    const groupNewName = document.getElementById('fn-import-group-new-name');
    const groupExistingPl = document.getElementById('fn-import-group-existing-pl');
    const selectPl = document.getElementById('fn-import-select-pl');

    const closeModal = () => modal.classList.add('hidden');
    if (closeBtn) closeBtn.addEventListener('click', closeModal);
    if (cancelBtn) cancelBtn.addEventListener('click', closeModal);

    // 单选框切换新建/现有歌单模式
    document.querySelectorAll('input[name="fn-import-mode"]').forEach(radio => {
        radio.addEventListener('change', (e) => {
            if (e.target.value === 'new') {
                groupNewName.classList.remove('hidden');
                groupExistingPl.classList.add('hidden');
            } else {
                groupNewName.classList.add('hidden');
                groupExistingPl.classList.remove('hidden');
            }
        });
    });

    if (importBtn) {
        importBtn.addEventListener('click', async () => {
            if (!state.playlistSongs || state.playlistSongs.length === 0) {
                showToast('请先在上方提取歌单歌曲列表或载入历史记录', 'warning');
                return;
            }

            // 打开模态框并重置状态
            modal.classList.remove('hidden');
            formArea.classList.remove('hidden');
            runningArea.classList.add('hidden');
            resultArea.classList.add('hidden');
            confirmBtn.disabled = false;
            confirmBtn.textContent = '🚀 开始匹配并导入';

            document.getElementById('fn-import-modal-desc').textContent = 
                `即将匹配并导入「${state.playlistRawResult ? state.playlistRawResult.title : '提取歌单'}」共 ${state.playlistSongs.length} 首歌曲至飞牛曲库`;
            
            document.getElementById('fn-import-new-name').value = 
                (state.playlistRawResult && state.playlistRawResult.title) ? state.playlistRawResult.title : '外部导入歌单';

            // 载入已有飞牛歌单到下拉列表
            selectPl.innerHTML = '<option value="">-- 请选择现有歌单 --</option>';
            if (state.feiniuPlaylists && state.feiniuPlaylists.length > 0) {
                state.feiniuPlaylists.forEach(p => {
                    selectPl.innerHTML += `<option value="${escapeHtml(p.guid)}">${escapeHtml(p.name)} (${p.trackCount} 首)</option>`;
                });
            } else {
                try {
                    const res = await fetch(`${API.feiniuPlaylists}?page=1&size=100`);
                    const data = await res.json();
                    if (data.success && data.data && data.data.list) {
                        state.feiniuPlaylists = data.data.list;
                        state.feiniuPlaylists.forEach(p => {
                            selectPl.innerHTML += `<option value="${escapeHtml(p.guid)}">${escapeHtml(p.name)} (${p.trackCount} 首)</option>`;
                        });
                    }
                } catch (e) {}
            }
        });
    }

    if (confirmBtn) {
        confirmBtn.addEventListener('click', async () => {
            const mode = document.querySelector('input[name="fn-import-mode"]:checked').value;
            let targetName = '';
            let targetGuid = '';

            if (mode === 'new') {
                targetName = document.getElementById('fn-import-new-name').value.trim();
                if (!targetName) {
                    showToast('请输入歌单名称', 'warning');
                    return;
                }
            } else {
                targetGuid = selectPl.value;
                if (!targetGuid) {
                    showToast('请选择目标飞牛歌单', 'warning');
                    return;
                }
            }

            // 切换到运行中状态
            formArea.classList.add('hidden');
            runningArea.classList.remove('hidden');
            confirmBtn.disabled = true;
            confirmBtn.textContent = '⏳ 正在检索匹配并写入...';

            const payload = {
                name: targetName,
                playlist_guid: targetGuid,
                songs: state.playlistSongs
            };

            try {
                const res = await fetch(API.feiniuPlaylistImport, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                
                const rawText = await res.text();
                let result;
                try {
                    result = JSON.parse(rawText);
                } catch (parseErr) {
                    throw new Error(`服务端返回了非 JSON 内容 (HTTP ${res.status}): ${rawText.slice(0, 80).replace(/<[^>]*>/g, '').trim() || '页面错误或网关超时'}`);
                }

                if (!res.ok) {
                    throw new Error(result.error || result.detail || result.message || `请求失败 (HTTP ${res.status})`);
                }

                runningArea.classList.add('hidden');
                resultArea.classList.remove('hidden');

                if (result.success && result.data) {
                    const rep = result.data;
                    document.getElementById('fn-rep-total').textContent = rep.total;
                    document.getElementById('fn-rep-matched').textContent = rep.matched_count;
                    document.getElementById('fn-rep-unmatched').textContent = rep.unmatched_count;

                    showToast(`歌单「${rep.playlist_name}」导入完成！已成功匹配入库 ${rep.matched_count} 首`, 'success');

                    // 未匹配歌曲清单
                    const unmatchedBox = document.getElementById('fn-unmatched-box');
                    const unmatchedTextarea = document.getElementById('fn-unmatched-textarea');
                    if (rep.unmatched_songs && rep.unmatched_songs.length > 0) {
                        unmatchedBox.classList.remove('hidden');
                        const unmatchedLines = rep.unmatched_songs.map((s, i) => `${i + 1}. ${s.song_name} - ${s.artist}`).join('\n');
                        unmatchedTextarea.value = unmatchedLines;
                    } else {
                        unmatchedBox.classList.add('hidden');
                    }

                    confirmBtn.textContent = '✅ 导入完成';
                    loadFeiNiuPlaylists();
                } else {
                    showToast('导入失败: ' + (result.error || '未知错误'), 'error');
                    confirmBtn.disabled = false;
                    confirmBtn.textContent = '重新尝试';
                }
            } catch (err) {
                runningArea.classList.add('hidden');
                showToast('导入请求异常: ' + err.message, 'error');
                confirmBtn.disabled = false;
                confirmBtn.textContent = '重新尝试';
            }
        });
    }

    if (copyUnmatchedBtn) {
        copyUnmatchedBtn.addEventListener('click', () => {
            const textarea = document.getElementById('fn-unmatched-textarea');
            if (!textarea || !textarea.value) return;
            navigator.clipboard.writeText(textarea.value).then(() => {
                showToast('已复制未命中歌曲清单到剪贴板', 'success');
            });
        });
    }
}

// ==================== 侧边栏折叠与展开功能 ====================

function initSidebarCollapse() {
    let isCollapsed = localStorage.getItem('music_toolkit_sidebar_collapsed') === 'true';

    const updateUI = (collapsed) => {
        document.querySelectorAll('.grid-layout').forEach(layout => {
            if (collapsed) {
                layout.classList.add('sidebar-collapsed');
            } else {
                layout.classList.remove('sidebar-collapsed');
            }
        });
        localStorage.setItem('music_toolkit_sidebar_collapsed', collapsed ? 'true' : 'false');
    };

    if (isCollapsed) {
        updateUI(true);
    }

    // 绑定右上角折叠按钮 (上三角+三横线)
    document.querySelectorAll('.btn-sidebar-toggle').forEach(btn => {
        btn.addEventListener('click', (e) => {
            e.stopPropagation();
            isCollapsed = true;
            updateUI(true);
        });
    });

    // 绑定折叠后左侧浮动的展开面板按钮
    document.querySelectorAll('.sidebar-expand-trigger').forEach(btn => {
        btn.addEventListener('click', (e) => {
            e.stopPropagation();
            isCollapsed = false;
            updateUI(false);
        });
    });
}

// ==================== 系统双重认证与权限管理 ====================

function showAuthPortal() {
    const portal = document.getElementById('system-auth-portal');
    if (portal) portal.style.display = 'flex';
    document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
    document.querySelectorAll('.nav-tab').forEach(t => t.classList.remove('active'));
}

function hideAuthPortal() {
    const portal = document.getElementById('system-auth-portal');
    if (portal) portal.style.display = 'none';
}

async function checkAuthStatus() {
    try {
        const res = await originalFetch(API.authStatus, {
            headers: {
                'Authorization': `Bearer ${localStorage.getItem('music_toolkit_token') || ''}`
            }
        });
        if (!res.ok) return null;
        const data = await res.json();
        state.authStatus = data;

        // 本地用户 Chip 状态
        const localChip = document.getElementById('chip-local-user');
        const localLabel = document.getElementById('local-user-label');
        const logoutBtn = document.getElementById('btn-logout-local');

        if (data.local_authenticated) {
            if (localChip) localChip.classList.add('active');
            if (localLabel) localLabel.textContent = `本地: ${data.local_user}`;
            if (logoutBtn) logoutBtn.style.display = 'inline-flex';
        } else {
            if (localChip) localChip.classList.remove('active');
            if (localLabel) localLabel.textContent = '本地: 未登录';
            if (logoutBtn) logoutBtn.style.display = 'none';
        }

        // 飞牛连接 Chip 状态
        const fnChip = document.getElementById('chip-feiniu');
        const fnLabel = document.getElementById('feiniu-chip-label');
        if (data.feiniu_connected) {
            if (fnChip) fnChip.classList.add('active');
            if (fnLabel) fnLabel.textContent = `飞牛: ${data.feiniu_user || '已连接'}`;
        } else {
            if (fnChip) fnChip.classList.remove('active');
            if (fnLabel) fnLabel.textContent = '飞牛NAS';
        }

        const lockIcons = document.querySelectorAll('.tab-lock-icon');
        const toolTabs = document.querySelectorAll('.nav-tab:not([data-tab="tab-feiniu"])');

        if (data.unlocked) {
            // 系统全量解锁
            toolTabs.forEach(t => t.classList.remove('locked'));
            lockIcons.forEach(icon => icon.style.display = 'none');
            hideAuthPortal();

            // 如果当前无激活 Tab，默认激活格式检查
            let activeTab = document.querySelector('.nav-tab.active');
            if (!activeTab) {
                const defaultTab = document.querySelector('.nav-tab[data-tab="tab-format"]');
                if (defaultTab) {
                    defaultTab.classList.add('active');
                    const targetEl = document.getElementById('tab-format');
                    if (targetEl) targetEl.classList.add('active');
                }
            }
        } else {
            // 系统未解锁，进入认证守卫模式
            toolTabs.forEach(t => t.classList.add('locked'));
            lockIcons.forEach(icon => icon.style.display = 'inline');
            showAuthPortal();

            // 根据是否已初始化切换显示创建账号或密码登录
            const initBox = document.getElementById('auth-box-init');
            const loginBox = document.getElementById('auth-box-login');
            if (data.initialized) {
                if (initBox) initBox.style.display = 'none';
                if (loginBox) loginBox.style.display = 'block';
            } else {
                if (initBox) initBox.style.display = 'block';
                if (loginBox) loginBox.style.display = 'none';
            }
        }

        return data;
    } catch (e) {
        console.error('获取系统认证状态失败:', e);
        return null;
    }
}

function initAuthManager() {
    // 门户模式选项卡切换 (本地账号 vs 飞牛直连)
    const switchBtns = document.querySelectorAll('.auth-tab-btn');
    switchBtns.forEach(btn => {
        btn.addEventListener('click', () => {
            switchBtns.forEach(b => b.classList.remove('active'));
            document.querySelectorAll('.auth-panel-content').forEach(p => p.classList.remove('active'));

            btn.classList.add('active');
            const targetId = btn.getAttribute('data-target');
            const targetEl = document.getElementById(targetId);
            if (targetEl) targetEl.classList.add('active');
        });
    });

    // 首次使用创建管理员表单
    const initForm = document.getElementById('form-auth-init');
    if (initForm) {
        initForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const username = document.getElementById('auth-init-username').value.trim();
            const password = document.getElementById('auth-init-password').value;
            const confirm = document.getElementById('auth-init-password-confirm').value;
            const submitBtn = document.getElementById('btn-auth-init-submit');

            if (!username) {
                showToast('请输入用户名', 'warning');
                return;
            }
            if (password.length < 4) {
                showToast('密码长度至少需要 4 位', 'warning');
                return;
            }
            if (password !== confirm) {
                showToast('两次输入的密码不一致，请核对', 'error');
                return;
            }

            submitBtn.disabled = true;
            submitBtn.textContent = '⏳ 正在初始化...';

            try {
                const res = await originalFetch(API.authInit, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username, password })
                });
                const result = await res.json();
                if (result.success && result.token) {
                    localStorage.setItem('music_toolkit_token', result.token);
                    showToast(`🎉 管理员账号创建成功！欢迎 ${result.username}，系统已解锁。`, 'success');
                    await checkAuthStatus();
                    loadFormatRecords();
                } else {
                    showToast(result.error || '创建管理员失败', 'error');
                }
            } catch (err) {
                showToast('初始化请求异常: ' + err.message, 'error');
            } finally {
                submitBtn.disabled = false;
                submitBtn.textContent = '🚀 创建管理员并解锁系统';
            }
        });
    }

    // 本地账号登录表单
    const loginForm = document.getElementById('form-auth-login');
    if (loginForm) {
        loginForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const username = document.getElementById('auth-login-username').value.trim();
            const password = document.getElementById('auth-login-password').value;
            const submitBtn = document.getElementById('btn-auth-login-submit');

            submitBtn.disabled = true;
            submitBtn.textContent = '⏳ 正在登录...';

            try {
                const res = await originalFetch(API.authLogin, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username, password })
                });
                const result = await res.json();
                if (result.success && result.token) {
                    localStorage.setItem('music_toolkit_token', result.token);
                    showToast(`🔓 登录成功！欢迎回来 ${result.username}`, 'success');
                    await checkAuthStatus();
                    loadFormatRecords();
                } else {
                    showToast(result.error || '登录失败，请检查账号密码', 'error');
                }
            } catch (err) {
                showToast('登录请求异常: ' + err.message, 'error');
            } finally {
                submitBtn.disabled = false;
                submitBtn.textContent = '🔓 登录并解锁系统';
            }
        });
    }

    // 本地账号退出登录
    const logoutBtn = document.getElementById('btn-logout-local');
    if (logoutBtn) {
        logoutBtn.addEventListener('click', async () => {
            if (!confirm('确认退出当前本地管理员登录？退出后若未连接飞牛音乐将重新锁定系统。')) return;
            try {
                await originalFetch(API.authLogout, {
                    method: 'POST',
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('music_toolkit_token') || ''}`
                    }
                });
            } catch (e) {
                // ignore
            }
            localStorage.removeItem('music_toolkit_token');
            showToast('已退出本地账号', 'info');
            await checkAuthStatus();
        });
    }

    // 认证门户飞牛直连表单
    const portalFnForm = document.getElementById('form-portal-feiniu-connect');
    if (portalFnForm) {
        portalFnForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const btn = document.getElementById('btn-portal-fn-connect');
            btn.disabled = true;
            btn.textContent = '⏳ 正在连接飞牛 NAS...';

            const payload = {
                server_url: document.getElementById('portal-fn-server-url').value.trim(),
                username: document.getElementById('portal-fn-username').value.trim(),
                password: document.getElementById('portal-fn-password').value,
                access_code: document.getElementById('portal-fn-access-code').value.trim()
            };

            try {
                const res = await originalFetch(API.feiniuConnect, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                const result = await res.json();
                if (result.success) {
                    showToast(`⚡ 飞牛 NAS 连接成功！系统已解锁。欢迎 ${result.username}`, 'success');
                    await checkFeiNiuStatus();
                    await checkAuthStatus();
                    loadFeiNiuPlaylists();
                    loadFormatRecords();
                } else {
                    showToast(`连接失败: ${result.error || '未知错误'}`, 'error');
                }
            } catch (err) {
                showToast(`连接请求异常: ${err.message}`, 'error');
            } finally {
                btn.disabled = false;
                btn.textContent = '⚡ 连接飞牛并解锁系统';
            }
        });
    }
}

// 页面加载启动
document.addEventListener('DOMContentLoaded', async () => {
    initThemeToggle();
    initSidebarCollapse();
    initActionModalEvents();
    initDirStatsWatchers();
    fetchSystemStatus();
    initNavTabs();
    initFormatChecker();
    initDeduplicator();
    initLosslessChecker();
    initPlaylistExtractor();
    initFeiNiuManager();
    initFeiNiuImportModal();
    initAuthManager();

    const auth = await checkAuthStatus();
    await checkFeiNiuStatus();

    if (auth && auth.unlocked) {
        loadFormatRecords();
    }
});


