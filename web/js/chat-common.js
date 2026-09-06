// 通用聊天功能 - 所有角色共用
// 全局变量
let isFirstMessage = true;
let currentSessionID = '';
let messageHistory = [];
const API_BASE_URL = window.CONFIG ? window.CONFIG.API_BASE_URL : 'http://localhost:8088';
let USER_ID = '';
let USER_NAME = '';
let JWT_TOKEN = '';
let sessions = {};
let currentSessionTitle = '新对话';
let selectedFiles = [];
let CURRENT_ROLE_CONFIG = null;
let agentModeEnabled = false;
const AUTH_STORAGE = window.CONFIG && window.CONFIG.AUTH_STORAGE === 'local'
    ? window.localStorage
    : window.sessionStorage;

function getStoredAuth() {
    return {
        token: AUTH_STORAGE.getItem('jwt_token'),
        userId: AUTH_STORAGE.getItem('user_id'),
        userName: AUTH_STORAGE.getItem('user_name')
    };
}

function persistAuth(token, userId, userName) {
    AUTH_STORAGE.setItem('jwt_token', token);
    AUTH_STORAGE.setItem('user_id', userId);
    AUTH_STORAGE.setItem('user_name', userName);
}

function clearAuth() {
    AUTH_STORAGE.removeItem('jwt_token');
    AUTH_STORAGE.removeItem('user_id');
    AUTH_STORAGE.removeItem('user_name');
}

function isOfflineToken(token) {
    return typeof token === 'string' && token.startsWith('offline_');
}

function getDemoPassword() {
    const input = document.getElementById('demoPasswordInput');
    return input ? input.value.trim() : '';
}

function showAuthError(message) {
    const errorNode = document.getElementById('loginStatusMessage');
    if (errorNode) {
        errorNode.textContent = message;
        errorNode.style.display = 'block';
        return;
    }

    alert(message);
}

function clearAuthError() {
    const errorNode = document.getElementById('loginStatusMessage');
    if (errorNode) {
        errorNode.textContent = '';
        errorNode.style.display = 'none';
    }
}

function renderSecurityNotice() {
    const overlay = document.getElementById('loginOverlay');
    if (!overlay) return;

    const loginHeader = overlay.querySelector('.login-header');
    const testUsersSection = overlay.querySelector('.test-users-section');
    if (!loginHeader || !testUsersSection) return;

    if (!document.getElementById('loginSecurityNotice')) {
        const notice = document.createElement('div');
        notice.id = 'loginSecurityNotice';
        notice.style.cssText = 'margin-top:12px;padding:10px 12px;border-radius:10px;background:rgba(15,23,42,0.06);color:#334155;font-size:12px;line-height:1.6;';
        notice.textContent = window.CONFIG && window.CONFIG.OFFLINE_MODE
            ? '当前为显式离线演示模式，仅用于本地展示。'
            : '当前为在线安全模式，需输入演示密码，且不会再自动降级为离线模式。';
        loginHeader.appendChild(notice);
    }

    if (!window.CONFIG || !window.CONFIG.OFFLINE_MODE) {
        if (!document.getElementById('demoPasswordInput')) {
            const wrapper = document.createElement('div');
            wrapper.style.cssText = 'margin:12px 0 14px;';
            wrapper.innerHTML = `
                <input
                    id="demoPasswordInput"
                    type="password"
                    placeholder="请输入演示密码"
                    autocomplete="current-password"
                    style="width:100%;padding:12px 14px;border:1px solid rgba(148,163,184,0.45);border-radius:12px;font-size:14px;outline:none;"
                />
                <div id="loginStatusMessage" style="display:none;margin-top:8px;color:#b91c1c;font-size:12px;line-height:1.5;"></div>
            `;
            testUsersSection.parentNode.insertBefore(wrapper, testUsersSection);
        }
    } else if (!document.getElementById('loginStatusMessage')) {
        const message = document.createElement('div');
        message.id = 'loginStatusMessage';
        message.style.cssText = 'display:none;margin:12px 0 0;color:#b91c1c;font-size:12px;line-height:1.5;';
        testUsersSection.parentNode.insertBefore(message, testUsersSection);
    }

    const hint = overlay.querySelector('.login-hint');
    if (hint) {
        hint.textContent = window.CONFIG && window.CONFIG.OFFLINE_MODE
            ? '离线演示模式已显式开启，不连接后端认证服务'
            : '请输入演示密码后，再选择一个测试身份登录';
    }
}

function loginWithExplicitOfflineMode(userName, userId) {
    JWT_TOKEN = 'offline_demo_token_' + Date.now();
    USER_ID = userId;
    USER_NAME = userName;
    persistAuth(JWT_TOKEN, USER_ID, USER_NAME);

    const userAvatar = document.getElementById('userAvatar');
    if (userAvatar) {
        userAvatar.textContent = userName;
    }

    const loginOverlay = document.getElementById('loginOverlay');
    if (loginOverlay) {
        loginOverlay.style.display = 'none';
    }

    initializeAfterLogin();
}

// 初始化聊天
function initializeChat(roleConfig) {
    CURRENT_ROLE_CONFIG = roleConfig;
    console.log('初始化聊天，角色:', roleConfig.role);

    // 检查是否已有登录信息
    renderSecurityNotice();
    const savedToken = AUTH_STORAGE.getItem('jwt_token');
    const savedUserId = AUTH_STORAGE.getItem('user_id');
    const savedUserName = AUTH_STORAGE.getItem('user_name');

    if (savedToken && savedUserId && savedUserName) {
        // 已登录，恢复登录状态
        JWT_TOKEN = savedToken;
        USER_ID = savedUserId;
        USER_NAME = savedUserName;

        console.log(`恢复登录状态: ${USER_NAME} (${USER_ID})`);

        // 更新用户头像显示
        const userAvatar = document.getElementById('userAvatar');
        if (userAvatar) {
            userAvatar.textContent = USER_NAME;
        }

        // 隐藏登录界面
        document.getElementById('loginOverlay').style.display = 'none';

        // 初始化会话
        initializeAfterLogin();
    } else {
        // 未登录，显示登录界面
        console.log('未登录，显示登录界面');
        document.getElementById('loginOverlay').style.display = 'flex';
    }
}

