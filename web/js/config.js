// 忆安智护 前端配置文件
// 根据部署环境自动选择 API 地址

const CONFIG = {
    // 离线演示模式：无需后端，所有数据保存在 localStorage
    OFFLINE_MODE: false,

    // API 基础地址配置
    API_BASE_URL: 'http://localhost:8088',

    // 默认工作空间
    DEFAULT_WORKSPACE: 'default',

    // 登录态仅保存在当前浏览器会话，避免长期持久化
    AUTH_STORAGE: 'session',

    // 文件上传配置
    FILE_UPLOAD: {
        MAX_SIZE: 10 * 1024 * 1024, // 10MB
        ALLOWED_TYPES: ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.pdf', '.doc', '.docx', '.txt'],
        ALLOWED_MIME_TYPES: [
            'image/jpeg', 'image/png', 'image/gif', 'image/bmp',
            'application/pdf',
            'application/msword',
            'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
            'text/plain'
        ]
    },

    // 会话配置
    SESSION: {
        MAX_HISTORY_LENGTH: 10, // 发送给后端的最大历史消息数
        AUTO_SAVE: true, // 自动保存会话
        STORAGE_PREFIX: 'chatSessions_' // localStorage 前缀
    },

    // UI 配置
    UI: {
        MESSAGE_ANIMATION: true,
        TYPING_INDICATOR: true,
        AUTO_SCROLL: true
    },

    // 调试模式
    DEBUG: false,

    // 日志函数
    log: function(...args) {
        if (this.DEBUG) {
            console.log('[忆安智护]', ...args);
        }
    },

    error: function(...args) {
        console.error('[忆安智护 Error]', ...args);
    },

    warn: function(...args) {
        console.warn('[忆安智护 Warning]', ...args);
    }
};

// 打印配置信息
CONFIG.log('配置加载完成:', {
    API_BASE_URL: CONFIG.API_BASE_URL,
    protocol: window.location.protocol,
    hostname: window.location.hostname
});

// 导出配置（兼容不同的模块系统）
if (typeof module !== 'undefined' && module.exports) {
    module.exports = CONFIG;
}
if (typeof window !== 'undefined') {
    window.CONFIG = CONFIG;
}
