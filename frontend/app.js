// ==================== Music Toolkit 前端交互逻辑 ====================

const API = {
    systemStatus: '/api/system/status',
    formatScan: '/api/format/scan',
    formatProgress: '/api/format/progress',
    formatRecords: '/api/format/records',
    formatFixSingle: '/api/format/fix-single',
    formatFixBatch: '/api/format/fix-batch',
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
    audioStream: '/api/audio/stream'
};

// 状态对象
const state = {
    system: null,
    formatRecords: [],
    losslessRecords: [],
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
        musicLabel.textContent = data.music_dir || '/music';
        if (data.music_dir_exists) musicChip.classList.add('active');

        // 设置默认输入框
        if (!document.getElementById('fmt-music-dir').value) {
            document.getElementById('fmt-music-dir').value = data.music_dir || '';
        }
        if (!document.getElementById('fmt-output-dir').value) {
            document.getElementById('fmt-output-dir').value = data.output_dir || '';
        }
        if (!document.getElementById('dedup-music-dir').value) {
            document.getElementById('dedup-music-dir').value = data.music_dir || '';
        }
        if (!document.getElementById('lossless-music-dir').value) {
            document.getElementById('lossless-music-dir').value = data.music_dir || '';
        }

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

// ==================== 选项卡切换 ====================

function initNavTabs() {
    const tabs = document.querySelectorAll('.nav-tab');
    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            tabs.forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));

            tab.classList.add('active');
            const targetId = tab.getAttribute('data-tab');
            const targetEl = document.getElementById(targetId);
            if (targetEl) targetEl.classList.add('active');

            if (targetId === 'tab-dedup') {
                loadDuplicateGroups();
            } else if (targetId === 'tab-format') {
                loadFormatRecords();
            } else if (targetId === 'tab-lossless') {
                loadLosslessRecords();
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
            output_dir: document.getElementById('fmt-output-dir').value.trim() || undefined,
            action: document.getElementById('fmt-action').value,
            keep_structure: document.getElementById('fmt-keep-structure').checked,
            fix_extension: document.getElementById('fmt-fix-ext').checked,
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

    // 批量纠正
    document.getElementById('btn-batch-fix-rename').addEventListener('click', async () => {
        const selected = Array.from(document.querySelectorAll('.fmt-item-check:checked')).map(cb => cb.value);
        if (selected.length === 0) {
            showToast('请先勾选需要修正后缀的文件', 'warn');
            return;
        }

        if (!confirm(`确定要直接重命名修正选中的 ${selected.length} 个文件后缀吗？`)) return;

        try {
            const res = await fetch(API.formatFixBatch, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ file_paths: selected, action: 'rename_fix' })
            });
            const data = await res.json();
            showToast(`批量修正完成：成功处理 ${data.total} 个文件`, 'success');
            loadFormatRecords();
        } catch (e) {
            showToast('批量修正失败: ' + e.message, 'error');
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

        return `
            <tr>
                <td><input type="checkbox" class="fmt-item-check" value="${r.file_path}"></td>
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
                <td><span style="font-size: 11px; color: var(--text-sub);">${r.status || '-'}</span></td>
                <td>
                    <button class="btn btn-secondary btn-sm" onclick="fixSingleFormat('${encodeURIComponent(r.file_path)}', '${r.suggested_ext}')" title="原地修正扩展名">
                        ✏️ 修正后缀
                    </button>
                </td>
            </tr>
        `;
    }).join('');
}

window.fixSingleFormat = async function(encodedPath, suggestedExt) {
    const filePath = decodeURIComponent(encodedPath);
    try {
        const res = await fetch(API.formatFixSingle, {
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

    // 一键智能清理
    document.getElementById('btn-clean-recycle').addEventListener('click', () => cleanRecommended('recycle'));
    document.getElementById('btn-clean-delete').addEventListener('click', () => {
        if (confirm('【高危提醒】这将永久彻底删除所有重复组中被判定为冗余的音频文件，无法撤销！建议使用移入回收站。确定彻底删除吗？')) {
            cleanRecommended('delete');
        }
    });
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
                                    <td width="140" style="text-align: right;">
                                        <button class="btn btn-secondary btn-sm" onclick="playAudio('${encodeURIComponent(s.file_path)}', '${escapeHtml(s.file_name)}')" title="试听音频">
                                            ▶ 试听
                                        </button>
                                        <button class="btn btn-danger-outline btn-sm" onclick="cleanSingleFile('${encodeURIComponent(s.file_path)}')" title="移入回收站">
                                            ♻️
                                        </button>
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

function escapeHtml(str) {
    if (!str) return '';
    return str.replace(/&/g, '&amp;')
              .replace(/</g, '&lt;')
              .replace(/>/g, '&gt;')
              .replace(/"/g, '&quot;')
              .replace(/'/g, '&#039;');
}

function initThemeToggle() {
    const btn = document.getElementById('btn-toggle-theme');
    if (!btn) return;

    const savedTheme = localStorage.getItem('music_toolkit_theme') || 'light';
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
            gradeBadge = '<span class="tag-lossless" style="background: linear-gradient(135deg, #ec4899, #8b5cf6);">🏆 真 Hi-Res</span>';
        } else if (r.grade === 'true_lossless') {
            gradeBadge = '<span class="tag-lossless">💎 真无损 (CD)</span>';
        } else if (r.grade === 'fake_320k') {
            gradeBadge = '<span class="badge-fmt" style="background: rgba(245, 158, 11, 0.15); color: #d97706; border: 1px solid rgba(245, 158, 11, 0.35);">⚠️ 假无损 (320k)</span>';
            cutoffStyle = 'color: #d97706; font-weight: 700;';
        } else if (r.grade === 'fake_low_bitrate') {
            gradeBadge = '<span class="badge-fmt" style="background: rgba(239, 68, 68, 0.15); color: #dc2626; border: 1px solid rgba(239, 68, 68, 0.35);">🚫 劣质假无损 (128k)</span>';
            cutoffStyle = 'color: #dc2626; font-weight: 700;';
        } else {
            gradeBadge = '<span class="badge-fmt">未知</span>';
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

// 页面加载启动
document.addEventListener('DOMContentLoaded', () => {
    initThemeToggle();
    fetchSystemStatus();
    initNavTabs();
    initFormatChecker();
    initDeduplicator();
    initLosslessChecker();
    loadFormatRecords();
});