// 快速登录
async function quickLogin(userName, userId) {
    try {
        console.log(`[认证] 快速登录开始: ${userName} (${userId})`);
        console.log(`[认证] API地址: ${API_BASE_URL}`);

        // 显示加载状态
        const loginOverlay = document.getElementById('loginOverlay');

        // 安全获取角色图标
        let roleIcon = '👤';
        if (CURRENT_ROLE_CONFIG && CURRENT_ROLE_CONFIG.role) {
            roleIcon = CURRENT_ROLE_CONFIG.role === 'caregiver' ? '🧑‍⚕️' :
                CURRENT_ROLE_CONFIG.role === 'doctor' ? '👨‍⚕️' :
                CURRENT_ROLE_CONFIG.role === 'family' ? '👨‍👩‍👧' : '👴';
        }

        loginOverlay.innerHTML = `
            <div class="login-container" style="text-align: center;">
                <div class="login-logo">${roleIcon}</div>
                <div class="login-title">正在登录...</div>
                <div class="login-subtitle">请稍候</div>
            </div>
        `;

        // 离线演示模式：直接跳过认证
        if (window.CONFIG && window.CONFIG.OFFLINE_MODE) {
            console.log('[认证] 离线演示模式，跳过认证');
            loginWithExplicitOfflineMode(userName, userId);
            return;
        }

        // 在线模式：调用后端 API
        console.log(`[认证] 发送登录请求到: ${API_BASE_URL}/api/auth/login`);
        const demoPassword = getDemoPassword();
        if (!demoPassword) {
            renderSecurityNotice();
            showAuthError('请输入演示密码后再登录');
            const passwordInput = document.getElementById('demoPasswordInput');
            if (passwordInput) passwordInput.focus();
            return;
        }

        const response = await fetch(`${API_BASE_URL}/api/auth/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                user_id: userId,
                password: demoPassword,
                workspace_id: window.CONFIG ? window.CONFIG.DEFAULT_WORKSPACE : 'default'
            })
        });

        console.log(`[认证] 登录响应状态: ${response.status}`);

        if (response.ok) {
            const data = await response.json();
            if (data.success && data.data && data.data.token) {
                JWT_TOKEN = data.data.token;
                USER_ID = userId;
                USER_NAME = userName;
                persistAuth(JWT_TOKEN, USER_ID, USER_NAME);
                const userAvatar = document.getElementById('userAvatar');
                if (userAvatar) userAvatar.textContent = userName;
                loginOverlay.style.display = 'none';
                initializeAfterLogin();
            } else {
                throw new Error('登录响应格式错误');
            }
        } else {
            const errorText = await response.text();
            throw new Error(`HTTP ${response.status}`);
        }
    } catch (error) {
        console.error('[认证] 快速登录失败:', error);

        // 如果是网络错误，降级到离线模式
        if (error.message === 'Failed to fetch' || error.message.includes('fetch')) {
            console.warn('[认证] 后端不可用，自动切换到离线演示模式');
            JWT_TOKEN = 'offline_fallback_token_' + Date.now();
            USER_ID = userId;
            USER_NAME = userName;
            persistAuth(JWT_TOKEN, USER_ID, USER_NAME);
            const userAvatar = document.getElementById('userAvatar');
            if (userAvatar) userAvatar.textContent = userName;
            const loginOverlay = document.getElementById('loginOverlay');
            loginOverlay.style.display = 'none';
            initializeAfterLogin();

            // 显示提示信息
            setTimeout(() => {
                alert('⚠️ 后端服务不可用，已切换到离线演示模式\n\n部分功能可能受限，可视化服务仍可正常使用。');
            }, 500);
        } else {
            // 其他错误才弹出提示
            alert(`登录失败：${error.message}\n\n将继续使用离线模式`);
            // 降级到离线模式
            JWT_TOKEN = 'offline_fallback_token_' + Date.now();
            USER_ID = userId;
            USER_NAME = userName;
            persistAuth(JWT_TOKEN, USER_ID, USER_NAME);
            const loginOverlay = document.getElementById('loginOverlay');
            loginOverlay.style.display = 'none';
            initializeAfterLogin();
        }
    }
}

// 登录后初始化
function initializeAfterLogin() {
    messageHistory = [];
    currentSessionID = 'session_' + USER_ID + '_' + Date.now();
    sessions = {};
    isFirstMessage = true;
    currentSessionTitle = '新对话';
    selectedFiles = [];

    console.log('初始化会话ID:', currentSessionID);

    // 加载该用户的历史会话
    loadUserSessions();
}

// 退出登录
function logout() {
    if (confirm('确定要切换用户吗？当前会话将被保存。')) {
        // 保存当前会话
        saveCurrentSession();

        // 清空全局变量
        messageHistory = [];
        currentSessionID = '';
        sessions = {};
        isFirstMessage = true;
        selectedFiles = [];

        // 清除登录信息
        clearAuth();

        // 重新加载页面
        location.reload();
    }
}

// 自动调整输入框高度
function autoResize(textarea) {
    textarea.style.height = 'auto';
    textarea.style.height = Math.min(textarea.scrollHeight, 150) + 'px';
}

// 处理回车键
function handleKeyDown(event) {
    if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault();
        sendMessage();
    }
}

// 发送消息
function isExplicitNursingRecordIntent(message) {
    return /^(记录|护理记录|新增记录|保存记录)[：:\s]/.test(String(message || '').trim()) ||
        /^(记录|护理记录|新增记录|保存记录)/.test(String(message || '').trim());
}

async function sendMessage() {
    const input = document.getElementById('messageInput');
    const message = input.value.trim();

    if (!message && selectedFiles.length === 0) return;

    // 隐藏欢迎界面
    if (isFirstMessage) {
        const welcomeScreen = document.getElementById('welcomeScreen');
        if (welcomeScreen) {
            welcomeScreen.style.display = 'none';
        }
        isFirstMessage = false;
    }

    // 先上传文件（如果有）
    let fileAttachments = [];
    if (selectedFiles.length > 0) {
        try {
            fileAttachments = await uploadFiles(selectedFiles);
        } catch (error) {
            console.error('文件上传失败:', error);
            alert('文件上传失败: ' + error.message);
            return;
        }
    }

    // 构建消息内容
    let fullMessage = message;
    if (fileAttachments.length > 0) {
        const fileInfo = fileAttachments.map(f => `[文件: ${f.file_name}]`).join(' ');
        fullMessage = message + (message ? '\n' : '') + fileInfo;
    }

    // 先添加用户消息到界面（临时显示，后续会用脱敏版本替换）
    // Mask identifiers before rendering or persisting browser-side history.
    // Explicit record commands bypass conversational generation and write to
    // the JWT-protected nursing record endpoint.
    if (fileAttachments.length === 0 && isExplicitNursingRecordIntent(fullMessage)) {
        const categorySelect = document.getElementById('nursingRecordCategory');
        const category = categorySelect ? categorySelect.value : '日常护理';
        input.value = '';
        input.style.height = 'auto';
        saveNursingRecord(fullMessage, category);
        return;
    }

    const displayMessage = maskSensitiveForDisplay(fullMessage);
    const userMessageElement = addMessage('user', displayMessage, null, fileAttachments);

    // 添加到历史记录（临时，后续会更新为脱敏版本）
    messageHistory.push({
        role: 'user',
        content: displayMessage,
        timestamp: Date.now(),
        files: fileAttachments
    });

    // 如果是第一条消息，根据内容生成会话标题
    if (messageHistory.length === 1) {
        currentSessionTitle = displayMessage.length > 15 ? displayMessage.substring(0, 15) + '...' : displayMessage;
    }

    // 清空输入框和文件选择
    input.value = '';
    input.style.height = 'auto';
    selectedFiles = [];
    updateFilePreview();

    // 禁用发送按钮
    const sendBtn = document.getElementById('sendBtn');
    sendBtn.disabled = true;

    // 显示加载动画
    showTypingIndicator();

    // 【移除】不再在发送前拦截可视化请求，改为在接收到LLM回复后处理

    // 检查是否是普通问题（不涉及数据查询）
    const isSimpleQuestion = !fullMessage.includes('最近') && !fullMessage.includes('数据') && !fullMessage.includes('趋势') && !fullMessage.includes('查询');

    // 如果后端不可用且是简单问题，直接用模拟回复
    const backendAvailable = JWT_TOKEN && !isOfflineToken(JWT_TOKEN);

    // 离线演示模式或后端不可用时：模拟 AI 回复
    if (window.CONFIG && window.CONFIG.OFFLINE_MODE) {
        setTimeout(() => {
            hideTypingIndicator();
            const demoReply = generateOfflineReply(fullMessage);
            addMessage('assistant', demoReply);
            messageHistory.push({
                role: 'assistant',
                content: demoReply,
                timestamp: Date.now()
            });
            saveCurrentSession();
            sendBtn.disabled = false;
        }, 800 + Math.random() * 600);
        return;
    }

    if (!backendAvailable) {
        hideTypingIndicator();
        addMessage('assistant', '认证状态无效，请重新登录后再继续对话。');
        sendBtn.disabled = false;
        return;
    }

    try {
        const savedToken = AUTH_STORAGE.getItem('jwt_token');
        if (!savedToken) {
            throw new Error('认证失败，请刷新页面重试');
        }

        JWT_TOKEN = savedToken;

        // 调用后端API
        const response = await fetch(`${API_BASE_URL}/api/chat`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${JWT_TOKEN}`
            },
            body: JSON.stringify({
                user_id: USER_ID,
                session_id: currentSessionID,
                message: fullMessage,
                history: messageHistory.slice(-10),
                files: fileAttachments,
                agent_mode: agentModeEnabled
            })
        });

        console.log(`[聊天] 响应状态: ${response.status}`);

        const data = await response.json();

        if (!response.ok) {
            if (response.status === 401) {
                clearAuth();
                JWT_TOKEN = '';
                USER_ID = '';
                USER_NAME = '';
                alert('登录已过期，请重新登录');
                location.reload();
                return;
            }
            // 优先使用后端返回的错误信息
            const errorMsg = data.error || data.message || `服务器错误 (${response.status})`;
            throw new Error(errorMsg);
        }

        // 隐藏加载动画
        hideTypingIndicator();

        if (data.success) {
            // 🔍 调试：打印后端返回的数据
            console.log('📥 后端返回数据:', {
                user_message: data.data.user_message,
                fullMessage: fullMessage,
                isDifferent: data.data.user_message !== fullMessage
            });

            // 更新会话ID
            if (data.data.session_id) {
                currentSessionID = data.data.session_id;
            }

            // 🔒 如果后端返回了脱敏后的用户消息，更新界面显示
            if (data.data.user_message && data.data.user_message !== fullMessage) {
                console.log('🔄 准备更新用户消息为脱敏版本');
                // 更新用户消息显示为脱敏版本
                updateLastUserMessage(data.data.user_message);

                // 更新历史记录中的用户消息为脱敏版本
                if (messageHistory.length > 0 && messageHistory[messageHistory.length - 1].role === 'user') {
                    messageHistory[messageHistory.length - 1].content = data.data.user_message;
                }
            } else {
                console.log('⚠️ 未更新用户消息，原因:',
                    !data.data.user_message ? '后端未返回user_message' :
                    data.data.user_message === fullMessage ? 'user_message与原消息相同' : '未知');
            }

            // 🔒 显示敏感信息检测提示
            if (data.data.sensitive_infos && data.data.sensitive_infos.length > 0) {
                showSensitiveInfoWarning(data.data.sensitive_infos);
            }

            // 🔒 显示安全警告
            if (data.data.security_warnings && data.data.security_warnings.length > 0) {
                showSecurityWarnings(data.data.security_warnings);
            }

            // 添加AI回复到界面
            addMessage('assistant', data.data.message);

            if (data.data.memory_write_applied) {
                addMemoryWriteStatus();
            }

            // 仅显示后端返回的安全执行摘要，不渲染推理或工具原始数据。
            if (data.data.agent_execution) {
                addAgentExecutionSummary(data.data.agent_execution);
            }

            // ✨ 检测回复中是否包含血压数据，如果有则生成可视化图表
            await handleVisualizationIfNeeded(fullMessage, data.data.message);

            // 添加到历史记录
            messageHistory.push({
                role: 'assistant',
                content: data.data.message,
                timestamp: data.data.timestamp
            });

            // 保存会话
            saveCurrentSession();
        } else {
            throw new Error(data.error || '未知错误');
        }
    } catch (error) {
        console.error('发送消息失败:', error);
        hideTypingIndicator();

        const errorMessage = `抱歉，${error.message}`;
        addMessage('assistant', errorMessage);
    } finally {
        sendBtn.disabled = false;
    }
}

