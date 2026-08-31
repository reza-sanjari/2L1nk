
const Settings = (() => {
    const KEY = '2l1nk_settings';
    const DEFAULTS = {
        accentColor: '#8b5cf6', accentRgb: '139, 92, 246', accentDark: '#3b1a6e',
        bgFrom: '#0d0d12', bgTo: '#1a1a2e', shapeColor: '#3b1a6e',
        bgStyle: 'shapes', glow: true, bubbleSquare: false,
        fontSize: 'normal', timestamps: false, compact: false,
        notifSound: true, notifDesktop: false,
    };
    const PRESETS = [
        { name: 'Purple', color: '#8b5cf6', rgb: '139, 92, 246', dark: '#3b1a6e' },
        { name: 'Blue', color: '#1d9bf0', rgb: '29, 155, 240', dark: '#0a3d6b' },
        { name: 'Green', color: '#00c853', rgb: '0, 200, 83', dark: '#005723' },
        { name: 'Red', color: '#f44336', rgb: '244, 67, 54', dark: '#7f0000' },
        { name: 'Orange', color: '#ff6d00', rgb: '255, 109, 0', dark: '#7f3400' },
        { name: 'Cyan', color: '#00bcd4', rgb: '0, 188, 212', dark: '#006064' },
        { name: 'Turquoise', color: '#2dd4bf', rgb: '45, 212, 191', dark: '#134e4a' },
    ];

    function load() {
        try {
            const s = localStorage.getItem(KEY);
            return s ? { ...DEFAULTS, ...JSON.parse(s) } : { ...DEFAULTS };
        } catch { return { ...DEFAULTS }; }
    }

    function save(s) { localStorage.setItem(KEY, JSON.stringify(s)); }

    function apply(s) {
        const r = document.documentElement;
        r.style.setProperty('--accent', s.accentColor);
        r.style.setProperty('--accent-rgb', s.accentRgb);
        r.style.setProperty('--accent-dark', s.accentDark);
        r.style.setProperty('--bg-from', s.bgFrom);
        r.style.setProperty('--bg-to', s.bgTo);
        r.style.setProperty('--shape-color', s.shapeColor);
        const b = document.body;
        b.classList.remove('bg-shapes', 'bg-grid', 'bg-gradient', 'bg-none');
        b.classList.add(`bg-${s.bgStyle}`);
        b.classList.toggle('no-glow', !s.glow);
        b.classList.toggle('bubble-square', s.bubbleSquare);
        b.classList.remove('font-sm', 'font-lg');
        if (s.fontSize !== 'normal') b.classList.add(`font-${s.fontSize}`);
        b.classList.toggle('show-timestamps', s.timestamps);
        b.classList.toggle('compact', s.compact);
    }

    return { load, save, apply, PRESETS, DEFAULTS };
})();

function resetSettings() {
    Settings.save({ ...Settings.DEFAULTS });
    Settings.apply(Settings.DEFAULTS);
    syncSettingsUI(Settings.DEFAULTS);
}

function setSetting(key, value) {
    const s = Settings.load();
    if (key === 'notifDesktop' && value) {
        Notification.requestPermission().then(perm => {
            const tog = document.getElementById('tog-desktop');
            if (perm !== 'granted') { if (tog) tog.checked = false; return; }
            s[key] = true;
            Settings.save(s);
            Settings.apply(s);
            syncSettingsUI(s);
        });
        return;
    }
    s[key] = value;
    Settings.save(s);
    Settings.apply(s);
    syncSettingsUI(s);
}

function syncSettingsUI(s) {
    document.querySelectorAll('.settings-swatch').forEach(el => {
        el.classList.toggle('active', el.dataset.color === s.accentColor);
    });
    [['seg-bg', 'bgStyle', v => v], ['seg-bubble', 'bubbleSquare', v => v ? 'square' : 'round'], ['seg-fontsize', 'fontSize', v => v]].forEach(([id, key, mapFn]) => {
        const seg = document.getElementById(id);
        if (!seg) return;
        const cur = mapFn(s[key]);
        seg.querySelectorAll('button').forEach(btn => btn.classList.toggle('active', btn.dataset.val === String(cur)));
    });
    [['bg-from-picker', 'bgFrom'], ['bg-to-picker', 'bgTo'], ['shape-color-picker', 'shapeColor']].forEach(([id, key]) => {
        const el = document.getElementById(id);
        if (el) el.value = s[key];
    });
    [['tog-glow', 'glow'], ['tog-timestamps', 'timestamps'], ['tog-compact', 'compact'], ['tog-sound', 'notifSound'], ['tog-desktop', 'notifDesktop']].forEach(([id, key]) => {
        const el = document.getElementById(id);
        if (el) el.checked = !!s[key];
    });
}

function playNotifSound() {
    try {
        const ctx = new (window.AudioContext || window.webkitAudioContext)();
        const osc = ctx.createOscillator();
        const gain = ctx.createGain();
        osc.connect(gain);
        gain.connect(ctx.destination);
        osc.frequency.setValueAtTime(880, ctx.currentTime);
        osc.frequency.exponentialRampToValueAtTime(660, ctx.currentTime + 0.15);
        gain.gain.setValueAtTime(0.25, ctx.currentTime);
        gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.35);
        osc.start(ctx.currentTime);
        osc.stop(ctx.currentTime + 0.35);
    } catch { }
}

function showDesktopNotif(roomName, text) {
    if (Notification.permission === 'granted') {
        try { new Notification(`2L1nk — ${roomName}`, { body: text || 'New message' }); } catch { }
    }
}

function copySettingsValue(sessionKey, btnId) {
    const val = sessionStorage.getItem(sessionKey) ?? '';
    if (!val) return;
    navigator.clipboard.writeText(val).then(() => {
        const btn = document.getElementById(btnId);
        if (!btn) return;
        btn.classList.add('copied');
        const orig = btn.innerHTML;
        btn.innerHTML = '<i class="fas fa-check"></i> Copied';
        setTimeout(() => { btn.innerHTML = orig; btn.classList.remove('copied'); }, 1800);
    }).catch(() => { });
}

function formatTime(unixSeconds) {
    const d = new Date(unixSeconds * 1000);
    const h = d.getHours().toString().padStart(2, '0');
    const m = d.getMinutes().toString().padStart(2, '0');
    return `${h}:${m}`;
}