// 快捷问题
function sendQuickQuestion(question) {
    document.getElementById('messageInput').value = question;
    sendMessage();
}

async function saveNursingRecord(content, category) {
    const recordContent = String(content || '').trim();
    if (!recordContent) return;

    const savedToken = AUTH_STORAGE.getItem('jwt_token');
    if (!savedToken || isOfflineToken(savedToken)) {
        addMessage('assistant', '认证状态无效，请重新登录后再保存护理记录。');
        return;
    }

    const displayRecordContent = maskSensitiveForDisplay(recordContent);
    addMessage('user', displayRecordContent);
    messageHistory.push({ role: 'user', content: displayRecordContent, timestamp: Date.now() });
    if (messageHistory.length === 1) {
        currentSessionTitle = displayRecordContent.length > 15 ? displayRecordContent.substring(0, 15) + '...' : displayRecordContent;
    }
    showTypingIndicator();

    try {
        const response = await fetch(`${API_BASE_URL}/api/v1/nursing/records`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${savedToken}`
            },
            body: JSON.stringify({
                session_id: currentSessionID,
                content: recordContent,
                category: category || '日常护理'
            })
        });
        const data = await response.json();
        if (!response.ok || !data.success) {
            throw new Error(data.error || `保存失败 (${response.status})`);
        }

        const record = data.data;
        const savedAt = record.created_at || new Date().toLocaleString();
        const acknowledgement = `护理记录已保存\n记录 ID：${record.record_id}\n类别：${record.category}\n时间：${savedAt}\n范围：当前受保护会话`;
        addMessage('assistant', acknowledgement);
        addMemoryWriteStatus();
        addNursingRecordSecuritySummary(record.security_execution);
        messageHistory.push({ role: 'assistant', content: acknowledgement, timestamp: record.timestamp || Date.now() });
        if (record.sensitive_types && record.sensitive_types.length > 0) {
            showSecurityWarnings(['护理记录中的敏感字段已按策略脱敏']);
        }
        saveCurrentSession();
    } catch (error) {
        console.error('保存护理记录失败:', error);
        addMessage('assistant', `护理记录未保存：${error.message}`);
    } finally {
        hideTypingIndicator();
    }
}

function addNursingRecordSecuritySummary(execution) {
    if (!execution) return;
    const container = document.getElementById('messagesContainer');
    if (!container) return;
    const summary = document.createElement('div');
    summary.className = 'memory-write-status';
    const sensitive = Array.isArray(execution.sensitive_types) && execution.sensitive_types.length
        ? `；敏感字段：${execution.sensitive_types.join('、')}（已脱敏）` : '';
    summary.textContent = `受控记录结果：ASDF已检查；输入归一化${execution.input_normalized ? '已执行' : '无需执行'}；范围：${execution.storage_scope || 'protected_session'}${sensitive}`;
    summary.style.cssText = [
        'margin: 6px 0 10px 58px', 'padding-left: 10px',
        'border-left: 3px solid #176b87', 'color: #176b87', 'font-size: 12px'
    ].join(';');
    container.appendChild(summary);
    container.scrollTop = container.scrollHeight;
}

function saveNursingRecordFromInput() {
    const input = document.getElementById('messageInput');
    if (!input) return;
    const content = input.value.trim();
    if (!content) {
        input.focus();
        return;
    }
    const categorySelect = document.getElementById('nursingRecordCategory');
    const category = categorySelect ? categorySelect.value : '日常护理';
    input.value = '';
    input.style.height = 'auto';
    saveNursingRecord(content, category);
}

// 添加消息到界面
function addMessage(type, content, timestamp, files) {
    const container = document.getElementById('messagesContainer');
    const messageDiv = document.createElement('div');
    messageDiv.className = `message ${type}`;

    let timeStr;
    if (timestamp) {
        const date = new Date(timestamp);
        timeStr = `${date.getHours().toString().padStart(2, '0')}:${date.getMinutes().toString().padStart(2, '0')}`;
    } else {
        const now = new Date();
        timeStr = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`;
    }

    // 构建文件附件HTML
    let filesHTML = '';
    if (files && files.length > 0) {
        filesHTML = files.map(file => {
            const icon = getFileIcon(file.file_name);
            const size = formatFileSize(file.file_size);
            return `
                <div class="file-message-attachment">
                    <span>${icon}</span>
                    <span>${file.file_name}</span>
                    <span style="opacity: 0.7;">(${size})</span>
                </div>
            `;
        }).join('');
    }

    const avatarIcon = type === 'user' ? '👤' :
        (CURRENT_ROLE_CONFIG.role === 'caregiver' ? '🧑‍⚕️' :
         CURRENT_ROLE_CONFIG.role === 'doctor' ? '👨‍⚕️' :
         CURRENT_ROLE_CONFIG.role === 'family' ? '👨‍👩‍👧' : '👴');

    messageDiv.innerHTML = `
        <div class="message-avatar">${avatarIcon}</div>
        <div class="message-content">
            <div class="message-bubble">
                ${formatMessage(content)}
                ${filesHTML}
            </div>
            <div class="message-time">${timeStr}</div>
        </div>
    `;

    container.appendChild(messageDiv);
    container.scrollTop = container.scrollHeight;

    return messageDiv; // 返回消息元素，以便后续更新
}

function addMemoryWriteStatus() {
    const container = document.getElementById('messagesContainer');
    if (!container) return;

    const status = document.createElement('div');
    status.className = 'memory-write-status';
    status.textContent = '已写入当前受保护会话';
    status.style.cssText = [
        'margin: 6px 0 10px 58px',
        'padding-left: 10px',
        'border-left: 3px solid #2e7d32',
        'color: #2e7d32',
        'font-size: 12px'
    ].join(';');
    container.appendChild(status);
    container.scrollTop = container.scrollHeight;
}

// 更新最后一条用户消息的内容（用于显示脱敏后的版本）
function updateLastUserMessage(redactedContent) {
    const container = document.getElementById('messagesContainer');
    const messages = container.querySelectorAll('.message.user');

    if (messages.length > 0) {
        const lastUserMessage = messages[messages.length - 1];
        const contentBubble = lastUserMessage.querySelector('.message-bubble');

        if (contentBubble) {
            // 保留文件附件HTML，只更新文本内容
            const fileAttachments = contentBubble.querySelectorAll('.file-message-attachment');
            let filesHTML = '';
            fileAttachments.forEach(attachment => {
                filesHTML += attachment.outerHTML;
            });

            contentBubble.innerHTML = formatMessage(redactedContent) + filesHTML;
            console.log('✅ 用户消息已更新为脱敏版本');
        }
    }
}

// 格式化消息内容
function maskSensitiveForDisplay(content) {
    return String(content || '').replace(/(?:\d[\s\-]*){11,18}/g, (candidate) => {
        const digits = candidate.replace(/\D/g, '');
        const isPhone = digits.length === 11 && /^1[3-9]\d{9}$/.test(digits);
        const isIDCard = digits.length === 18 && /^(?:1[1-5]|2[1-3]|3[1-7]|4[1-6]|5[0-4]|6[1-5])\d{16}$/.test(digits);
        if (!isPhone && !isIDCard) return candidate;
        return isPhone ? `${digits.slice(0, 3)}****${digits.slice(-4)}` : `${digits.slice(0, 6)}********${digits.slice(-4)}`;
    });
}

function formatMessage(content) {
    return escapeHtml(String(content || '')).replace(/\n/g, '<br>');
}

// 显示受控 Agent 的安全执行摘要。原始推理、参数和观察结果不会进入浏览器。
function addAgentExecutionSummary(execution) {
    const container = document.getElementById('messagesContainer');
    const traceDiv = document.createElement('div');
    traceDiv.className = 'agent-trace';

    let stepsHTML = '';
    if (execution.steps && execution.steps.length > 0) {
        execution.steps.forEach((step) => {
            stepsHTML += `<div class="agent-step">`;
            stepsHTML += `<div class="agent-step-header">步骤 ${step.step_number}</div>`;
            stepsHTML += `<div class="agent-action"><span class="agent-label">已登记工具:</span> ${escapeHtml(step.tool_name || '未调用工具')}</div>`;
            stepsHTML += `<div class="agent-execution-status"><span class="agent-label">状态:</span> ${escapeHtml(step.status || 'completed')} · ${Number(step.duration_ms || 0)}ms</div>`;
            stepsHTML += `</div>`;
        });
    }

    const fallbackTag = execution.fallback ? ' <span class="agent-fallback-tag">(安全降级)</span>' : '';
    const statsHTML = `<div class="agent-stats">
        ${execution.tool_calls || 0}次工具调用 | ${execution.total_time_ms || 0}ms${fallbackTag}
    </div>`;

    traceDiv.innerHTML = `
        <div class="agent-trace-toggle" onclick="this.parentElement.classList.toggle('expanded')">
            受控执行记录 (点击展开)
        </div>
        <div class="agent-trace-content">
            ${stepsHTML}
            ${statsHTML}
        </div>
    `;

    container.appendChild(traceDiv);
    container.scrollTop = container.scrollHeight;
}

// HTML转义
function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

// Agent模式开关
function toggleAgentMode(enabled) {
    agentModeEnabled = enabled;
    console.log(`受控 Agent 执行: ${enabled ? '开启' : '关闭'}`);
    const input = document.getElementById('messageInput');
    if (input) {
        input.placeholder = enabled
            ? '受控执行：按身份与会话边界调用已登记工具...'
            : '例如：显示张奶奶最近3天的血压趋势...';
    }
    const status = document.getElementById('agentModeStatus');
    if (status) {
        status.textContent = enabled ? '当前：受控 Agent 执行' : '当前：普通对话';
    }
}

// 显示加载动画
function showTypingIndicator() {
    const container = document.getElementById('messagesContainer');
    const typingDiv = document.createElement('div');
    typingDiv.className = 'message assistant';
    typingDiv.id = 'typingIndicator';

    const avatarIcon = CURRENT_ROLE_CONFIG.role === 'caregiver' ? '🧑‍⚕️' :
        CURRENT_ROLE_CONFIG.role === 'doctor' ? '👨‍⚕️' :
        CURRENT_ROLE_CONFIG.role === 'family' ? '👨‍👩‍👧' : '👴';

    typingDiv.innerHTML = `
        <div class="message-avatar">${avatarIcon}</div>
        <div class="message-content">
            <div class="message-bubble">
                <div class="typing-indicator">
                    <div class="typing-dot"></div>
                    <div class="typing-dot"></div>
                    <div class="typing-dot"></div>
                </div>
            </div>
        </div>
    `;
    container.appendChild(typingDiv);
    container.scrollTop = container.scrollHeight;
}

// 隐藏加载动画
function hideTypingIndicator() {
    const indicator = document.getElementById('typingIndicator');
    if (indicator) {
        indicator.remove();
    }
}

// 保存当前会话
function saveCurrentSession() {
    if (currentSessionID && messageHistory.length > 0) {
        sessions[currentSessionID] = {
            title: currentSessionTitle,
            messages: [...messageHistory],
            timestamp: Date.now()
        };
        const storageKey = `chatSessions_${CURRENT_ROLE_CONFIG.role}_${USER_ID}`;
        localStorage.setItem(storageKey, JSON.stringify(sessions));
        updateHistoryList();
    }
}

// 新对话
function newChat() {
    saveCurrentSession();

    if (!currentSessionID) {
        currentSessionID = 'session_' + USER_ID + '_' + Date.now();
    }

    messageHistory = [];
    isFirstMessage = true;
    currentSessionTitle = '新对话 ' + new Date().toLocaleString('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' });

    // 清空消息容器，显示欢迎界面
    const container = document.getElementById('messagesContainer');
    const welcomeIcon = CURRENT_ROLE_CONFIG.role === 'caregiver' ? '🧑‍⚕️' :
        CURRENT_ROLE_CONFIG.role === 'doctor' ? '👨‍⚕️' :
        CURRENT_ROLE_CONFIG.role === 'family' ? '👨‍👩‍👧' : '👴';

    let quickQuestionsHTML = '';
    if (CURRENT_ROLE_CONFIG.quickQuestions) {
        quickQuestionsHTML = CURRENT_ROLE_CONFIG.quickQuestions.map(q => {
            const action = q.category
                ? `saveNursingRecord(${JSON.stringify(q.question)}, ${JSON.stringify(q.category)})`
                : `sendQuickQuestion(${JSON.stringify(q.question)})`;
            return `
            <div class="quick-question" onclick='${action}'>
                <div class="quick-question-icon">${q.icon}</div>
                <div class="quick-question-text">${q.text}</div>
            </div>
        `;
        }).join('');
    }

    container.innerHTML = `
        <div class="welcome-screen" id="welcomeScreen">
            <div class="welcome-icon">${welcomeIcon}</div>
            <div class="welcome-title">您好！我是${CURRENT_ROLE_CONFIG.roleName}助手</div>
            <div class="welcome-desc">
                ${getWelcomeDesc()}
            </div>
            <div class="quick-questions">
                ${quickQuestionsHTML}
            </div>
        </div>
    `;

    updateHistoryList();
}

// 获取欢迎描述
function getWelcomeDesc() {
    switch (CURRENT_ROLE_CONFIG.role) {
        case 'caregiver':
            return '我可以帮您快速记录老人的生命体征数据、护理情况、用药信息等。只需用对话方式告诉我，我会自动识别并保存到系统中。';
        case 'doctor':
            return '我可以帮您查询和分析老人的健康数据，生成趋势图表和健康报告。您可以通过对话方式询问任何健康相关的问题。';
        case 'family':
            return '我可以向您报告老人的健康状况，解答您的关心和疑问。您可以随时询问老人的情况，我会用通俗易懂的语言为您解释。';
        case 'elder':
            return '您可以点击下面的按钮，或者点击麦克风说话，我会告诉您的健康状况。';
        default:
            return '';
    }
}

// 附件功能
function attachFile() {
    document.getElementById('fileInput').click();
}

// 处理文件选择
function handleFileSelect(event) {
    const files = Array.from(event.target.files);
    const config = window.CONFIG || { FILE_UPLOAD: { MAX_SIZE: 10 * 1024 * 1024, ALLOWED_TYPES: ['.jpg', '.jpeg', '.png', '.pdf', '.txt'] } };

    for (const file of files) {
        const maxSize = config.FILE_UPLOAD.MAX_SIZE;
        if (file.size > maxSize) {
            alert(`文件 ${file.name} 大小超过${(maxSize / 1024 / 1024).toFixed(0)}MB限制`);
            continue;
        }

        const allowedTypes = config.FILE_UPLOAD.ALLOWED_TYPES;
        const fileExt = '.' + file.name.split('.').pop().toLowerCase();
        if (!allowedTypes.includes(fileExt)) {
            alert(`文件 ${file.name} 类型不支持`);
            continue;
        }

        selectedFiles.push(file);
    }

    updateFilePreview();
    event.target.value = '';
}

// 更新文件预览
function updateFilePreview() {
    const container = document.getElementById('filePreviewContainer');
    if (!container) return;

    container.innerHTML = '';

    selectedFiles.forEach((file, index) => {
        const fileItem = document.createElement('div');
        fileItem.className = 'file-preview-item';

        const fileIcon = getFileIcon(file.name);
        const fileSize = formatFileSize(file.size);

        fileItem.innerHTML = `
            <div class="file-preview-icon">${fileIcon}</div>
            <div class="file-preview-info">
                <div class="file-preview-name">${file.name}</div>
                <div class="file-preview-size">${fileSize}</div>
            </div>
            <button class="file-preview-remove" onclick="removeFile(${index})">×</button>
        `;

        container.appendChild(fileItem);
    });
}

// 移除文件
function removeFile(index) {
    selectedFiles.splice(index, 1);
    updateFilePreview();
}

// 获取文件图标
function getFileIcon(filename) {
    const ext = filename.split('.').pop().toLowerCase();
    const iconMap = {
        'jpg': '🖼️', 'jpeg': '🖼️', 'png': '🖼️',
        'pdf': '📄',
        'txt': '📃'
    };
    return iconMap[ext] || '📎';
}

// 格式化文件大小
function formatFileSize(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(2) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(2) + ' MB';
}

// 上传文件到服务器
async function uploadFiles(files) {
    if (!JWT_TOKEN) {
        throw new Error('无法获取认证令牌，请刷新页面重试');
    }

    const uploadedFiles = [];

    for (const file of files) {
        const formData = new FormData();
        formData.append('file', file);

        console.log(`[文件上传] 开始上传文件: ${file.name}, 大小: ${file.size} bytes, 用户ID: ${USER_ID}`);

        try {
            const response = await fetch(`${API_BASE_URL}/api/files/upload`, {
                method: 'POST',
                headers: {
                    'Authorization': `Bearer ${JWT_TOKEN}`
                },
                body: formData
            });

            console.log(`[文件上传] 响应状态: ${response.status}`);

            if (!response.ok) {
                if (response.status === 401) {
                    clearAuth();
                    JWT_TOKEN = '';
                    throw new Error('认证失败，请刷新页面重试');
                }

                // 尝试读取错误响应
                let errorMessage = `上传失败 (HTTP ${response.status})`;
                try {
                    const errorData = await response.json();
                    if (errorData.error) {
                        errorMessage = errorData.error;
                    }
                } catch (e) {
                    console.error('[文件上传] 无法解析错误响应:', e);
                }

                throw new Error(errorMessage);
            }

            const data = await response.json();
            console.log('[文件上传] 响应数据:', data);

            if (data.success) {
                uploadedFiles.push(data.data);
                console.log(`[文件上传] 文件上传成功: ${file.name}`);
            } else {
                throw new Error(data.error || '上传失败');
            }
        } catch (error) {
            console.error(`[文件上传] 文件 ${file.name} 上传失败:`, error);
            throw error;
        }
    }

    return uploadedFiles;
}

// 加载用户会话
function loadUserSessions() {
    const storageKey = `chatSessions_${CURRENT_ROLE_CONFIG.role}_${USER_ID}`;
    const savedSessions = localStorage.getItem(storageKey);
    if (savedSessions) {
        try {
            sessions = JSON.parse(savedSessions);
            console.log(`加载用户 ${USER_ID} 的历史会话:`, Object.keys(sessions).length, '个会话');
            updateHistoryList();
        } catch (e) {
            console.error('加载历史会话失败:', e);
            sessions = {};
        }
    } else {
        console.log(`用户 ${USER_ID} 暂无历史会话`);
        sessions = {};
        updateHistoryList();
    }
}

// 更新历史记录列表
function updateHistoryList() {
    const historyContainer = document.getElementById('chatHistory');
    if (!historyContainer) return;

    historyContainer.innerHTML = '';

    const sortedSessions = Object.entries(sessions).sort((a, b) => b[1].timestamp - a[1].timestamp);

    sortedSessions.forEach(([sessionId, session]) => {
        const historyItem = document.createElement('div');
        historyItem.className = 'history-item' + (sessionId === currentSessionID ? ' active' : '');

        const icon = getSessionIcon(session.title);
        historyItem.innerHTML = `${icon} ${session.title}`;

        historyItem.addEventListener('click', function() {
            loadSession(sessionId);
        });

        historyContainer.appendChild(historyItem);
    });

    if (sortedSessions.length === 0) {
        historyContainer.innerHTML = `
            <div style="padding: 20px; text-align: center; color: #95a5a6; font-size: 14px;">
                暂无历史记录<br>
                <span style="font-size: 12px;">开始新对话后会显示在这里</span>
            </div>
        `;
    }
}

// 根据标题获取图标
function getSessionIcon(title) {
    if (title.includes('血压') || title.includes('数据')) return '📝';
    if (title.includes('体检') || title.includes('报告')) return '📊';
    if (title.includes('用药') || title.includes('药物')) return '💊';
    return '💬';
}

// 加载会话
function loadSession(sessionId) {
    saveCurrentSession();

    const session = sessions[sessionId];
    if (!session) return;

    currentSessionID = sessionId;
    messageHistory = [...session.messages];
    currentSessionTitle = session.title;
    isFirstMessage = false;

    const container = document.getElementById('messagesContainer');
    container.innerHTML = '';

    messageHistory.forEach(msg => {
        addMessage(msg.role, msg.content, msg.timestamp);
    });

    updateHistoryList();
}

// 页面关闭前保存当前会话
window.addEventListener('beforeunload', function() {
    saveCurrentSession();
});

// 🔒 显示敏感信息检测警告
function showSensitiveInfoWarning(sensitiveInfos) {
    const container = document.getElementById('messagesContainer');

    // 创建警告消息元素
    const warningDiv = document.createElement('div');
    warningDiv.className = 'message-wrapper system-warning';
    warningDiv.style.cssText = `
        margin: 10px 0;
        padding: 12px 16px;
        background: linear-gradient(135deg, #fff3cd 0%, #ffeaa7 100%);
        border-left: 4px solid #ffc107;
        border-radius: 8px;
        box-shadow: 0 2px 8px rgba(255, 193, 7, 0.2);
        animation: slideIn 0.3s ease-out;
    `;

    // 构建警告内容
    let warningHTML = `
        <div style="display: flex; align-items: start; gap: 10px;">
            <div style="font-size: 20px; flex-shrink: 0;">🔒</div>
            <div style="flex: 1;">
                <div style="font-weight: bold; color: #856404; margin-bottom: 8px;">
                    检测到 ${sensitiveInfos.length} 个敏感信息已自动脱敏
                </div>
                <div style="font-size: 13px; color: #856404; line-height: 1.6;">
    `;

    // 显示每个敏感信息的详情
    sensitiveInfos.forEach((info, index) => {
        const typeNames = {
            'phone': '手机号',
            'id_card': '身份证号',
            'bank_card': '银行卡号',
            'email': '邮箱地址',
            'api_key': 'API密钥',
            'password': '密码',
            'credit_card': '信用卡号',
            'ssn': '社会保障号',
            'passport': '护照号'
        };

        const typeName = typeNames[info.type] || info.type;
        const confidence = info.confidence ? `(置信度: ${(info.confidence * 100).toFixed(0)}%)` : '';

        warningHTML += `
            <div style="margin: 4px 0; padding: 6px 10px; background: rgba(255,255,255,0.6); border-radius: 4px;">
                <span style="font-weight: 600;">类型:</span> ${escapeHtml(typeName)} ${confidence}<br>
                <span style="font-weight: 600;">状态:</span> 已按策略脱敏
            </div>
        `;
    });

    warningHTML += `
                </div>
                <div style="margin-top: 8px; font-size: 12px; color: #856404; opacity: 0.8;">
                    💡 系统已自动保护您的隐私信息，请勿在对话中传输敏感数据
                </div>
            </div>
        </div>
    `;

    warningDiv.innerHTML = warningHTML;
    container.appendChild(warningDiv);

    // 滚动到底部
    container.scrollTop = container.scrollHeight;
}

// 🔒 显示安全警告
function showSecurityWarnings(warnings) {
    const container = document.getElementById('messagesContainer');

    // 创建警告消息元素
    const warningDiv = document.createElement('div');
    warningDiv.className = 'message-wrapper security-warning';
    warningDiv.style.cssText = `
        margin: 10px 0;
        padding: 12px 16px;
        background: linear-gradient(135deg, #ffe5e5 0%, #ffcccc 100%);
        border-left: 4px solid #e74c3c;
        border-radius: 8px;
        box-shadow: 0 2px 8px rgba(231, 76, 60, 0.2);
        animation: slideIn 0.3s ease-out;
    `;

    let warningHTML = `
        <div style="display: flex; align-items: start; gap: 10px;">
            <div style="font-size: 20px; flex-shrink: 0;">⚠️</div>
            <div style="flex: 1;">
                <div style="font-weight: bold; color: #c0392b; margin-bottom: 8px;">
                    安全提醒
                </div>
                <div style="font-size: 13px; color: #c0392b; line-height: 1.6;">
    `;

    warnings.forEach((warning, index) => {
        warningHTML += `
            <div style="margin: 4px 0; padding: 6px 10px; background: rgba(255,255,255,0.6); border-radius: 4px;">
                ${index + 1}. ${warning}
            </div>
        `;
    });

    warningHTML += `
                </div>
            </div>
        </div>
    `;

    warningDiv.innerHTML = warningHTML;
    container.appendChild(warningDiv);

    // 滚动到底部
    container.scrollTop = container.scrollHeight;
}

// 离线模式 AI 模拟回复
function generateOfflineReply(message) {
    const role = CURRENT_ROLE_CONFIG ? CURRENT_ROLE_CONFIG.role : 'caregiver';

    const replies = {
        caregiver: [
            '好的，我已经记录了这条信息。如果您还有其他需要记录的内容，请继续告诉我。',
            '已为您保存记录。如需查看历史数据，可以直接问我。',
            '收到！这条记录已保存到系统中。还有其他老人的情况需要记录吗？',
        ],
        doctor: [
            '根据目前的数据，老人的各项指标在正常范围内。建议您继续关注血压和血糖的变化趋势。',
            '我已分析了老人的健康数据，整体状况良好。如果您需要查看详细的趋势图表，可以告诉我具体的时间范围。',
            '从数据来看，老人的健康状况比较稳定。如需进一步分析或生成报告，请随时告诉我。',
        ],
        family: [
            '您的家人今天状态不错，各项指标都在正常范围内。如果有特别想了解的方面，可以具体问我。',
            '老人今天的身体状况良好，饮食和休息都正常。您可以放心，我们会继续照顾好老人的。',
            '一切都挺好的，老人今天心情也不错。如果您有什么担心的事情，随时可以问我。',
        ],
        elder: [
            '您今天的身体状况挺好的！记得按时吃药，多喝水哦。',
            '您的血压和心率都在正常范围内，继续保持！有哪里不舒服随时告诉我。',
            '今天天气不错，适合出去走走。您的身体指标都很正常，放心！',
        ]
    };

    const roleReplies = replies[role] || replies.caregiver;
    return roleReplies[Math.floor(Math.random() * roleReplies.length)];
}

// Override legacy login flow: require explicit password in online mode and
// never silently downgrade to offline mode on authentication failure.
async function quickLogin(userName, userId) {
    clearAuthError();
    console.log(`[auth] login start: ${userName} (${userId})`);

    if (window.CONFIG && window.CONFIG.OFFLINE_MODE) {
        console.log('[auth] explicit offline demo mode enabled');
        loginWithExplicitOfflineMode(userName, userId);
        return;
    }

    const password = getDemoPassword();
    if (!password) {
        renderSecurityNotice();
        showAuthError('请输入演示密码后再登录');
        const passwordInput = document.getElementById('demoPasswordInput');
        if (passwordInput) {
            passwordInput.focus();
        }
        return;
    }

    const loginOverlay = document.getElementById('loginOverlay');
    const loginStatus = document.getElementById('loginStatusMessage');
    if (loginStatus) {
        loginStatus.style.display = 'none';
    }

    try {
        if (loginOverlay) {
            loginOverlay.style.pointerEvents = 'none';
            loginOverlay.style.opacity = '0.96';
        }

        const response = await fetch(`${API_BASE_URL}/api/auth/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                user_id: userId,
                password: password,
                workspace_id: window.CONFIG ? window.CONFIG.DEFAULT_WORKSPACE : 'default'
            })
        });

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        const data = await response.json();
        if (!data.success || !data.data || !data.data.token) {
            throw new Error('登录响应格式错误');
        }

        JWT_TOKEN = data.data.token;
        USER_ID = userId;
        USER_NAME = userName;
        persistAuth(JWT_TOKEN, USER_ID, USER_NAME);

        const userAvatar = document.getElementById('userAvatar');
        if (userAvatar) {
            userAvatar.textContent = userName;
        }

        if (loginOverlay) {
            loginOverlay.style.display = 'none';
            loginOverlay.style.pointerEvents = '';
            loginOverlay.style.opacity = '';
        }

        initializeAfterLogin();
    } catch (error) {
        console.error('[auth] login failed:', error);
        if (loginOverlay) {
            loginOverlay.style.pointerEvents = '';
            loginOverlay.style.opacity = '';
        }
        renderSecurityNotice();
        showAuthError(`登录失败：${error.message}。如需本地演示，请显式开启 OFFLINE_MODE。`);
    }
}

// 覆盖旧版 quickLogin：在线模式不再静默降级到离线模式
async function quickLogin(userName, userId) {
    clearAuthError();
    console.log(`[认证] 开始登录: ${userName} (${userId})`);

    if (window.CONFIG && window.CONFIG.OFFLINE_MODE) {
        console.log('[认证] 使用显式离线演示模式');
        loginWithExplicitOfflineMode(userName, userId);
        return;
    }

    const password = getDemoPassword();
    if (!password) {
        renderSecurityNotice();
        showAuthError('请输入演示密码后再登录');
        const passwordInput = document.getElementById('demoPasswordInput');
        if (passwordInput) {
            passwordInput.focus();
        }
        return;
    }

    try {
        const response = await fetch(`${API_BASE_URL}/api/auth/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                user_id: userId,
                password: password,
                workspace_id: window.CONFIG ? window.CONFIG.DEFAULT_WORKSPACE : 'default'
            })
        });

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        const data = await response.json();
        if (!data.success || !data.data || !data.data.token) {
            throw new Error('登录响应格式错误');
        }

        JWT_TOKEN = data.data.token;
        USER_ID = userId;
        USER_NAME = userName;
        persistAuth(JWT_TOKEN, USER_ID, USER_NAME);

        const userAvatar = document.getElementById('userAvatar');
        if (userAvatar) {
            userAvatar.textContent = userName;
        }

        const loginOverlay = document.getElementById('loginOverlay');
        if (loginOverlay) {
            loginOverlay.style.display = 'none';
        }

        initializeAfterLogin();
    } catch (error) {
        console.error('[认证] 登录失败:', error);
        renderSecurityNotice();
        showAuthError(`登录失败：${error.message}。如需本地演示，请显式开启 OFFLINE_MODE。`);
    }
}

// ========== 可视化处理函数 ==========

/**
 * 检测并处理可视化需求
 * @param {string} userMessage - 用户的原始消息
 * @param {string} assistantReply - LLM的回复内容
 */
async function handleVisualizationIfNeeded(userMessage, assistantReply) {
    // 检测是否是可视化请求
    const visualizationKeywords = ['生成.*图', '画.*图', '折线图', '柱状图', '饼图', '趋势图', '可视化'];
    const isVisualizationRequest = visualizationKeywords.some(keyword => new RegExp(keyword).test(userMessage));

    if (!isVisualizationRequest) {
        return; // 不是可视化请求，直接返回
    }

    console.log('[可视化] 检测到可视化请求，开始处理...');

    // 从用户消息或LLM回复中提取目标人物
    const personMatch = (userMessage + ' ' + assistantReply).match(/(张奶奶|李爷爷|王奶奶|赵爷爷|.*?奶奶|.*?爷爷)/);
    const targetPerson = personMatch ? personMatch[1] : '目标老人';

    // 从LLM回复中提取血压数据
    // 匹配格式：YYYY-MM-DD: 收缩压/舒张压mmHg 或 MM-DD: 收缩压/舒张压
    const bloodPressurePattern = /(\d{4}-\d{2}-\d{2}|\d{2}-\d{2}|\d{1,2}月\d{1,2}日)[：:]\s*(\d{2,3})[/／](\d{2,3})\s*mmHg/g;
    const matches = Array.from(assistantReply.matchAll(bloodPressurePattern));

    if (matches.length === 0) {
        console.log('[可视化] 回复中未找到血压数据，无法生成图表');
        return;
    }

    console.log(`[可视化] 提取到 ${matches.length} 条血压数据`);

    // 构建数据上下文
    let dataContext = `【${targetPerson}最近血压记录】\n`;
    matches.forEach(match => {
        const date = match[1];
        const systolic = match[2];
        const diastolic = match[3];

        // 标准化日期格式
        let standardDate = date;
        if (date.includes('月')) {
            // 将 "1月15日" 转换为 "01-15"
            const monthDay = date.match(/(\d{1,2})月(\d{1,2})日/);
            if (monthDay) {
                standardDate = `${monthDay[1].padStart(2, '0')}-${monthDay[2].padStart(2, '0')}`;
            }
        } else if (date.length === 10) {
            // YYYY-MM-DD -> MM-DD
            standardDate = date.substring(5);
        }

        dataContext += `${standardDate}: ${systolic}/${diastolic}mmHg\n`;
    });

    console.log('[可视化] 数据上下文:\n', dataContext);

    try {
        // 调用可视化服务生成图表
        const vizResponse = await fetch('http://localhost:5001/generate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                chart_type: 'line',
                title: `${targetPerson}血压趋势图`,
                data_context: dataContext
            })
        });

        if (!vizResponse.ok) {
            throw new Error(`可视化服务返回 ${vizResponse.status}`);
        }

        const vizData = await vizResponse.json();
        console.log('[可视化] 图表生成成功:', vizData);

        // 在聊天界面中添加图表消息
        const chartMessage = `📊 **图表已生成**

🔗 查看图表：${vizData.chart_url}

💾 保存路径：${vizData.chart_path}`;

        addMessage('assistant', chartMessage);
        messageHistory.push({
            role: 'assistant',
            content: chartMessage,
            timestamp: Date.now()
        });
        saveCurrentSession();

    } catch (error) {
        console.error('[可视化] 图表生成失败:', error);

        const errorMessage = `⚠️ 图表生成失败：${error.message}

请确认可视化服务是否正常运行：http://localhost:5001/health`;

        addMessage('assistant', errorMessage);
        messageHistory.push({
            role: 'assistant',
            content: errorMessage,
            timestamp: Date.now()
        });
        saveCurrentSession();
    }
}
